package stmgr

import (
	"context"

	amt "github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"

	ffi "github.com/filecoin-project/filecoin-ffi"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"

	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

func GetMinerWorkerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	var mas miner.State
	_, err := sm.LoadActorStateRaw(ctx, maddr, &mas, st)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.Info.Worker, nil
}

func GetMinerOwner(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	var mas miner.State
	_, err := sm.LoadActorStateRaw(ctx, maddr, &mas, st)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.Info.Owner, nil
}

func GetPower(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (types.BigInt, types.BigInt, error) {
	var ps power.State
	_, err := sm.LoadActorState(ctx, maddr, &ps, ts)
	if err != nil {
		return big.Zero(), big.Zero(), xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	var mpow big.Int
	if maddr != address.Undef {
		var claim power.Claim
		if _, err := adt.AsMap(sm.cs.Store(ctx), ps.Claims).Get(adt.AddrKey(maddr), &claim); err != nil {
			return big.Zero(), big.Zero(), err
		}

		mpow = claim.Power
	}

	return mpow, ps.TotalNetworkPower, nil
}

func GetMinerPeerID(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (peer.ID, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return "", xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.Info.PeerId, nil
}

func GetMinerWorker(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (address.Address, error) {
	return GetMinerWorkerRaw(ctx, sm, sm.parentState(ts), maddr)
}

func GetMinerPostState(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (*miner.PoStState, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get eps) failed to load miner actor state: %w", err)
	}

	return &mas.PoStState, nil
}

func SectorSetSizes(ctx context.Context, sm *StateManager, maddr address.Address, ts *types.TipSet) (api.MinerSectors, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return api.MinerSectors{}, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	blks := cbor.NewCborStore(sm.ChainStore().Blockstore())
	ss, err := amt.LoadAMT(ctx, blks, mas.Sectors)
	if err != nil {
		return api.MinerSectors{}, err
	}

	ps, err := amt.LoadAMT(ctx, blks, mas.ProvingSet)
	if err != nil {
		return api.MinerSectors{}, err
	}

	return api.MinerSectors{
		Pset: ps.Count,
		Sset: ss.Count,
	}, nil
}

func GetMinerProvingSet(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) ([]*api.ChainSectorInfo, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get pset) failed to load miner actor state: %w", err)
	}

	return LoadSectorsFromSet(ctx, sm.ChainStore().Blockstore(), mas.ProvingSet)
}

func GetMinerSectorSet(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) ([]*api.ChainSectorInfo, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return LoadSectorsFromSet(ctx, sm.ChainStore().Blockstore(), mas.Sectors)
}

func GetSectorsForElectionPost(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (*sectorbuilder.SortedPublicSectorInfo, error) {
	sectors, err := GetMinerProvingSet(ctx, sm, ts, maddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to get sector set for miner: %w", err)
	}

	var uselessOtherArray []ffi.PublicSectorInfo
	for _, s := range sectors {
		var uselessBuffer [32]byte
		copy(uselessBuffer[:], s.CommR)
		uselessOtherArray = append(uselessOtherArray, ffi.PublicSectorInfo{
			SectorNum: s.SectorID,
			CommR:     uselessBuffer,
		})
	}

	ssi := sectorbuilder.NewSortedPublicSectorInfo(uselessOtherArray)
	return &ssi, nil
}

func GetMinerSectorSize(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (abi.SectorSize, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return 0, xerrors.Errorf("(get ssize) failed to load miner actor state: %w", err)
	}

	return mas.Info.SectorSize, nil
}

func GetMinerSlashed(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (uint64, error) {
	panic("TODO")
}

func GetMinerFaults(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) ([]abi.SectorNumber, error) {
	var mas actors.StorageMinerActorState
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get ssize) failed to load miner actor state: %w", err)
	}

	ss, lerr := amt.LoadAMT(ctx, cbor.NewCborStore(sm.cs.Blockstore()), mas.Sectors)
	if lerr != nil {
		return nil, aerrors.HandleExternalError(lerr, "could not load proving set node")
	}

	faults, err := mas.FaultSet.All(2 * ss.Count)
	if err != nil {
		return nil, xerrors.Errorf("reading fault bit set: %w", err)
	}

	out := make([]abi.SectorNumber, len(faults))
	for i, fault := range faults {
		out[i] = abi.SectorNumber(fault)
	}

	return out, nil
}

func GetStorageDeal(ctx context.Context, sm *StateManager, dealId abi.DealID, ts *types.TipSet) (*api.MarketDeal, error) {
	var state actors.StorageMarketState
	if _, err := sm.LoadActorState(ctx, actors.StorageMarketAddress, &state, ts); err != nil {
		return nil, err
	}

	da, err := amt.LoadAMT(ctx, cbor.NewCborStore(sm.ChainStore().Blockstore()), state.Proposals)
	if err != nil {
		return nil, err
	}

	var dp market.DealProposal
	if err := da.Get(ctx, uint64(dealId), &dp); err != nil {
		return nil, err
	}

	sa, err := amt.LoadAMT(ctx, cbor.NewCborStore(sm.ChainStore().Blockstore()), state.States)
	if err != nil {
		return nil, err
	}

	var st market.DealState
	if err := sa.Get(ctx, uint64(dealId), &st); err != nil {
		return nil, err
	}

	return &api.MarketDeal{
		Proposal: dp,
		State:    st,
	}, nil
}

func ListMinerActors(ctx context.Context, sm *StateManager, ts *types.TipSet) ([]address.Address, error) {
	var state actors.StoragePowerState
	if _, err := sm.LoadActorState(ctx, actors.StoragePowerAddress, &state, ts); err != nil {
		return nil, err
	}

	var miners []address.Address
	err := adt.AsMap(sm.cs.Store(ctx), state.Claims).ForEach(nil, func(k string) error {
		a, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}
		miners = append(miners, a)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return miners, nil
}

func LoadSectorsFromSet(ctx context.Context, bs blockstore.Blockstore, ssc cid.Cid) ([]*api.ChainSectorInfo, error) {
	a, err := amt.LoadAMT(ctx, cbor.NewCborStore(bs), ssc)
	if err != nil {
		return nil, err
	}

	var sset []*api.ChainSectorInfo
	if err := a.ForEach(ctx, func(i uint64, v *cbg.Deferred) error {
		var comms [][]byte
		if err := cbor.DecodeInto(v.Raw, &comms); err != nil {
			return err
		}
		sset = append(sset, &api.ChainSectorInfo{
			SectorID: abi.SectorNumber(i),
			CommR:    comms[0],
			CommD:    comms[1],
		})
		return nil
	}); err != nil {
		return nil, err
	}

	return sset, nil
}

func ComputeState(ctx context.Context, sm *StateManager, height abi.ChainEpoch, msgs []*types.Message, ts *types.TipSet) (cid.Cid, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	base, _, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return cid.Undef, err
	}

	fstate, err := sm.handleStateForks(ctx, base, height, ts.Height())
	if err != nil {
		return cid.Undef, err
	}

	r := store.NewChainRand(sm.cs, ts.Cids(), height)
	vmi, err := vm.NewVM(fstate, height, r, actors.NetworkAddress, sm.cs.Blockstore(), sm.cs.VMSys())
	if err != nil {
		return cid.Undef, err
	}

	for i, msg := range msgs {
		ret, err := vmi.ApplyMessage(ctx, msg)
		if err != nil {
			return cid.Undef, xerrors.Errorf("applying message %s: %w", msg.Cid(), err)
		}
		if ret.ExitCode != 0 {
			log.Infof("compute state apply message %d failed (exit: %d): %s", i, ret.ExitCode, ret.ActorErr)
		}
	}

	return vmi.Flush(ctx)
}
