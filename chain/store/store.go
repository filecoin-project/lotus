package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	amt "github.com/filecoin-project/go-amt-ipld"
	"github.com/filecoin-project/go-lotus/chain/types"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/pkg/errors"
	cbg "github.com/whyrusleeping/cbor-gen"
	pubsub "github.com/whyrusleeping/pubsub"
	"golang.org/x/xerrors"
)

var log = logging.Logger("chainstore")

var chainHeadKey = dstore.NewKey("head")

type ChainStore struct {
	bs bstore.Blockstore
	ds dstore.Datastore

	heaviestLk sync.Mutex
	heaviest   *types.TipSet

	bestTips *pubsub.PubSub
	pubLk    sync.Mutex

	tstLk   sync.Mutex
	tipsets map[uint64][]cid.Cid

	headChangeNotifs []func(rev, app []*types.TipSet) error
}

func NewChainStore(bs bstore.Blockstore, ds dstore.Batching) *ChainStore {
	cs := &ChainStore{
		bs:       bs,
		ds:       ds,
		bestTips: pubsub.New(64),
		tipsets:  make(map[uint64][]cid.Cid),
	}

	hcnf := func(rev, app []*types.TipSet) error {
		cs.pubLk.Lock()
		defer cs.pubLk.Unlock()
		for _, r := range rev {
			cs.bestTips.Pub(&HeadChange{
				Type: HCRevert,
				Val:  r,
			}, "headchange")
		}
		for _, r := range app {
			cs.bestTips.Pub(&HeadChange{
				Type: HCApply,
				Val:  r,
			}, "headchange")
		}
		return nil
	}

	cs.headChangeNotifs = append(cs.headChangeNotifs, hcnf)

	return cs
}

func (cs *ChainStore) Load() error {
	head, err := cs.ds.Get(chainHeadKey)
	if err == dstore.ErrNotFound {
		log.Warn("no previous chain state found")
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "failed to load chain state from datastore")
	}

	var tscids []cid.Cid
	if err := json.Unmarshal(head, &tscids); err != nil {
		return errors.Wrap(err, "failed to unmarshal stored chain head")
	}

	ts, err := cs.LoadTipSet(tscids)
	if err != nil {
		return err
	}

	cs.heaviest = ts

	return nil
}

func (cs *ChainStore) writeHead(ts *types.TipSet) error {
	data, err := json.Marshal(ts.Cids())
	if err != nil {
		return errors.Wrap(err, "failed to marshal tipset")
	}

	if err := cs.ds.Put(chainHeadKey, data); err != nil {
		return errors.Wrap(err, "failed to write chain head to datastore")
	}

	return nil
}

func (cs *ChainStore) SubNewTips() chan *types.TipSet {
	subch := cs.bestTips.Sub("best")
	out := make(chan *types.TipSet)
	go func() {
		defer close(out)
		for val := range subch {
			out <- val.(*types.TipSet)
		}
	}()
	return out
}

const (
	HCRevert  = "revert"
	HCApply   = "apply"
	HCCurrent = "current"
)

type HeadChange struct {
	Type string
	Val  *types.TipSet
}

func (cs *ChainStore) SubHeadChanges(ctx context.Context) chan *HeadChange {
	cs.pubLk.Lock()
	subch := cs.bestTips.Sub("headchange")
	head := cs.GetHeaviestTipSet()
	cs.pubLk.Unlock()

	out := make(chan *HeadChange, 16)
	out <- &HeadChange{
		Type: HCCurrent,
		Val:  head,
	}

	go func() {
		defer close(out)
		for {
			select {
			case val, ok := <-subch:
				if !ok {
					log.Warn("chain head sub exit loop")
					return
				}
				select {
				case out <- val.(*HeadChange):
				case <-ctx.Done():
				}
			case <-ctx.Done():
				go cs.bestTips.Unsub(subch)
			}
		}
	}()
	return out
}

func (cs *ChainStore) SubscribeHeadChanges(f func(rev, app []*types.TipSet) error) {
	cs.headChangeNotifs = append(cs.headChangeNotifs, f)
}

func (cs *ChainStore) SetGenesis(b *types.BlockHeader) error {
	fts := &FullTipSet{
		Blocks: []*types.FullBlock{
			{Header: b},
		},
	}

	if err := cs.PutTipSet(fts); err != nil {
		return err
	}

	return cs.ds.Put(dstore.NewKey("0"), b.Cid().Bytes())
}

func (cs *ChainStore) PutTipSet(ts *FullTipSet) error {
	for _, b := range ts.Blocks {
		if err := cs.persistBlock(b); err != nil {
			return err
		}
	}

	expanded, err := cs.expandTipset(ts.TipSet().Blocks()[0])
	if err != nil {
		return xerrors.Errorf("errored while expanding tipset: %w", err)
	}
	fmt.Printf("expanded %s into %s\n", ts.TipSet().Cids(), expanded.Cids())

	if err := cs.MaybeTakeHeavierTipSet(expanded); err != nil {
		return errors.Wrap(err, "MaybeTakeHeavierTipSet failed in PutTipSet")
	}
	return nil
}

func (cs *ChainStore) MaybeTakeHeavierTipSet(ts *types.TipSet) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	if cs.heaviest == nil || cs.Weight(ts) > cs.Weight(cs.heaviest) {
		// TODO: don't do this for initial sync. Now that we don't have a
		// difference between 'bootstrap sync' and 'caught up' sync, we need
		// some other heuristic.
		if cs.heaviest != nil {
			revert, apply, err := cs.ReorgOps(cs.heaviest, ts)
			if err != nil {
				return errors.Wrap(err, "computing reorg ops failed")
			}
			for _, hcf := range cs.headChangeNotifs {
				if err := hcf(revert, apply); err != nil {
					return errors.Wrap(err, "head change func errored (BAD)")
				}
			}
		} else {
			log.Warn("no heaviest tipset found, using %s", ts.Cids())
		}

		log.Infof("New heaviest tipset! %s", ts.Cids())
		cs.heaviest = ts

		if err := cs.writeHead(ts); err != nil {
			log.Errorf("failed to write chain head: %s", err)
			return nil
		}
	}
	return nil
}

func (cs *ChainStore) Contains(ts *types.TipSet) (bool, error) {
	for _, c := range ts.Cids() {
		has, err := cs.bs.Has(c)
		if err != nil {
			return false, err
		}

		if !has {
			return false, nil
		}
	}
	return true, nil
}

func (cs *ChainStore) GetBlock(c cid.Cid) (*types.BlockHeader, error) {
	sb, err := cs.bs.Get(c)
	if err != nil {
		return nil, err
	}

	return types.DecodeBlock(sb.RawData())
}

func (cs *ChainStore) LoadTipSet(cids []cid.Cid) (*types.TipSet, error) {
	var blks []*types.BlockHeader
	for _, c := range cids {
		b, err := cs.GetBlock(c)
		if err != nil {
			return nil, err
		}

		blks = append(blks, b)
	}

	return types.NewTipSet(blks)
}

// returns true if 'a' is an ancestor of 'b'
func (cs *ChainStore) IsAncestorOf(a, b *types.TipSet) (bool, error) {
	if b.Height() <= a.Height() {
		return false, nil
	}

	cur := b
	for !a.Equals(cur) && cur.Height() > a.Height() {
		next, err := cs.LoadTipSet(b.Parents())
		if err != nil {
			return false, err
		}

		cur = next
	}

	return cur.Equals(a), nil
}

func (cs *ChainStore) NearestCommonAncestor(a, b *types.TipSet) (*types.TipSet, error) {
	l, _, err := cs.ReorgOps(a, b)
	if err != nil {
		return nil, err
	}

	return cs.LoadTipSet(l[len(l)-1].Parents())
}

func (cs *ChainStore) ReorgOps(a, b *types.TipSet) ([]*types.TipSet, []*types.TipSet, error) {
	left := a
	right := b

	var leftChain, rightChain []*types.TipSet
	for !left.Equals(right) {
		if left.Height() > right.Height() {
			leftChain = append(leftChain, left)
			par, err := cs.LoadTipSet(left.Parents())
			if err != nil {
				return nil, nil, err
			}

			left = par
		} else {
			rightChain = append(rightChain, right)
			par, err := cs.LoadTipSet(right.Parents())
			if err != nil {
				log.Infof("failed to fetch right.Parents: %s", err)
				return nil, nil, err
			}

			right = par
		}
	}

	return leftChain, rightChain, nil
}

func (cs *ChainStore) Weight(ts *types.TipSet) uint64 {
	return ts.Blocks()[0].ParentWeight.Uint64() + uint64(len(ts.Cids()))
}

func (cs *ChainStore) GetHeaviestTipSet() *types.TipSet {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	return cs.heaviest
}

func (cs *ChainStore) addToTipSetTracker(b *types.BlockHeader) error {
	cs.tstLk.Lock()
	defer cs.tstLk.Unlock()

	tss := cs.tipsets[b.Height]
	for _, oc := range tss {
		if oc == b.Cid() {
			log.Warn("tried to add block to tipset tracker that was already there")
			return nil
		}
	}

	cs.tipsets[b.Height] = append(tss, b.Cid())

	// TODO: do we want to look for slashable submissions here? might as well...

	return nil
}

func (cs *ChainStore) PersistBlockHeader(b *types.BlockHeader) error {
	sb, err := b.ToStorageBlock()
	if err != nil {
		return err
	}

	if err := cs.addToTipSetTracker(b); err != nil {
		return xerrors.Errorf("failed to insert new block (%s) into tipset tracker: %w", b.Cid(), err)
	}

	return cs.bs.Put(sb)
}

func (cs *ChainStore) persistBlock(b *types.FullBlock) error {
	if err := cs.PersistBlockHeader(b.Header); err != nil {
		return err
	}

	for _, m := range b.BlsMessages {
		if _, err := cs.PutMessage(m); err != nil {
			return err
		}
	}
	for _, m := range b.SecpkMessages {
		if _, err := cs.PutMessage(m); err != nil {
			return err
		}
	}
	return nil
}

type storable interface {
	ToStorageBlock() (block.Block, error)
}

func PutMessage(bs blockstore.Blockstore, m storable) (cid.Cid, error) {
	b, err := m.ToStorageBlock()
	if err != nil {
		return cid.Undef, err
	}

	if err := bs.Put(b); err != nil {
		return cid.Undef, err
	}

	return b.Cid(), nil
}

func (cs *ChainStore) PutMessage(m storable) (cid.Cid, error) {
	return PutMessage(cs.bs, m)
}

func (cs *ChainStore) expandTipset(b *types.BlockHeader) (*types.TipSet, error) {
	// Hold lock for the whole function for now, if it becomes a problem we can
	// fix pretty easily
	cs.tstLk.Lock()
	defer cs.tstLk.Unlock()

	all := []*types.BlockHeader{b}

	tsets, ok := cs.tipsets[b.Height]
	if !ok {
		return types.NewTipSet(all)
	}

	for _, bhc := range tsets {
		if bhc == b.Cid() {
			continue
		}

		h, err := cs.GetBlock(bhc)
		if err != nil {
			return nil, xerrors.Errorf("failed to load block (%s) for tipset expansion: %w", bhc, err)
		}

		if types.CidArrsEqual(h.Parents, b.Parents) {
			all = append(all, h)
		}
	}

	// TODO: other validation...?

	return types.NewTipSet(all)
}

func (cs *ChainStore) AddBlock(b *types.BlockHeader) error {
	if err := cs.PersistBlockHeader(b); err != nil {
		return err
	}

	ts, err := cs.expandTipset(b)
	if err != nil {
		return err
	}

	if err := cs.MaybeTakeHeavierTipSet(ts); err != nil {
		return errors.Wrap(err, "MaybeTakeHeavierTipSet failed")
	}

	return nil
}

func (cs *ChainStore) GetGenesis() (*types.BlockHeader, error) {
	data, err := cs.ds.Get(dstore.NewKey("0"))
	if err != nil {
		return nil, err
	}

	c, err := cid.Cast(data)
	if err != nil {
		return nil, err
	}

	genb, err := cs.bs.Get(c)
	if err != nil {
		return nil, err
	}

	return types.DecodeBlock(genb.RawData())
}

func (cs *ChainStore) GetMessage(c cid.Cid) (*types.Message, error) {
	sb, err := cs.bs.Get(c)
	if err != nil {
		log.Errorf("get message get failed: %s: %s", c, err)
		return nil, err
	}

	return types.DecodeMessage(sb.RawData())
}

func (cs *ChainStore) GetSignedMessage(c cid.Cid) (*types.SignedMessage, error) {
	sb, err := cs.bs.Get(c)
	if err != nil {
		log.Errorf("get message get failed: %s: %s", c, err)
		return nil, err
	}

	return types.DecodeSignedMessage(sb.RawData())
}

func (cs *ChainStore) readAMTCids(root cid.Cid) ([]cid.Cid, error) {
	bs := amt.WrapBlockstore(cs.bs)
	a, err := amt.LoadAMT(bs, root)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}

	var cids []cid.Cid
	for i := uint64(0); i < a.Count; i++ {
		var c cbg.CborCid
		if err := a.Get(i, &c); err != nil {
			return nil, xerrors.Errorf("failed to load cid from amt: %w", err)
		}

		cids = append(cids, cid.Cid(c))
	}

	return cids, nil
}

func (cs *ChainStore) MessagesForBlock(b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	cst := hamt.CSTFromBstore(cs.bs)
	var msgmeta types.MsgMeta
	if err := cst.Get(context.TODO(), b.Messages, &msgmeta); err != nil {
		return nil, nil, xerrors.Errorf("failed to load msgmeta: %w", err)
	}

	blscids, err := cs.readAMTCids(msgmeta.BlsMessages)
	if err != nil {
		return nil, nil, errors.Wrap(err, "loading bls message cids for block")
	}

	secpkcids, err := cs.readAMTCids(msgmeta.SecpkMessages)
	if err != nil {
		return nil, nil, errors.Wrap(err, "loading secpk message cids for block")
	}

	blsmsgs, err := cs.LoadMessagesFromCids(blscids)
	if err != nil {
		return nil, nil, errors.Wrap(err, "loading bls messages for block")
	}

	secpkmsgs, err := cs.LoadSignedMessagesFromCids(secpkcids)
	if err != nil {
		return nil, nil, errors.Wrap(err, "loading secpk messages for block")
	}

	return blsmsgs, secpkmsgs, nil
}

func (cs *ChainStore) GetReceipt(b *types.BlockHeader, i int) (*types.MessageReceipt, error) {
	bs := amt.WrapBlockstore(cs.bs)
	a, err := amt.LoadAMT(bs, b.MessageReceipts)
	if err != nil {
		return nil, errors.Wrap(err, "amt load")
	}

	var r types.MessageReceipt
	if err := a.Get(uint64(i), &r); err != nil {
		return nil, err
	}

	return &r, nil
}

func (cs *ChainStore) LoadMessagesFromCids(cids []cid.Cid) ([]*types.Message, error) {
	msgs := make([]*types.Message, 0, len(cids))
	for i, c := range cids {
		m, err := cs.GetMessage(c)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get message: (%s):%d", c, i)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (cs *ChainStore) LoadSignedMessagesFromCids(cids []cid.Cid) ([]*types.SignedMessage, error) {
	msgs := make([]*types.SignedMessage, 0, len(cids))
	for i, c := range cids {
		m, err := cs.GetSignedMessage(c)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get message: (%s):%d", c, i)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (cs *ChainStore) WaitForMessage(ctx context.Context, mcid cid.Cid) (cid.Cid, *types.MessageReceipt, error) {
	tsub := cs.SubHeadChanges(ctx)

	head := cs.GetHeaviestTipSet()

	bc, r, err := cs.tipsetContainsMsg(head, mcid)
	if err != nil {
		return cid.Undef, nil, err
	}

	if r != nil {
		return bc, r, nil
	}

	for {
		select {
		case val, ok := <-tsub:
			if !ok {
				return cid.Undef, nil, ctx.Err()
			}
			switch val.Type {
			case HCRevert:
				continue
			case HCApply:
				bc, r, err := cs.tipsetContainsMsg(val.Val, mcid)
				if err != nil {
					return cid.Undef, nil, err
				}
				if r != nil {
					return bc, r, nil
				}
			}
		case <-ctx.Done():
			return cid.Undef, nil, ctx.Err()
		}
	}
}

func (cs *ChainStore) tipsetContainsMsg(ts *types.TipSet, msg cid.Cid) (cid.Cid, *types.MessageReceipt, error) {
	for _, b := range ts.Blocks() {
		r, err := cs.blockContainsMsg(b, msg)
		if err != nil {
			return cid.Undef, nil, err
		}
		if r != nil {
			return b.Cid(), r, nil
		}
	}
	return cid.Undef, nil, nil
}

func (cs *ChainStore) blockContainsMsg(blk *types.BlockHeader, msg cid.Cid) (*types.MessageReceipt, error) {
	cst := hamt.CSTFromBstore(cs.bs)
	var msgmeta types.MsgMeta
	if err := cst.Get(context.TODO(), blk.Messages, &msgmeta); err != nil {
		return nil, err
	}

	blscids, err := cs.readAMTCids(msgmeta.BlsMessages)
	if err != nil {
		return nil, errors.Wrap(err, "loading bls message cids for block")
	}

	for i, c := range blscids {
		if c == msg {
			return cs.GetReceipt(blk, i)
		}
	}

	secpkcids, err := cs.readAMTCids(msgmeta.SecpkMessages)
	if err != nil {
		return nil, errors.Wrap(err, "loading secpk message cids for block")
	}

	for i, c := range secpkcids {
		if c == msg {
			return cs.GetReceipt(blk, i+len(blscids))
		}
	}

	return nil, nil
}

func (cs *ChainStore) Blockstore() blockstore.Blockstore {
	return cs.bs
}

func (cs *ChainStore) TryFillTipSet(ts *types.TipSet) (*FullTipSet, error) {
	var out []*types.FullBlock

	for _, b := range ts.Blocks() {
		bmsgs, smsgs, err := cs.MessagesForBlock(b)
		if err != nil {
			// TODO: check for 'not found' errors, and only return nil if this
			// is actually a 'not found' error
			return nil, nil
		}

		fb := &types.FullBlock{
			Header:        b,
			BlsMessages:   bmsgs,
			SecpkMessages: smsgs,
		}

		out = append(out, fb)
	}
	return NewFullTipSet(out), nil
}

func (cs *ChainStore) GetRandomness(ctx context.Context, pts *types.TipSet, tickets []*types.Ticket, lb int) ([]byte, error) {
	if lb < len(tickets) {
		log.Warn("self sampling randomness. this should be extremely rare, if you see this often it may be a bug")

		t := tickets[len(tickets)-(1+lb)]

		return t.VDFResult, nil
	}

	nv := lb - len(tickets)

	nextCids := pts.Cids()
	for {
		nts, err := cs.LoadTipSet(nextCids)
		if err != nil {
			return nil, err
		}

		mtb := nts.MinTicketBlock()
		if nv < len(mtb.Tickets) {
			t := mtb.Tickets[len(mtb.Tickets)-(1+nv)]
			return t.VDFResult, nil
		}

		nv -= len(mtb.Tickets)

		// special case for lookback behind genesis block
		// TODO(spec): this is not in the spec, need to sync that
		if mtb.Height == 0 {

			t := mtb.Tickets[0]

			rval := t.VDFResult
			for i := 0; i < nv; i++ {
				h := sha256.Sum256(rval)
				rval = h[:]
			}
			return rval, nil
		}

		nextCids = mtb.Parents
	}
}
