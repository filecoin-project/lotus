package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	actors "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/pkg/errors"
	pubsub "github.com/whyrusleeping/pubsub"
	sharray "github.com/whyrusleeping/sharray"
)

const ForkLengthThreshold = 20

var log = logging.Logger("f2")

var chainHeadKey = dstore.NewKey("head")

type GenesisBootstrap struct {
	Genesis  *BlockHeader
	MinerKey address.Address
}

func SetupInitActor(bs bstore.Blockstore, addrs []address.Address) (*types.Actor, error) {
	var ias actors.InitActorState
	ias.NextID = 100

	cst := hamt.CSTFromBstore(bs)
	amap := hamt.NewNode(cst)

	for i, a := range addrs {
		if err := amap.Set(context.TODO(), string(a.Bytes()), 100+uint64(i)); err != nil {
			return nil, err
		}
	}

	ias.NextID += uint64(len(addrs))
	if err := amap.Flush(context.TODO()); err != nil {
		return nil, err
	}
	amapcid, err := cst.Put(context.TODO(), amap)
	if err != nil {
		return nil, err
	}

	ias.AddressMap = amapcid

	statecid, err := cst.Put(context.TODO(), &ias)
	if err != nil {
		return nil, err
	}

	act := &types.Actor{
		Code: actors.InitActorCodeCid,
		Head: statecid,
	}

	return act, nil
}

func init() {
	bs := bstore.NewBlockstore(dstore.NewMapDatastore())
	cst := hamt.CSTFromBstore(bs)
	emptyobject, err := cst.Put(context.TODO(), map[string]string{})
	if err != nil {
		panic(err)
	}

	EmptyObjectCid = emptyobject
}

var EmptyObjectCid cid.Cid

func MakeInitialStateTree(bs bstore.Blockstore, actmap map[address.Address]types.BigInt) (*StateTree, error) {
	cst := hamt.CSTFromBstore(bs)
	state, err := NewStateTree(cst)
	if err != nil {
		return nil, err
	}

	emptyobject, err := cst.Put(context.TODO(), map[string]string{})
	if err != nil {
		return nil, err
	}

	var addrs []address.Address
	for a := range actmap {
		addrs = append(addrs, a)
	}

	initact, err := SetupInitActor(bs, addrs)
	if err != nil {
		return nil, err
	}

	if err := state.SetActor(actors.InitActorAddress, initact); err != nil {
		return nil, err
	}

	smact, err := SetupStorageMarketActor(bs)
	if err != nil {
		return nil, err
	}

	if err := state.SetActor(actors.StorageMarketAddress, smact); err != nil {
		return nil, err
	}

	err = state.SetActor(actors.NetworkAddress, &types.Actor{
		Code:    actors.AccountActorCodeCid,
		Balance: types.NewInt(100000000000),
		Head:    emptyobject,
	})
	if err != nil {
		return nil, err
	}

	for a, v := range actmap {
		err = state.SetActor(a, &types.Actor{
			Code:    actors.AccountActorCodeCid,
			Balance: v,
			Head:    emptyobject,
		})
		if err != nil {
			return nil, err
		}
	}

	return state, nil
}

func SetupStorageMarketActor(bs bstore.Blockstore) (*types.Actor, error) {
	sms := &actors.StorageMarketState{
		Miners:       make(map[address.Address]struct{}),
		TotalStorage: types.NewInt(0),
	}

	stcid, err := hamt.CSTFromBstore(bs).Put(context.TODO(), sms)
	if err != nil {
		return nil, err
	}

	return &types.Actor{
		Code:    actors.StorageMarketActorCodeCid,
		Head:    stcid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}, nil
}

func MakeGenesisBlock(bs bstore.Blockstore, w *Wallet) (*GenesisBootstrap, error) {
	fmt.Println("at end of make Genesis block")

	minerAddr, err := w.GenerateKey(KTSecp256k1)
	if err != nil {
		return nil, err
	}

	addrs := map[address.Address]types.BigInt{
		minerAddr: types.NewInt(50000000),
	}

	state, err := MakeInitialStateTree(bs, addrs)
	if err != nil {
		return nil, err
	}

	stateroot, err := state.Flush()
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(bs)
	emptyroot, err := sharray.Build(context.TODO(), 4, []interface{}{}, cst)
	if err != nil {
		return nil, err
	}
	fmt.Println("Empty Genesis root: ", emptyroot)

	b := &BlockHeader{
		Miner:           actors.InitActorAddress,
		Tickets:         []Ticket{},
		ElectionProof:   []byte("the Genesis block"),
		Parents:         []cid.Cid{},
		Height:          0,
		ParentWeight:    types.NewInt(0),
		StateRoot:       stateroot,
		Messages:        emptyroot,
		MessageReceipts: emptyroot,
	}

	sb, err := b.ToStorageBlock()
	if err != nil {
		return nil, err
	}

	if err := bs.Put(sb); err != nil {
		return nil, err
	}

	return &GenesisBootstrap{
		Genesis:  b,
		MinerKey: minerAddr,
	}, nil
}

type ChainStore struct {
	bs bstore.Blockstore
	ds dstore.Datastore

	heaviestLk sync.Mutex
	heaviest   *TipSet

	bestTips *pubsub.PubSub

	headChangeNotifs []func(rev, app []*TipSet) error
}

func NewChainStore(bs bstore.Blockstore, ds dstore.Batching) *ChainStore {
	return &ChainStore{
		bs:       bs,
		ds:       ds,
		bestTips: pubsub.New(64),
	}
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

func (cs *ChainStore) writeHead(ts *TipSet) error {
	data, err := json.Marshal(ts.Cids())
	if err != nil {
		return errors.Wrap(err, "failed to marshal tipset")
	}

	if err := cs.ds.Put(chainHeadKey, data); err != nil {
		return errors.Wrap(err, "failed to write chain head to datastore")
	}

	return nil
}

func (cs *ChainStore) SubNewTips() chan *TipSet {
	subch := cs.bestTips.Sub("best")
	out := make(chan *TipSet)
	go func() {
		defer close(out)
		for val := range subch {
			out <- val.(*TipSet)
		}
	}()
	return out
}

const (
	HCRevert = "revert"
	HCApply  = "apply"
)

type HeadChange struct {
	Type string
	Val  *TipSet
}

func (cs *ChainStore) SubHeadChanges() chan *HeadChange {
	subch := cs.bestTips.Sub("headchange")
	out := make(chan *HeadChange, 16)
	go func() {
		defer close(out)
		for val := range subch {
			out <- val.(*HeadChange)
		}
	}()
	return out
}

func (cs *ChainStore) SubscribeHeadChanges(f func(rev, app []*TipSet) error) {
	cs.headChangeNotifs = append(cs.headChangeNotifs, f)
}

func (cs *ChainStore) SetGenesis(b *BlockHeader) error {
	gents, err := NewTipSet([]*BlockHeader{b})
	if err != nil {
		return err
	}
	fts := &FullTipSet{
		Blocks: []*FullBlock{
			{Header: b},
		},
	}

	cs.heaviest = gents

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

	cs.maybeTakeHeavierTipSet(ts.TipSet())
	return nil
}

func (cs *ChainStore) maybeTakeHeavierTipSet(ts *TipSet) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	if cs.heaviest == nil || cs.Weight(ts) > cs.Weight(cs.heaviest) {
		revert, apply, err := cs.ReorgOps(cs.heaviest, ts)
		if err != nil {
			return err
		}
		for _, hcf := range cs.headChangeNotifs {
			hcf(revert, apply)
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

func (cs *ChainStore) Contains(ts *TipSet) (bool, error) {
	for _, c := range ts.cids {
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

func (cs *ChainStore) GetBlock(c cid.Cid) (*BlockHeader, error) {
	sb, err := cs.bs.Get(c)
	if err != nil {
		return nil, err
	}

	return DecodeBlock(sb.RawData())
}

func (cs *ChainStore) LoadTipSet(cids []cid.Cid) (*TipSet, error) {
	var blks []*BlockHeader
	for _, c := range cids {
		b, err := cs.GetBlock(c)
		if err != nil {
			return nil, err
		}

		blks = append(blks, b)
	}

	return NewTipSet(blks)
}

// returns true if 'a' is an ancestor of 'b'
func (cs *ChainStore) IsAncestorOf(a, b *TipSet) (bool, error) {
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

func (cs *ChainStore) NearestCommonAncestor(a, b *TipSet) (*TipSet, error) {
	l, _, err := cs.ReorgOps(a, b)
	if err != nil {
		return nil, err
	}

	return cs.LoadTipSet(l[len(l)-1].Parents())
}

func (cs *ChainStore) ReorgOps(a, b *TipSet) ([]*TipSet, []*TipSet, error) {
	left := a
	right := b

	var leftChain, rightChain []*TipSet
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
				return nil, nil, err
			}

			right = par
		}
	}

	return leftChain, rightChain, nil
}

func (cs *ChainStore) Weight(ts *TipSet) uint64 {
	return ts.Blocks()[0].ParentWeight.Uint64() + uint64(len(ts.Cids()))
}

func (cs *ChainStore) GetHeaviestTipSet() *TipSet {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	return cs.heaviest
}

func (cs *ChainStore) persistBlockHeader(b *BlockHeader) error {
	sb, err := b.ToStorageBlock()
	if err != nil {
		return err
	}

	return cs.bs.Put(sb)
}

func (cs *ChainStore) persistBlock(b *FullBlock) error {
	if err := cs.persistBlockHeader(b.Header); err != nil {
		return err
	}

	for _, m := range b.Messages {
		if err := cs.PutMessage(m); err != nil {
			return err
		}
	}
	return nil
}

func (cs *ChainStore) PutMessage(m *SignedMessage) error {
	sb, err := m.ToStorageBlock()
	if err != nil {
		return err
	}

	return cs.bs.Put(sb)
}

func (cs *ChainStore) AddBlock(b *BlockHeader) error {
	if err := cs.persistBlockHeader(b); err != nil {
		return err
	}

	ts, _ := NewTipSet([]*BlockHeader{b})
	cs.maybeTakeHeavierTipSet(ts)

	return nil
}

func (cs *ChainStore) GetGenesis() (*BlockHeader, error) {
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

	return DecodeBlock(genb.RawData())
}

func (cs *ChainStore) TipSetState(cids []cid.Cid) (cid.Cid, error) {
	ts, err := cs.LoadTipSet(cids)
	if err != nil {
		log.Error("failed loading tipset: ", cids)
		return cid.Undef, err
	}

	if len(ts.Blocks()) == 1 {
		return ts.Blocks()[0].StateRoot, nil
	}

	panic("cant handle multiblock tipsets yet")

}

func (cs *ChainStore) GetMessage(c cid.Cid) (*SignedMessage, error) {
	sb, err := cs.bs.Get(c)
	if err != nil {
		return nil, err
	}

	return DecodeSignedMessage(sb.RawData())
}

func (cs *ChainStore) MessageCidsForBlock(b *BlockHeader) ([]cid.Cid, error) {
	cst := hamt.CSTFromBstore(cs.bs)
	shar, err := sharray.Load(context.TODO(), b.Messages, 4, cst)
	if err != nil {
		return nil, errors.Wrap(err, "sharray load")
	}

	var cids []cid.Cid
	err = shar.ForEach(context.TODO(), func(i interface{}) error {
		c, ok := i.(cid.Cid)
		if !ok {
			return fmt.Errorf("value in message sharray was not a cid")
		}

		cids = append(cids, c)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return cids, nil
}

func (cs *ChainStore) MessagesForBlock(b *BlockHeader) ([]*SignedMessage, error) {
	cids, err := cs.MessageCidsForBlock(b)
	if err != nil {
		return nil, err
	}

	return cs.LoadMessagesFromCids(cids)
}

func (cs *ChainStore) GetReceipt(b *BlockHeader, i int) (*types.MessageReceipt, error) {
	cst := hamt.CSTFromBstore(cs.bs)
	shar, err := sharray.Load(context.TODO(), b.MessageReceipts, 4, cst)
	if err != nil {
		return nil, errors.Wrap(err, "sharray load")
	}

	ival, err := shar.Get(context.TODO(), i)
	if err != nil {
		return nil, err
	}

	// @warpfork, @EricMyhre help me. save me.
	out, err := json.Marshal(ival)
	if err != nil {
		return nil, err
	}
	var r types.MessageReceipt
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

func (cs *ChainStore) LoadMessagesFromCids(cids []cid.Cid) ([]*SignedMessage, error) {
	msgs := make([]*SignedMessage, 0, len(cids))
	for _, c := range cids {
		m, err := cs.GetMessage(c)
		if err != nil {
			return nil, err
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (cs *ChainStore) GetBalance(addr address.Address) (types.BigInt, error) {
	ts := cs.GetHeaviestTipSet()
	stcid, err := cs.TipSetState(ts.Cids())
	if err != nil {
		return types.BigInt{}, err
	}

	cst := hamt.CSTFromBstore(cs.bs)
	state, err := LoadStateTree(cst, stcid)
	if err != nil {
		return types.BigInt{}, err
	}

	act, err := state.GetActor(addr)
	if err != nil {
		return types.BigInt{}, err
	}

	return act.Balance, nil
}

func (cs *ChainStore) WaitForMessage(ctx context.Context, mcid cid.Cid) (cid.Cid, *types.MessageReceipt, error) {
	tsub := cs.SubHeadChanges()

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
		case val := <-tsub:
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

func (cs *ChainStore) tipsetContainsMsg(ts *TipSet, msg cid.Cid) (cid.Cid, *types.MessageReceipt, error) {
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

func (cs *ChainStore) blockContainsMsg(blk *BlockHeader, msg cid.Cid) (*types.MessageReceipt, error) {
	msgs, err := cs.MessageCidsForBlock(blk)
	if err != nil {
		return nil, err
	}

	for i, mc := range msgs {
		if mc == msg {
			return cs.GetReceipt(blk, i)
		}
	}

	return nil, nil
}
