package store

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/vm"
	"go.opencensus.io/trace"
	"go.uber.org/multierr"

	amt "github.com/filecoin-project/go-amt-ipld"
	"github.com/filecoin-project/lotus/chain/types"

	lru "github.com/hashicorp/golang-lru"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
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

	reorgCh          chan<- reorg
	headChangeNotifs []func(rev, app []*types.TipSet) error

	mmCache *lru.ARCCache
	tsCache *lru.ARCCache

	vmcalls *types.VMSyscalls
}

func NewChainStore(bs bstore.Blockstore, ds dstore.Batching, vmcalls *types.VMSyscalls) *ChainStore {
	c, _ := lru.NewARC(2048)
	tsc, _ := lru.NewARC(4096)
	cs := &ChainStore{
		bs:       bs,
		ds:       ds,
		bestTips: pubsub.New(64),
		tipsets:  make(map[uint64][]cid.Cid),
		mmCache:  c,
		tsCache:  tsc,
		vmcalls:  vmcalls,
	}

	cs.reorgCh = cs.reorgWorker(context.TODO())

	hcnf := func(rev, app []*types.TipSet) error {
		cs.pubLk.Lock()
		defer cs.pubLk.Unlock()

		notif := make([]*HeadChange, len(rev)+len(app))

		for i, r := range rev {
			notif[i] = &HeadChange{
				Type: HCRevert,
				Val:  r,
			}
		}
		for i, r := range app {
			notif[i+len(rev)] = &HeadChange{
				Type: HCApply,
				Val:  r,
			}
		}

		cs.bestTips.Pub(notif, "headchange")
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
		return xerrors.Errorf("failed to load chain state from datastore: %w", err)
	}

	var tscids []cid.Cid
	if err := json.Unmarshal(head, &tscids); err != nil {
		return xerrors.Errorf("failed to unmarshal stored chain head: %w", err)
	}

	ts, err := cs.LoadTipSet(types.NewTipSetKey(tscids...))
	if err != nil {
		return xerrors.Errorf("loading tipset: %w", err)
	}

	cs.heaviest = ts

	return nil
}

func (cs *ChainStore) writeHead(ts *types.TipSet) error {
	data, err := json.Marshal(ts.Cids())
	if err != nil {
		return xerrors.Errorf("failed to marshal tipset: %w", err)
	}

	if err := cs.ds.Put(chainHeadKey, data); err != nil {
		return xerrors.Errorf("failed to write chain head to datastore: %w", err)
	}

	return nil
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

func (cs *ChainStore) SubHeadChanges(ctx context.Context) chan []*HeadChange {
	cs.pubLk.Lock()
	subch := cs.bestTips.Sub("headchange")
	head := cs.GetHeaviestTipSet()
	cs.pubLk.Unlock()

	out := make(chan []*HeadChange, 16)
	out <- []*HeadChange{{
		Type: HCCurrent,
		Val:  head,
	}}

	go func() {
		defer close(out)
		var unsubOnce sync.Once

		for {
			select {
			case val, ok := <-subch:
				if !ok {
					log.Warn("chain head sub exit loop")
					return
				}
				if len(out) > 0 {
					log.Warnf("head change sub is slow, has %d buffered entries", len(out))
				}
				select {
				case out <- val.([]*HeadChange):
				case <-ctx.Done():
				}
			case <-ctx.Done():
				unsubOnce.Do(func() {
					go cs.bestTips.Unsub(subch)
				})
			}
		}
	}()
	return out
}

func (cs *ChainStore) SubscribeHeadChanges(f func(rev, app []*types.TipSet) error) {
	cs.headChangeNotifs = append(cs.headChangeNotifs, f)
}

func (cs *ChainStore) SetGenesis(b *types.BlockHeader) error {
	ts, err := types.NewTipSet([]*types.BlockHeader{b})
	if err != nil {
		return err
	}

	if err := cs.PutTipSet(context.TODO(), ts); err != nil {
		return err
	}

	return cs.ds.Put(dstore.NewKey("0"), b.Cid().Bytes())
}

func (cs *ChainStore) PutTipSet(ctx context.Context, ts *types.TipSet) error {
	for _, b := range ts.Blocks() {
		if err := cs.PersistBlockHeaders(b); err != nil {
			return err
		}
	}

	expanded, err := cs.expandTipset(ts.Blocks()[0])
	if err != nil {
		return xerrors.Errorf("errored while expanding tipset: %w", err)
	}
	log.Debugf("expanded %s into %s\n", ts.Cids(), expanded.Cids())

	if err := cs.MaybeTakeHeavierTipSet(ctx, expanded); err != nil {
		return xerrors.Errorf("MaybeTakeHeavierTipSet failed in PutTipSet: %w", err)
	}
	return nil
}

func (cs *ChainStore) MaybeTakeHeavierTipSet(ctx context.Context, ts *types.TipSet) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	w, err := cs.Weight(ctx, ts)
	if err != nil {
		return err
	}
	heaviestW, err := cs.Weight(ctx, cs.heaviest)
	if err != nil {
		return err
	}

	if w.GreaterThan(heaviestW) {
		// TODO: don't do this for initial sync. Now that we don't have a
		// difference between 'bootstrap sync' and 'caught up' sync, we need
		// some other heuristic.
		return cs.takeHeaviestTipSet(ctx, ts)
	}
	return nil
}

type reorg struct {
	old *types.TipSet
	new *types.TipSet
}

func (cs *ChainStore) reorgWorker(ctx context.Context) chan<- reorg {
	out := make(chan reorg, 32)
	go func() {
		defer log.Warn("reorgWorker quit")

		for {
			select {
			case r := <-out:
				revert, apply, err := cs.ReorgOps(r.old, r.new)
				if err != nil {
					log.Error("computing reorg ops failed: ", err)
					continue
				}

				// reverse the apply array
				for i := len(apply)/2 - 1; i >= 0; i-- {
					opp := len(apply) - 1 - i
					apply[i], apply[opp] = apply[opp], apply[i]
				}

				for _, hcf := range cs.headChangeNotifs {
					if err := hcf(revert, apply); err != nil {
						log.Error("head change func errored (BAD): ", err)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func (cs *ChainStore) takeHeaviestTipSet(ctx context.Context, ts *types.TipSet) error {
	_, span := trace.StartSpan(ctx, "takeHeaviestTipSet")
	defer span.End()

	if cs.heaviest != nil { // buf
		if len(cs.reorgCh) > 0 {
			log.Warnf("Reorg channel running behind, %d reorgs buffered", len(cs.reorgCh))
		}
		cs.reorgCh <- reorg{
			old: cs.heaviest,
			new: ts,
		}
	} else {
		log.Warnf("no heaviest tipset found, using %s", ts.Cids())
	}

	span.AddAttributes(trace.BoolAttribute("newHead", true))

	log.Infof("New heaviest tipset! %s (height=%d)", ts.Cids(), ts.Height())
	cs.heaviest = ts

	if err := cs.writeHead(ts); err != nil {
		log.Errorf("failed to write chain head: %s", err)
		return nil
	}

	return nil
}

// SetHead sets the chainstores current 'best' head node.
// This should only be called if something is broken and needs fixing
func (cs *ChainStore) SetHead(ts *types.TipSet) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	return cs.takeHeaviestTipSet(context.TODO(), ts)
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

func (cs *ChainStore) LoadTipSet(tsk types.TipSetKey) (*types.TipSet, error) {
	v, ok := cs.tsCache.Get(tsk)
	if ok {
		return v.(*types.TipSet), nil
	}

	var blks []*types.BlockHeader
	for _, c := range tsk.Cids() {
		b, err := cs.GetBlock(c)
		if err != nil {
			return nil, xerrors.Errorf("get block %s: %w", c, err)
		}

		blks = append(blks, b)
	}

	ts, err := types.NewTipSet(blks)
	if err != nil {
		return nil, err
	}

	cs.tsCache.Add(tsk, ts)

	return ts, nil
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

func (cs *ChainStore) GetHeaviestTipSet() *types.TipSet {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	return cs.heaviest
}

func (cs *ChainStore) AddToTipSetTracker(b *types.BlockHeader) error {
	cs.tstLk.Lock()
	defer cs.tstLk.Unlock()

	tss := cs.tipsets[b.Height]
	for _, oc := range tss {
		if oc == b.Cid() {
			log.Debug("tried to add block to tipset tracker that was already there")
			return nil
		}
	}

	cs.tipsets[b.Height] = append(tss, b.Cid())

	// TODO: do we want to look for slashable submissions here? might as well...

	return nil
}

func (cs *ChainStore) PersistBlockHeaders(b ...*types.BlockHeader) error {
	sbs := make([]block.Block, len(b))

	for i, header := range b {
		var err error
		sbs[i], err = header.ToStorageBlock()
		if err != nil {
			return err
		}
	}

	batchSize := 256
	calls := len(b) / batchSize

	var err error
	for i := 0; i <= calls; i++ {
		start := batchSize * i
		end := start + batchSize
		if end > len(b) {
			end = len(b)
		}

		err = multierr.Append(err, cs.bs.PutMany(sbs[start:end]))
	}

	return err
}

type storable interface {
	ToStorageBlock() (block.Block, error)
}

func PutMessage(bs bstore.Blockstore, m storable) (cid.Cid, error) {
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

	inclMiners := map[address.Address]bool{b.Miner: true}
	for _, bhc := range tsets {
		if bhc == b.Cid() {
			continue
		}

		h, err := cs.GetBlock(bhc)
		if err != nil {
			return nil, xerrors.Errorf("failed to load block (%s) for tipset expansion: %w", bhc, err)
		}

		if inclMiners[h.Miner] {
			log.Warnf("Have multiple blocks from miner %s at height %d in our tipset cache", h.Miner, h.Height)
			continue
		}

		if types.CidArrsEqual(h.Parents, b.Parents) {
			all = append(all, h)
			inclMiners[h.Miner] = true
		}
	}

	// TODO: other validation...?

	return types.NewTipSet(all)
}

func (cs *ChainStore) AddBlock(ctx context.Context, b *types.BlockHeader) error {
	if err := cs.PersistBlockHeaders(b); err != nil {
		return err
	}

	ts, err := cs.expandTipset(b)
	if err != nil {
		return err
	}

	if err := cs.MaybeTakeHeavierTipSet(ctx, ts); err != nil {
		return xerrors.Errorf("MaybeTakeHeavierTipSet failed: %w", err)
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

func (cs *ChainStore) GetCMessage(c cid.Cid) (ChainMsg, error) {
	m, err := cs.GetMessage(c)
	if err == nil {
		return m, nil
	}
	if err != bstore.ErrNotFound {
		log.Warn("GetCMessage: unexpected error getting unsigned message: %s", err)
	}

	return cs.GetSignedMessage(c)
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

type ChainMsg interface {
	Cid() cid.Cid
	VMMessage() *types.Message
	ToStorageBlock() (block.Block, error)
}

func (cs *ChainStore) MessagesForTipset(ts *types.TipSet) ([]ChainMsg, error) {
	applied := make(map[address.Address]uint64)
	balances := make(map[address.Address]types.BigInt)

	cst := hamt.CSTFromBstore(cs.bs)
	st, err := state.LoadStateTree(cst, ts.Blocks()[0].ParentStateRoot)
	if err != nil {
		return nil, xerrors.Errorf("failed to load state tree")
	}

	preloadAddr := func(a address.Address) error {
		if _, ok := applied[a]; !ok {
			act, err := st.GetActor(a)
			if err != nil {
				return err
			}

			applied[a] = act.Nonce
			balances[a] = act.Balance
		}
		return nil
	}

	var out []ChainMsg
	for _, b := range ts.Blocks() {
		bms, sms, err := cs.MessagesForBlock(b)
		if err != nil {
			return nil, xerrors.Errorf("failed to get messages for block: %w", err)
		}

		cmsgs := make([]ChainMsg, 0, len(bms)+len(sms))
		for _, m := range bms {
			cmsgs = append(cmsgs, m)
		}
		for _, sm := range sms {
			cmsgs = append(cmsgs, sm)
		}

		for _, cm := range cmsgs {
			m := cm.VMMessage()
			if err := preloadAddr(m.From); err != nil {
				return nil, err
			}

			if applied[m.From] != m.Nonce {
				continue
			}
			applied[m.From]++

			if balances[m.From].LessThan(m.RequiredFunds()) {
				continue
			}
			balances[m.From] = types.BigSub(balances[m.From], m.RequiredFunds())

			out = append(out, cm)
		}
	}

	return out, nil
}

type mmCids struct {
	bls   []cid.Cid
	secpk []cid.Cid
}

func (cs *ChainStore) readMsgMetaCids(mmc cid.Cid) ([]cid.Cid, []cid.Cid, error) {
	o, ok := cs.mmCache.Get(mmc)
	if ok {
		mmcids := o.(*mmCids)
		return mmcids.bls, mmcids.secpk, nil
	}

	cst := hamt.CSTFromBstore(cs.bs)
	var msgmeta types.MsgMeta
	if err := cst.Get(context.TODO(), mmc, &msgmeta); err != nil {
		return nil, nil, xerrors.Errorf("failed to load msgmeta: %w", err)
	}

	blscids, err := cs.readAMTCids(msgmeta.BlsMessages)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading bls message cids for block: %w", err)
	}

	secpkcids, err := cs.readAMTCids(msgmeta.SecpkMessages)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading secpk message cids for block: %w", err)
	}

	cs.mmCache.Add(mmc, &mmCids{
		bls:   blscids,
		secpk: secpkcids,
	})

	return blscids, secpkcids, nil
}

func (cs *ChainStore) GetPath(ctx context.Context, from types.TipSetKey, to types.TipSetKey) ([]*HeadChange, error) {
	fts, err := cs.LoadTipSet(from)
	if err != nil {
		return nil, xerrors.Errorf("loading from tipset %s: %w", from, err)
	}
	tts, err := cs.LoadTipSet(to)
	if err != nil {
		return nil, xerrors.Errorf("loading to tipset %s: %w", to, err)
	}
	revert, apply, err := cs.ReorgOps(fts, tts)
	if err != nil {
		return nil, xerrors.Errorf("error getting tipset branches: %w", err)
	}

	path := make([]*HeadChange, len(revert)+len(apply))
	for i, r := range revert {
		path[i] = &HeadChange{Type: HCRevert, Val: r}
	}
	for j, i := 0, len(apply)-1; i >= 0; j, i = j+1, i-1 {
		path[j+len(revert)] = &HeadChange{Type: HCApply, Val: apply[i]}
	}
	return path, nil
}

func (cs *ChainStore) MessagesForBlock(b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	blscids, secpkcids, err := cs.readMsgMetaCids(b.Messages)
	if err != nil {
		return nil, nil, err
	}

	blsmsgs, err := cs.LoadMessagesFromCids(blscids)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading bls messages for block: %w", err)
	}

	secpkmsgs, err := cs.LoadSignedMessagesFromCids(secpkcids)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading secpk messages for block: %w", err)
	}

	return blsmsgs, secpkmsgs, nil
}

func (cs *ChainStore) GetParentReceipt(b *types.BlockHeader, i int) (*types.MessageReceipt, error) {
	bs := amt.WrapBlockstore(cs.bs)
	a, err := amt.LoadAMT(bs, b.ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
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
			return nil, xerrors.Errorf("failed to get message: (%s):%d: %w", err, c, i)
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
			return nil, xerrors.Errorf("failed to get message: (%s):%d: %w", err, c, i)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (cs *ChainStore) Blockstore() bstore.Blockstore {
	return cs.bs
}

func (cs *ChainStore) VMSys() *types.VMSyscalls {
	return cs.vmcalls
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

func drawRandomness(t *types.Ticket, round int64) []byte {
	h := sha256.New()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(round))

	h.Write(t.VRFProof)
	h.Write(buf[:])

	return h.Sum(nil)
}

func (cs *ChainStore) GetRandomness(ctx context.Context, blks []cid.Cid, round int64) ([]byte, error) {
	_, span := trace.StartSpan(ctx, "store.GetRandomness")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("round", round))

	for {
		nts, err := cs.LoadTipSet(types.NewTipSetKey(blks...))
		if err != nil {
			return nil, err
		}

		mtb := nts.MinTicketBlock()

		if int64(nts.Height()) <= round {
			return drawRandomness(nts.MinTicketBlock().Ticket, round), nil
		}

		// special case for lookback behind genesis block
		// TODO(spec): this is not in the spec, need to sync that
		if mtb.Height == 0 {

			// round is negative
			thash := drawRandomness(mtb.Ticket, round*-1)

			// for negative lookbacks, just use the hash of the positive tickethash value
			h := sha256.Sum256(thash)
			return h[:], nil
		}

		blks = mtb.Parents
	}
}

func (cs *ChainStore) GetTipsetByHeight(ctx context.Context, h uint64, ts *types.TipSet) (*types.TipSet, error) {
	if ts == nil {
		ts = cs.GetHeaviestTipSet()
	}

	if h > ts.Height() {
		return nil, xerrors.Errorf("looking for tipset with height less than start point")
	}

	if ts.Height()-h > build.ForkLengthThreshold {
		log.Warnf("expensive call to GetTipsetByHeight, seeking %d levels", ts.Height()-h)
	}

	for {
		pts, err := cs.LoadTipSet(ts.Parents())
		if err != nil {
			return nil, err
		}

		if h > pts.Height() {
			return ts, nil
		}

		ts = pts
	}
}

type chainRand struct {
	cs   *ChainStore
	blks []cid.Cid
	bh   uint64
}

func NewChainRand(cs *ChainStore, blks []cid.Cid, bheight uint64) vm.Rand {
	return &chainRand{
		cs:   cs,
		blks: blks,
		bh:   bheight,
	}
}

func (cr *chainRand) GetRandomness(ctx context.Context, round int64) ([]byte, error) {
	return cr.cs.GetRandomness(ctx, cr.blks, round)
}
