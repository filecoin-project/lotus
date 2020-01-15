package gen

import (
	"bytes"
	"context"
	"fmt"

	amt "github.com/filecoin-project/go-amt-ipld"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	peer "github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	actors "github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/genesis"
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
		Code: actors.InitCodeCid,
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

	if err := state.SetActor(actors.InitAddress, initact); err != nil {
		return nil, xerrors.Errorf("set init actor: %w", err)
	}

	cronact, err := SetupCronActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup cron actor: %w", err)
	}

	if err := state.SetActor(actors.CronAddress, cronact); err != nil {
		return nil, xerrors.Errorf("set cron actor: %w", err)
	}

	spact, err := SetupStoragePowerActor(bs)
	if err != nil {
		return nil, xerrors.Errorf("setup storage market actor: %w", err)
	}

	if err := state.SetActor(actors.StoragePowerAddress, spact); err != nil {
		return nil, xerrors.Errorf("set storage market actor: %w", err)
	}

	netAmt := types.FromFil(build.TotalFilecoin)
	for _, amt := range actmap {
		netAmt = types.BigSub(netAmt, amt)
	}

	err = state.SetActor(actors.NetworkAddress, &types.Actor{
		Code:    actors.AccountCodeCid,
		Balance: netAmt,
		Head:    emptyobject,
	})
	if err != nil {
		return nil, xerrors.Errorf("set network account actor: %w", err)
	}

	err = state.SetActor(actors.BurntFundsAddress, &types.Actor{
		Code:    actors.AccountCodeCid,
		Balance: types.NewInt(0),
		Head:    emptyobject,
	})
	if err != nil {
		return nil, xerrors.Errorf("set burnt funds account actor: %w", err)
	}

	for a, v := range actmap {
		err = state.SetActor(a, &types.Actor{
			Code:    actors.AccountCodeCid,
			Balance: v,
			Head:    emptyobject,
		})
		if err != nil {
			return nil, xerrors.Errorf("setting account from actmap: %w", err)
		}
	}

	return state, nil
}

func SetupCronActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := hamt.CSTFromBstore(bs)
	cas := &actors.CronActorState{}

	stcid, err := cst.Put(context.TODO(), cas)
	if err != nil {
		return nil, err
	}

	return &types.Actor{
		Code:    actors.CronCodeCid,
		Head:    stcid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}, nil
}

func SetupStoragePowerActor(bs bstore.Blockstore) (*types.Actor, error) {
	cst := hamt.CSTFromBstore(bs)
	nd := hamt.NewNode(cst)
	emptyhamt, err := cst.Put(context.TODO(), nd)
	if err != nil {
		return nil, err
	}

	blks := amt.WrapBlockstore(bs)
	emptyamt, err := amt.FromArray(blks, nil)
	if err != nil {
		return nil, xerrors.Errorf("amt build failed: %w", err)
	}

	sms := &actors.StoragePowerState{
		Miners:         emptyhamt,
		ProvingBuckets: emptyamt,
		TotalStorage:   types.NewInt(0),
	}

	stcid, err := cst.Put(context.TODO(), sms)
	if err != nil {
		return nil, err
	}

	return &types.Actor{
		Code:    actors.StoragePowerCodeCid,
		Head:    stcid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}, nil
}

func SetupStorageMarketActor(bs bstore.Blockstore, sroot cid.Cid, deals []actors.StorageDealProposal) (cid.Cid, error) {
	cst := hamt.CSTFromBstore(bs)
	nd := hamt.NewNode(cst)
	emptyHAMT, err := cst.Put(context.TODO(), nd)
	if err != nil {
		return cid.Undef, err
	}

	blks := amt.WrapBlockstore(bs)

	cdeals := make([]cbg.CBORMarshaler, len(deals))
	for i, deal := range deals {
		cdeals[i] = &actors.OnChainDeal{
			PieceRef:             deal.PieceRef,
			PieceSize:            deal.PieceSize,
			Client:               deal.Client,
			Provider:             deal.Provider,
			ProposalExpiration:   deal.ProposalExpiration,
			Duration:             deal.Duration,
			StoragePricePerEpoch: deal.StoragePricePerEpoch,
			StorageCollateral:    deal.StorageCollateral,
			ActivationEpoch:      1,
		}
	}

	dealAmt, err := amt.FromArray(blks, cdeals)
	if err != nil {
		return cid.Undef, xerrors.Errorf("amt build failed: %w", err)
	}

	sms := &actors.StorageMarketState{
		Balances:   emptyHAMT,
		Deals:      dealAmt,
		NextDealID: uint64(len(deals)),
	}

	stcid, err := cst.Put(context.TODO(), sms)
	if err != nil {
		return cid.Undef, err
	}

	act := &types.Actor{
		Code:    actors.StorageMarketCodeCid,
		Head:    stcid,
		Nonce:   0,
		Balance: types.NewInt(0),
	}

	state, err := state.LoadStateTree(cst, sroot)
	if err != nil {
		return cid.Undef, xerrors.Errorf("making new state tree: %w", err)
	}

	if err := state.SetActor(actors.StorageMarketAddress, act); err != nil {
		return cid.Undef, xerrors.Errorf("set storage market actor: %w", err)
	}

	return state.Flush()
}

type GenMinerCfg struct {
	PreSeals map[string]genesis.GenesisMiner

	// The addresses of the created miner, this is set by the genesis setup
	MinerAddrs []address.Address

	PeerIDs []peer.ID
}

func mustEnc(i cbg.CBORMarshaler) []byte {
	enc, err := actors.SerializeParams(i)
	if err != nil {
		panic(err) // ok
	}
	return enc
}

func SetupStorageMiners(ctx context.Context, cs *store.ChainStore, sroot cid.Cid, gmcfg *GenMinerCfg) (cid.Cid, []actors.StorageDealProposal, error) {
	vm, err := vm.NewVM(sroot, 0, nil, actors.NetworkAddress, cs.Blockstore(), cs.VMSys())
	if err != nil {
		return cid.Undef, nil, xerrors.Errorf("failed to create NewVM: %w", err)
	}

	if len(gmcfg.MinerAddrs) == 0 {
		return cid.Undef, nil, xerrors.New("no genesis miners")
	}

	if len(gmcfg.MinerAddrs) != len(gmcfg.PreSeals) {
		return cid.Undef, nil, xerrors.Errorf("miner address list, and preseal count doesn't match (%d != %d)", len(gmcfg.MinerAddrs), len(gmcfg.PreSeals))
	}

	var deals []actors.StorageDealProposal

	for i, maddr := range gmcfg.MinerAddrs {
		ps, psok := gmcfg.PreSeals[maddr.String()]
		if !psok {
			return cid.Undef, nil, xerrors.Errorf("no preseal for miner %s", maddr)
		}

		minerParams := &actors.CreateStorageMinerParams{
			Owner:      ps.Owner,
			Worker:     ps.Worker,
			SectorSize: ps.SectorSize,
			PeerID:     gmcfg.PeerIDs[i], // TODO: grab from preseal too
		}

		params := mustEnc(minerParams)

		// TODO: hardcoding 6500 here is a little fragile, it changes any
		// time anyone changes the initial account allocations
		rval, err := doExecValue(ctx, vm, actors.StoragePowerAddress, ps.Worker, types.FromFil(6500), actors.SPAMethods.CreateStorageMiner, params)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("failed to create genesis miner: %w", err)
		}

		maddrret, err := address.NewFromBytes(rval)
		if err != nil {
			return cid.Undef, nil, err
		}

		_, err = vm.Flush(ctx)
		if err != nil {
			return cid.Undef, nil, err
		}

		cst := hamt.CSTFromBstore(cs.Blockstore())
		if err := reassignMinerActorAddress(vm, cst, maddrret, maddr); err != nil {
			return cid.Undef, nil, err
		}

		power := types.BigMul(types.NewInt(minerParams.SectorSize), types.NewInt(uint64(len(ps.Sectors))))

		params = mustEnc(&actors.UpdateStorageParams{Delta: power})

		_, err = doExec(ctx, vm, actors.StoragePowerAddress, maddr, actors.SPAMethods.UpdateStorage, params)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("failed to update total storage: %w", err)
		}

		// we have to flush the vm here because it buffers stuff internally for perf reasons
		if _, err := vm.Flush(ctx); err != nil {
			return cid.Undef, nil, xerrors.Errorf("vm.Flush failed: %w", err)
		}

		st := vm.StateTree()
		mact, err := st.GetActor(maddr)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("get miner actor failed: %w", err)
		}

		var mstate actors.StorageMinerActorState
		if err := cst.Get(ctx, mact.Head, &mstate); err != nil {
			return cid.Undef, nil, xerrors.Errorf("getting miner actor state failed: %w", err)
		}
		mstate.Power = types.BigMul(types.NewInt(ps.SectorSize), types.NewInt(uint64(len(ps.Sectors))))

		blks := amt.WrapBlockstore(cs.Blockstore())

		for _, s := range ps.Sectors {
			nssroot, err := actors.AddToSectorSet(ctx, blks, mstate.Sectors, s.SectorID, s.CommR[:], s.CommD[:])
			if err != nil {
				return cid.Undef, nil, xerrors.Errorf("failed to add fake sector to sector set: %w", err)
			}
			mstate.Sectors = nssroot
			mstate.ProvingSet = nssroot

			deals = append(deals, s.Deal)
		}

		nstate, err := cst.Put(ctx, &mstate)
		if err != nil {
			return cid.Undef, nil, err
		}

		mact.Head = nstate
		if err := st.SetActor(maddr, mact); err != nil {
			return cid.Undef, nil, err
		}
	}

	c, err := vm.Flush(ctx)
	return c, deals, err
}

func reassignMinerActorAddress(vm *vm.VM, cst *hamt.CborIpldStore, from, to address.Address) error {
	if from == to {
		return nil
	}
	act, err := vm.StateTree().GetActor(from)
	if err != nil {
		return xerrors.Errorf("reassign: failed to get 'from' actor: %w", err)
	}

	_, err = vm.StateTree().GetActor(to)
	if err == nil {
		return xerrors.Errorf("cannot reassign actor, target address taken")
	}
	if err := vm.StateTree().SetActor(to, act); err != nil {
		return xerrors.Errorf("failed to reassign actor: %w", err)
	}

	if err := adjustStorageMarketTracking(vm, cst, from, to); err != nil {
		return xerrors.Errorf("adjusting storage market tracking: %w", err)
	}

	// Now, adjust the tracking in the init actor
	return initActorReassign(vm, cst, from, to)
}

func adjustStorageMarketTracking(vm *vm.VM, cst *hamt.CborIpldStore, from, to address.Address) error {
	ctx := context.TODO()
	act, err := vm.StateTree().GetActor(actors.StoragePowerAddress)
	if err != nil {
		return xerrors.Errorf("loading storage power actor: %w", err)
	}

	var spst actors.StoragePowerState
	if err := cst.Get(ctx, act.Head, &spst); err != nil {
		return xerrors.Errorf("loading storage power actor state: %w", err)
	}

	miners, err := hamt.LoadNode(ctx, cst, spst.Miners)
	if err != nil {
		return xerrors.Errorf("loading miner set: %w", err)
	}

	if err := miners.Delete(ctx, string(from.Bytes())); err != nil {
		return xerrors.Errorf("deleting from spa set: %w", err)
	}

	if err := miners.Set(ctx, string(to.Bytes()), uint64(1)); err != nil {
		return xerrors.Errorf("failed setting miner: %w", err)
	}

	if err := miners.Flush(ctx); err != nil {
		return err
	}

	nminerscid, err := cst.Put(ctx, miners)
	if err != nil {
		return err
	}
	spst.Miners = nminerscid

	nhead, err := cst.Put(ctx, &spst)
	if err != nil {
		return err
	}

	act.Head = nhead

	return nil
}

func initActorReassign(vm *vm.VM, cst *hamt.CborIpldStore, from, to address.Address) error {
	ctx := context.TODO()
	initact, err := vm.StateTree().GetActor(actors.InitAddress)
	if err != nil {
		return xerrors.Errorf("couldnt get init actor: %w", err)
	}

	var st actors.InitActorState
	if err := cst.Get(ctx, initact.Head, &st); err != nil {
		return xerrors.Errorf("reassign loading init actor state: %w", err)
	}

	amap, err := hamt.LoadNode(ctx, cst, st.AddressMap)
	if err != nil {
		return xerrors.Errorf("failed to load init actor map: %w", err)
	}

	target, err := address.IDFromAddress(from)
	if err != nil {
		return xerrors.Errorf("failed to extract ID: %w", err)
	}

	var out string
	halt := xerrors.Errorf("halt")
	err = amap.ForEach(ctx, func(k string, v interface{}) error {
		_, val, err := cbg.CborReadHeader(bytes.NewReader(v.(*cbg.Deferred).Raw))
		if err != nil {
			return xerrors.Errorf("parsing int in map failed: %w", err)
		}

		if val == target {
			out = k
			return halt
		}
		return nil
	})

	if err == nil {
		return xerrors.Errorf("could not find from address in init ID map")
	}
	if !xerrors.Is(err, halt) {
		return xerrors.Errorf("finding address in ID map failed: %w", err)
	}

	if err := amap.Delete(ctx, out); err != nil {
		return xerrors.Errorf("deleting 'from' entry in amap: %w", err)
	}

	if err := amap.Set(ctx, out, target); err != nil {
		return xerrors.Errorf("setting 'to' entry in amap: %w", err)
	}

	if err := amap.Flush(ctx); err != nil {
		return xerrors.Errorf("failed to flush amap: %w", err)
	}

	ncid, err := cst.Put(ctx, amap)
	if err != nil {
		return err
	}

	st.AddressMap = ncid

	nacthead, err := cst.Put(ctx, &st)
	if err != nil {
		return err
	}

	initact.Head = nacthead

	return nil
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

func MakeGenesisBlock(bs bstore.Blockstore, sys *types.VMSyscalls, balances map[address.Address]types.BigInt, gmcfg *GenMinerCfg, ts uint64) (*GenesisBootstrap, error) {
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
	cs := store.NewChainStore(bs, datastore.NewMapDatastore(), sys)
	stateroot, deals, err := SetupStorageMiners(ctx, cs, stateroot, gmcfg)
	if err != nil {
		return nil, xerrors.Errorf("setup storage miners failed: %w", err)
	}

	stateroot, err = SetupStorageMarketActor(bs, stateroot, deals)
	if err != nil {
		return nil, xerrors.Errorf("setup storage market actor: %w", err)
	}

	stateroot, err = AdjustInitActorStartID(ctx, bs, stateroot, 1000)
	if err != nil {
		return nil, xerrors.Errorf("failed to adjust init actor start ID: %w", err)
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
		VRFProof: []byte("vrf proof0000000vrf proof0000000"),
	}

	b := &types.BlockHeader{
		Miner:  actors.InitAddress,
		Ticket: genesisticket,
		EPostProof: types.EPostProof{
			Proof:    []byte("not a real proof"),
			PostRand: []byte("i guess this is kinda random"),
		},
		Parents:               []cid.Cid{},
		Height:                0,
		ParentWeight:          types.NewInt(0),
		ParentStateRoot:       stateroot,
		Messages:              mmb.Cid(),
		ParentMessageReceipts: emptyroot,
		BLSAggregate:          types.Signature{Type: types.KTBLS, Data: []byte("signatureeee")},
		BlockSig:              &types.Signature{Type: types.KTBLS, Data: []byte("block signatureeee")},
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

func AdjustInitActorStartID(ctx context.Context, bs blockstore.Blockstore, stateroot cid.Cid, val uint64) (cid.Cid, error) {
	cst := hamt.CSTFromBstore(bs)

	tree, err := state.LoadStateTree(cst, stateroot)
	if err != nil {
		return cid.Undef, err
	}

	act, err := tree.GetActor(actors.InitAddress)
	if err != nil {
		return cid.Undef, err
	}

	var st actors.InitActorState
	if err := cst.Get(ctx, act.Head, &st); err != nil {
		return cid.Undef, err
	}

	st.NextID = val

	nstate, err := cst.Put(ctx, &st)
	if err != nil {
		return cid.Undef, err
	}

	act.Head = nstate

	if err := tree.SetActor(actors.InitAddress, act); err != nil {
		return cid.Undef, err
	}

	return tree.Flush()
}
