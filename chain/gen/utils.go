package gen

import (
	"context"
	"fmt"

	actors "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	peer "github.com/libp2p/go-libp2p-peer"
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

type GenMinerCfg struct {
	Owner  address.Address
	Worker address.Address

	// not quite generating real sectors yet, but this will be necessary
	//SectorDir string

	// The address of the created miner, this is set by the genesis setup
	MinerAddr address.Address

	PeerID peer.ID
}

func mustEnc(i interface{}) []byte {
	enc, err := actors.SerializeParams(i)
	if err != nil {
		panic(err)
	}
	return enc
}

func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sroot cid.Cid, gmcfg *GenMinerCfg) (cid.Cid, error) {
	vm, err := vm.NewVM(sroot, 0, actors.NetworkAddress, cs)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create NewVM: %w", err)
	}

	params := mustEnc(actors.CreateStorageMinerParams{
		Owner:      gmcfg.Owner,
		Worker:     gmcfg.Worker,
		SectorSize: types.NewInt(1024),
		PeerID:     gmcfg.PeerID,
	})

	rval, err := doExec(ctx, vm, actors.StorageMarketAddress, gmcfg.Owner, actors.SMAMethods.CreateStorageMiner, params)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
	}

	maddr, err := address.NewFromBytes(rval)
	if err != nil {
		return cid.Undef, err
	}

	gmcfg.MinerAddr = maddr

	params = mustEnc(actors.UpdateStorageParams{Delta: types.NewInt(5000)})

	_, err = doExec(ctx, vm, actors.StorageMarketAddress, maddr, actors.SMAMethods.UpdateStorage, params)

	// UGLY HACKY MODIFICATION OF MINER POWER

	// we have to flush the vm here because it buffers stuff internally for perf reasons
	if _, err := vm.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("vm.Flush failed: %w", err)
	}

	st := vm.StateTree()
	mact, err := st.GetActor(maddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("get miner actor failed: %w", err)
	}

	cst := hamt.CSTFromBstore(cs.Blockstore())
	var mstate actors.StorageMinerActorState
	if err := cst.Get(ctx, mact.Head, &mstate); err != nil {
		return cid.Undef, xerrors.Errorf("getting miner actor state failed: %w", err)
	}
	mstate.Power = types.NewInt(5000)

	nstate, err := cst.Put(ctx, mstate)
	if err != nil {
		return cid.Undef, err
	}

	mact.Head = nstate
	if err := st.SetActor(maddr, mact); err != nil {
		return cid.Undef, err
	}
	// End of super haxx

	return vm.Flush(ctx)
}

func doExec(ctx context.Context, vm *vm.VM, to, from address.Address, method uint64, params []byte) ([]byte, error) {
	ret, err := vm.ApplyMessage(context.TODO(), &types.Message{
		To:       to,
		From:     from,
		Method:   method,
		Params:   params,
		GasLimit: types.NewInt(1000000),
		GasPrice: types.NewInt(0),
		Value:    types.NewInt(0),
	})
	if err != nil {
		return nil, err
	}

	if ret.ExitCode != 0 {
		return nil, fmt.Errorf("failed to call method: %s", ret.ActorErr)
	}

	return ret.Return, nil
}

func MakeGenesisBlock(bs bstore.Blockstore, balances map[address.Address]types.BigInt, gmcfg *GenMinerCfg) (*GenesisBootstrap, error) {
	ctx := context.Background()

	state, err := MakeInitialStateTree(bs, balances)
	if err != nil {
		return nil, xerrors.Errorf("make initial state tree failed: %w", err)
	}

	stateroot, err := state.Flush()
	if err != nil {
		return nil, xerrors.Errorf("flush state tree failed: %w", err)
	}

	// temp chainstore
	cs := store.NewChainStore(bs, datastore.NewMapDatastore())
	stateroot, err = SetupStorageMiners(ctx, cs, stateroot, gmcfg)
	if err != nil {
		return nil, xerrors.Errorf("setup storage miners failed: %w", err)
	}

	cst := hamt.CSTFromBstore(bs)

	emptyroot, err := sharray.Build(context.TODO(), 4, []interface{}{}, cst)
	if err != nil {
		return nil, err
	}
	mmcid, err := cst.Put(context.TODO(), &types.MsgMeta{
		BlsMessages:   emptyroot,
		SecpkMessages: emptyroot,
	})
	if err != nil {
		return nil, err
	}

	fmt.Println("Empty Genesis root: ", emptyroot)

	genesisticket := &types.Ticket{
		VRFProof:  []byte("vrf proof"),
		VDFResult: []byte("i am a vdf result"),
		VDFProof:  []byte("vdf proof"),
	}

	b := &types.BlockHeader{
		Miner:           actors.InitActorAddress,
		Tickets:         []*types.Ticket{genesisticket},
		ElectionProof:   []byte("the Genesis block"),
		Parents:         []cid.Cid{},
		Height:          0,
		ParentWeight:    types.NewInt(0),
		StateRoot:       stateroot,
		Messages:        mmcid,
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
