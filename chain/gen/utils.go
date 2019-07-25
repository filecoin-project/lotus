package gen

import (
	"context"
	"fmt"

	actors "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/types"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	sharray "github.com/whyrusleeping/sharray"
)

type GenesisBootstrap struct {
	Genesis *types.BlockHeader
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

func MakeInitialStateTree(bs bstore.Blockstore, actmap map[address.Address]types.BigInt) (*state.StateTree, error) {
	cst := hamt.CSTFromBstore(bs)
	state, err := state.NewStateTree(cst)
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

func MakeGenesisBlock(bs bstore.Blockstore, balances map[address.Address]types.BigInt) (*GenesisBootstrap, error) {
	fmt.Println("at end of make Genesis block")

	state, err := MakeInitialStateTree(bs, balances)
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

	b := &types.BlockHeader{
		Miner:           actors.InitActorAddress,
		Tickets:         []types.Ticket{},
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
		Genesis: b,
	}, nil
}
