package gen

import (
	"context"
	"fmt"

	amt "github.com/filecoin-project/go-amt-ipld"
	"github.com/filecoin-project/lotus/build"
	actors "github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	peer "github.com/libp2p/go-libp2p-peer"
	cbg "github.com/whyrusleeping/cbor-gen"
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
		return nil, xerrors.Errorf("making new state tree: %w", err)
	}

	emptyobject, err := cst.Put(context.TODO(), map[string]string{})
	if err != nil {
		return nil, xerrors.Errorf("failed putting empty object: %w", err)
	}

	var addrs []address.Address
	for a := range actmap {
		addrs = append(addrs, a)
	}

	initact, err := SetupInitActor(bs, addrs)
	if err != nil {
		return nil, xerrors.Errorf("setup init actor: %w", err)
	}

	if err := state.SetActor(actors.InitActorAddress, initact); err != nil {
		return nil, xerrors.Errorf("set init actor: %w", err)
	}

	smact, err := SetupStorageMarketActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup storage market actor: %w", err)
	}

	if err := state.SetActor(actors.StorageMarketAddress, smact); err != nil {
		return nil, xerrors.Errorf("set storage market actor: %w", err)
	}

	netAmt := types.FromFil(build.TotalFilecoin)
	for _, amt := range actmap {
		netAmt = types.BigSub(netAmt, amt)
	}

	err = state.SetActor(actors.NetworkAddress, &types.Actor{
		Code:    actors.AccountActorCodeCid,
		Balance: netAmt,
		Head:    emptyobject,
	})
	if err != nil {
		return nil, xerrors.Errorf("set network account actor: %w", err)
	}

	err = state.SetActor(actors.BurntFundsAddress, &types.Actor{
		Code:    actors.AccountActorCodeCid,
		Balance: types.NewInt(0),
		Head:    emptyobject,
	})
	if err != nil {
		return nil, xerrors.Errorf("set burnt funds account actor: %w", err)
	}

	for a, v := range actmap {
		err = state.SetActor(a, &types.Actor{
			Code:    actors.AccountActorCodeCid,
			Balance: v,
			Head:    emptyobject,
		})
		if err != nil {
			return nil, xerrors.Errorf("setting account from actmap: %w", err)
		}
	}

	return state, nil
}

func SetupStorageMarketActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := hamt.CSTFromBstore(bs)
	nd := hamt.NewNode(cst)
	emptyhamt, err := cst.Put(context.TODO(), nd)
	if err != nil {
		return nil, err
	}

	sms := &actors.StoragePowerState{
		Miners:       emptyhamt,
		TotalStorage: types.NewInt(0),
	}

	stcid, err := cst.Put(context.TODO(), sms)
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
	Owners  []address.Address
	Workers []address.Address

	// not quite generating real sectors yet, but this will be necessary
	//SectorDir string

	// The addresses of the created miner, this is set by the genesis setup
	MinerAddrs []address.Address

	PeerIDs []peer.ID
}

func mustEnc(i cbg.CBORMarshaler) []byte {
	enc, err := actors.SerializeParams(i)
	if err != nil {
		panic(err)
	}
	return enc
}

func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sroot cid.Cid, gmcfg *GenMinerCfg) (cid.Cid, error) {
	vm, err := vm.NewVM(sroot, 0, nil, actors.NetworkAddress, cs.Blockstore())
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create NewVM: %w", err)
	}

	for i := 0; i < len(gmcfg.Workers); i++ {
		owner := gmcfg.Owners[i]
		worker := gmcfg.Workers[i]
		pid := gmcfg.PeerIDs[i]

		params := mustEnc(&actors.CreateStorageMinerParams{
			Owner:      owner,
			Worker:     worker,
			SectorSize: types.NewInt(build.SectorSize),
			PeerID:     pid,
		})

		// TODO: hardcoding 7000000 here is a little fragile, it changes any
		// time anyone changes the initial account allocations
		rval, err := doExecValue(ctx, vm, actors.StorageMarketAddress, owner, types.FromFil(6500), actors.SPAMethods.CreateStorageMiner, params)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to create genesis miner: %w", err)
		}

		maddr, err := address.NewFromBytes(rval)
		if err != nil {
			return cid.Undef, err
		}

		gmcfg.MinerAddrs = append(gmcfg.MinerAddrs, maddr)

		params = mustEnc(&actors.UpdateStorageParams{Delta: types.NewInt(5000)})

		_, err = doExec(ctx, vm, actors.StorageMarketAddress, maddr, actors.SPAMethods.UpdateStorage, params)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to update total storage: %w", err)
		}

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

		nstate, err := cst.Put(ctx, &mstate)
		if err != nil {
			return cid.Undef, err
		}

		mact.Head = nstate
		if err := st.SetActor(maddr, mact); err != nil {
			return cid.Undef, err
		}
		// End of super haxx
	}

	return vm.Flush(ctx)
}

func doExec(ctx context.Context, vm *vm.VM, to, from address.Address, method uint64, params []byte) ([]byte, error) {
	return doExecValue(ctx, vm, to, from, types.NewInt(0), method, params)
}

func doExecValue(ctx context.Context, vm *vm.VM, to, from address.Address, value types.BigInt, method uint64, params []byte) ([]byte, error) {
	act, err := vm.StateTree().GetActor(from)
	if err != nil {
		return nil, xerrors.Errorf("doExec failed to get from actor: %w", err)
	}

	ret, err := vm.ApplyMessage(context.TODO(), &types.Message{
		To:       to,
		From:     from,
		Method:   method,
		Params:   params,
		GasLimit: types.NewInt(1000000),
		GasPrice: types.NewInt(0),
		Value:    value,
		Nonce:    act.Nonce,
	})
	if err != nil {
		return nil, xerrors.Errorf("doExec apply message failed: %w", err)
	}

	if ret.ExitCode != 0 {
		return nil, fmt.Errorf("failed to call method: %s", ret.ActorErr)
	}

	return ret.Return, nil
}

func MakeGenesisBlock(bs bstore.Blockstore, balances map[address.Address]types.BigInt, gmcfg *GenMinerCfg, ts uint64) (*GenesisBootstrap, error) {
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

	blks := amt.WrapBlockstore(bs)

	emptyroot, err := amt.FromArray(blks, nil)
	if err != nil {
		return nil, xerrors.Errorf("amt build failed: %w", err)
	}

	mm := &types.MsgMeta{
		BlsMessages:   emptyroot,
		SecpkMessages: emptyroot,
	}
	mmb, err := mm.ToStorageBlock()
	if err != nil {
		return nil, xerrors.Errorf("serializing msgmeta failed: %w", err)
	}
	if err := bs.Put(mmb); err != nil {
		return nil, xerrors.Errorf("putting msgmeta block to blockstore: %w", err)
	}

	log.Infof("Empty Genesis root: %s", emptyroot)

	genesisticket := &types.Ticket{
		VRFProof: []byte("vrf proof"),
	}

	b := &types.BlockHeader{
		Miner:                 actors.InitActorAddress,
		Tickets:               []*types.Ticket{genesisticket},
		ElectionProof:         []byte("the Genesis block"),
		Parents:               []cid.Cid{},
		Height:                0,
		ParentWeight:          types.NewInt(0),
		ParentStateRoot:       stateroot,
		Messages:              mmb.Cid(),
		ParentMessageReceipts: emptyroot,
		BLSAggregate:          types.Signature{Type: types.KTBLS, Data: []byte("signatureeee")},
		BlockSig:              types.Signature{Type: types.KTBLS, Data: []byte("block signatureeee")},
		Timestamp:             ts,
	}

	sb, err := b.ToStorageBlock()
	if err != nil {
		return nil, xerrors.Errorf("serializing block header failed: %w", err)
	}

	if err := bs.Put(sb); err != nil {
		return nil, xerrors.Errorf("putting header to blockstore: %w", err)
	}

	return &GenesisBootstrap{
		Genesis: b,
	}, nil
}
