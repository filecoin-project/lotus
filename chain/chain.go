package chain

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-lotus/chain/address"

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

type GenesisBootstrap struct {
	Genesis  *BlockHeader
	MinerKey address.Address
}

func SetupInitActor(bs bstore.Blockstore, addrs []address.Address) (*Actor, error) {
	var ias InitActorState
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

	act := &Actor{
		Code: InitActorCodeCid,
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

func MakeInitialStateTree(bs bstore.Blockstore, actors map[address.Address]BigInt) (*StateTree, error) {
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
	for a := range actors {
		addrs = append(addrs, a)
	}

	initact, err := SetupInitActor(bs, addrs)
	if err != nil {
		return nil, err
	}

	if err := state.SetActor(InitActorAddress, initact); err != nil {
		return nil, err
	}

	/*
		smact, err := SetupStorageMarketActor(bs)
		if err != nil {
			return nil, err
		}

		if err := state.SetActor(StorageMarketAddress, smact); err != nil {
			return nil, err
		}
	*/

	err = state.SetActor(NetworkAddress, &Actor{
		Code:    AccountActorCodeCid,
		Balance: NewInt(100000000000),
		Head:    emptyobject,
	})
	if err != nil {
		return nil, err
	}

	for a, v := range actors {
		err = state.SetActor(a, &Actor{
			Code:    AccountActorCodeCid,
			Balance: v,
			Head:    emptyobject,
		})
		if err != nil {
			return nil, err
		}
	}

	return state, nil
}

/*
func SetupStorageMarketActor(bs bstore.Blockstore) (*Actor, error) {
	sms := &StorageMarketState{
		Miners:       make(map[address.Address]struct{}),
		TotalStorage: NewInt(0),
	}

	stcid, err := hamt.CSTFromBstore(bs).Put(context.TODO(), sms)
	if err != nil {
		return nil, err
	}

	return &Actor{
		Code:    StorageMarketActorCodeCid,
		Head:    stcid,
		Nonce:   0,
		Balance: NewInt(0),
	}, nil
}

func MakeGenesisBlock(bs bstore.Blockstore, w *Wallet) (*GenesisBootstrap, error) {
	fmt.Println("at end of make Genesis block")

	minerAddr, err := w.GenerateKey(KTSecp256k1)
	if err != nil {
		return nil, err
	}

	addrs := map[address.Address]BigInt{
		minerAddr: NewInt(50000000),
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
		Miner:           InitActorAddress,
		Tickets:         []Ticket{},
		ElectionProof:   []byte("the Genesis block"),
		Parents:         []cid.Cid{},
		Height:          0,
		ParentWeight:    NewInt(0),
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
*/

type ChainStore struct {
	bs bstore.Blockstore
	ds dstore.Datastore

	heaviestLk sync.Mutex
	heaviest   *TipSet

	bestTips *pubsub.PubSub

	headChange func(rev, app []*TipSet) error
}

func NewChainStore(bs bstore.Blockstore, ds dstore.Batching) *ChainStore {
	return &ChainStore{
		bs:       bs,
		ds:       ds,
		bestTips: pubsub.New(64),
	}
}

func (cs *ChainStore) SubNewTips() chan interface{} {
	return cs.bestTips.Sub("best")
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
		cs.headChange(revert, apply)
		log.Infof("New heaviest tipset! %s", ts.Cids())
		cs.heaviest = ts
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

func (cs *ChainStore) MessagesForBlock(b *BlockHeader) ([]*SignedMessage, error) {
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

	return cs.LoadMessagesFromCids(cids)
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
