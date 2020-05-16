package stmgr

import (
	"bytes"
	"context"
	"os"

	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt "github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func GetNetworkName(ctx context.Context, sm *StateManager, st cid.Cid) (dtypes.NetworkName, error) {
	var state init_.State
	_, err := sm.LoadActorStateRaw(ctx, builtin.InitActorAddr, &state, st)
	if err != nil {
		return "", xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return dtypes.NetworkName(state.NetworkName), nil
}

func GetMinerWorkerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	var mas miner.State
	_, err := sm.LoadActorStateRaw(ctx, maddr, &mas, st)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	cst := cbor.NewCborStore(sm.cs.Blockstore())
	state, err := state.LoadStateTree(cst, st)
	if err != nil {
		return address.Undef, xerrors.Errorf("load state tree: %w", err)
	}

	return vm.ResolveToKeyAddr(state, cst, mas.Info.Worker)
}

func GetPower(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (power.Claim, power.Claim, error) {
	return GetPowerRaw(ctx, sm, ts.ParentState(), maddr)
}

func GetPowerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (power.Claim, power.Claim, error) {
	var ps power.State
	_, err := sm.LoadActorStateRaw(ctx, builtin.StoragePowerActorAddr, &ps, st)
	if err != nil {
		return power.Claim{}, power.Claim{}, xerrors.Errorf("(get sset) failed to load power actor state: %w", err)
	}

	var mpow power.Claim
	if maddr != address.Undef {
		cm, err := adt.AsMap(sm.cs.Store(ctx), ps.Claims)
		if err != nil {
			return power.Claim{}, power.Claim{}, err
		}

		var claim power.Claim
		if _, err := cm.Get(adt.AddrKey(maddr), &claim); err != nil {
			return power.Claim{}, power.Claim{}, err
		}

		mpow = claim
	}

	return mpow, power.Claim{
		RawBytePower:    ps.TotalRawBytePower,
		QualityAdjPower: ps.TotalQualityAdjPower,
	}, nil
}

func SectorSetSizes(ctx context.Context, sm *StateManager, maddr address.Address, ts *types.TipSet) (api.MinerSectors, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return api.MinerSectors{}, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	notProving, err := abi.BitFieldUnion(mas.Faults, mas.Recoveries)
	if err != nil {
		return api.MinerSectors{}, err
	}

	npc, err := notProving.Count()
	if err != nil {
		return api.MinerSectors{}, err
	}

	blks := cbor.NewCborStore(sm.ChainStore().Blockstore())
	ss, err := amt.LoadAMT(ctx, blks, mas.Sectors)
	if err != nil {
		return api.MinerSectors{}, err
	}

	return api.MinerSectors{
		Sset: ss.Count,
		Pset: ss.Count - npc,
	}, nil
}

func PreCommitInfo(ctx context.Context, sm *StateManager, maddr address.Address, sid abi.SectorNumber, ts *types.TipSet) (miner.SectorPreCommitOnChainInfo, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return miner.SectorPreCommitOnChainInfo{}, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	i, ok, err := mas.GetPrecommittedSector(sm.cs.Store(ctx), sid)
	if err != nil {
		return miner.SectorPreCommitOnChainInfo{}, err
	}
	if !ok {
		return miner.SectorPreCommitOnChainInfo{}, xerrors.New("precommit not found")
	}

	return *i, nil
}

func GetMinerSectorSet(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address, filter *abi.BitField, filterOut bool) ([]*api.ChainSectorInfo, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return LoadSectorsFromSet(ctx, sm.ChainStore().Blockstore(), mas.Sectors, filter, filterOut)
}

func GetSectorsForWinningPoSt(ctx context.Context, pv ffiwrapper.Verifier, sm *StateManager, st cid.Cid, maddr address.Address, rand abi.PoStRandomness) ([]abi.SectorInfo, error) {
	var mas miner.State
	_, err := sm.LoadActorStateRaw(ctx, maddr, &mas, st)
	if err != nil {
		return nil, xerrors.Errorf("(get sectors) failed to load miner actor state: %w", err)
	}

	// TODO: Optimization: we could avoid loaditg the whole proving set here if we had AMT.GetNth with bitfield filtering
	sectorSet, err := GetProvingSetRaw(ctx, sm, mas)
	if err != nil {
		return nil, xerrors.Errorf("getting proving set: %w", err)
	}

	if len(sectorSet) == 0 {
		return nil, nil
	}

	spt, err := ffiwrapper.SealProofTypeFromSectorSize(mas.Info.SectorSize)
	if err != nil {
		return nil, xerrors.Errorf("getting seal proof type: %w", err)
	}

	wpt, err := spt.RegisteredWinningPoStProof()
	if err != nil {
		return nil, xerrors.Errorf("getting window proof type: %w", err)
	}

	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return nil, xerrors.Errorf("getting miner ID: %w", err)
	}

	ids, err := pv.GenerateWinningPoStSectorChallenge(ctx, wpt, abi.ActorID(mid), rand, uint64(len(sectorSet)))
	if err != nil {
		return nil, xerrors.Errorf("generating winning post challenges: %w", err)
	}

	out := make([]abi.SectorInfo, len(ids))
	for i, n := range ids {
		out[i] = abi.SectorInfo{
			RegisteredProof: wpt,
			SectorNumber:    sectorSet[n].ID,
			SealedCID:       sectorSet[n].Info.Info.SealedCID,
		}
	}

	return out, nil
}

func StateMinerInfo(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (miner.MinerInfo, error) {
	var mas miner.State
	_, err := sm.LoadActorStateRaw(ctx, maddr, &mas, ts.ParentState())
	if err != nil {
		return miner.MinerInfo{}, xerrors.Errorf("(get ssize) failed to load miner actor state: %w", err)
	}

	return mas.Info, nil
}

func GetMinerSlashed(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (bool, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return false, xerrors.Errorf("(get miner slashed) failed to load miner actor state")
	}

	var spas power.State
	_, err = sm.LoadActorState(ctx, builtin.StoragePowerActorAddr, &spas, ts)
	if err != nil {
		return false, xerrors.Errorf("(get miner slashed) failed to load power actor state")
	}

	store := sm.cs.Store(ctx)

	claims, err := adt.AsMap(store, spas.Claims)
	if err != nil {
		return false, err
	}

	ok, err := claims.Get(power.AddrKey(maddr), nil)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}

	return false, nil
}

func GetMinerDeadlines(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (*miner.Deadlines, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get ssize) failed to load miner actor state: %w", err)
	}

	return mas.LoadDeadlines(sm.cs.Store(ctx))
}

func GetMinerFaults(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (*abi.BitField, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get faults) failed to load miner actor state: %w", err)
	}

	return mas.Faults, nil
}

func GetMinerRecoveries(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (*abi.BitField, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get recoveries) failed to load miner actor state: %w", err)
	}

	return mas.Recoveries, nil
}

func GetStorageDeal(ctx context.Context, sm *StateManager, dealId abi.DealID, ts *types.TipSet) (*api.MarketDeal, error) {
	var state market.State
	if _, err := sm.LoadActorState(ctx, builtin.StorageMarketActorAddr, &state, ts); err != nil {
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

	sa, err := market.AsDealStateArray(sm.ChainStore().Store(ctx), state.States)
	if err != nil {
		return nil, err
	}

	st, found, err := sa.Get(dealId)
	if err != nil {
		return nil, err
	}

	if !found {
		st = &market.DealState{
			SectorStartEpoch: -1,
			LastUpdatedEpoch: -1,
			SlashEpoch:       -1,
		}
	}

	return &api.MarketDeal{
		Proposal: dp,
		State:    *st,
	}, nil
}

func ListMinerActors(ctx context.Context, sm *StateManager, ts *types.TipSet) ([]address.Address, error) {
	var state power.State
	if _, err := sm.LoadActorState(ctx, builtin.StoragePowerActorAddr, &state, ts); err != nil {
		return nil, err
	}

	m, err := adt.AsMap(sm.cs.Store(ctx), state.Claims)
	if err != nil {
		return nil, err
	}

	var miners []address.Address
	err = m.ForEach(nil, func(k string) error {
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

func LoadSectorsFromSet(ctx context.Context, bs blockstore.Blockstore, ssc cid.Cid, filter *abi.BitField, filterOut bool) ([]*api.ChainSectorInfo, error) {
	a, err := amt.LoadAMT(ctx, cbor.NewCborStore(bs), ssc)
	if err != nil {
		return nil, err
	}

	var sset []*api.ChainSectorInfo
	if err := a.ForEach(ctx, func(i uint64, v *cbg.Deferred) error {
		if filter != nil {
			set, err := filter.IsSet(i)
			if err != nil {
				return xerrors.Errorf("filter check error: %w", err)
			}
			if set == filterOut {
				return nil
			}
		}

		var oci miner.SectorOnChainInfo
		if err := cbor.DecodeInto(v.Raw, &oci); err != nil {
			return err
		}
		sset = append(sset, &api.ChainSectorInfo{
			Info: oci,
			ID:   abi.SectorNumber(i),
		})
		return nil
	}); err != nil {
		return nil, err
	}

	return sset, nil
}

func ComputeState(ctx context.Context, sm *StateManager, height abi.ChainEpoch, msgs []*types.Message, ts *types.TipSet) (cid.Cid, []*api.InvocResult, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	base, trace, err := sm.ExecutionTrace(ctx, ts)
	if err != nil {
		return cid.Undef, nil, err
	}

	fstate, err := sm.handleStateForks(ctx, base, height, ts.Height())
	if err != nil {
		return cid.Undef, nil, err
	}

	r := store.NewChainRand(sm.cs, ts.Cids(), height)
	vmi, err := vm.NewVM(fstate, height, r, sm.cs.Blockstore(), sm.cs.VMSys())
	if err != nil {
		return cid.Undef, nil, err
	}

	for i, msg := range msgs {
		// TODO: Use the signed message length for secp messages
		ret, err := vmi.ApplyMessage(ctx, msg)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("applying message %s: %w", msg.Cid(), err)
		}
		if ret.ExitCode != 0 {
			log.Infof("compute state apply message %d failed (exit: %d): %s", i, ret.ExitCode, ret.ActorErr)
		}
	}

	root, err := vmi.Flush(ctx)
	if err != nil {
		return cid.Undef, nil, err
	}

	return root, trace, nil
}

func GetProvingSetRaw(ctx context.Context, sm *StateManager, mas miner.State) ([]*api.ChainSectorInfo, error) {
	notProving, err := abi.BitFieldUnion(mas.Faults, mas.Recoveries)
	if err != nil {
		return nil, err
	}

	provset, err := LoadSectorsFromSet(ctx, sm.cs.Blockstore(), mas.Sectors, notProving, true)
	if err != nil {
		return nil, xerrors.Errorf("failed to get proving set: %w", err)
	}

	return provset, nil
}

func GetLookbackTipSetForRound(ctx context.Context, sm *StateManager, ts *types.TipSet, round abi.ChainEpoch) (*types.TipSet, error) {
	var lbr abi.ChainEpoch
	if round > build.WinningPoStSectorSetLookback {
		lbr = round - build.WinningPoStSectorSetLookback
	}

	// more null blocks than our lookback
	if lbr > ts.Height() {
		return ts, nil
	}

	lbts, err := sm.ChainStore().GetTipsetByHeight(ctx, lbr, ts, true)
	if err != nil {
		return nil, xerrors.Errorf("failed to get lookback tipset: %w", err)
	}

	return lbts, nil
}

func MinerGetBaseInfo(ctx context.Context, sm *StateManager, bcn beacon.RandomBeacon, tsk types.TipSetKey, round abi.ChainEpoch, maddr address.Address, pv ffiwrapper.Verifier) (*api.MiningBaseInfo, error) {
	ts, err := sm.ChainStore().LoadTipSet(tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load tipset for mining base: %w", err)
	}

	prev, err := sm.ChainStore().GetLatestBeaconEntry(ts)
	if err != nil {
		if os.Getenv("LOTUS_IGNORE_DRAND") != "_yes_" {
			return nil, xerrors.Errorf("failed to get latest beacon entry: %w", err)
		}

		prev = &types.BeaconEntry{}
	}

	entries, err := beacon.BeaconEntriesForBlock(ctx, bcn, round, *prev)
	if err != nil {
		return nil, err
	}

	rbase := *prev
	if len(entries) > 0 {
		rbase = entries[len(entries)-1]
	}

	lbts, err := GetLookbackTipSetForRound(ctx, sm, ts, round)
	if err != nil {
		return nil, xerrors.Errorf("getting lookback miner actor state: %w", err)
	}

	lbst, _, err := sm.TipSetState(ctx, lbts)
	if err != nil {
		return nil, err
	}

	var mas miner.State
	if _, err := sm.LoadActorStateRaw(ctx, maddr, &mas, lbst); err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err := maddr.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal miner address: %w", err)
	}

	prand, err := store.DrawRandomness(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, round, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to get randomness for winning post: %w", err)
	}

	sectors, err := GetSectorsForWinningPoSt(ctx, pv, sm, lbst, maddr, prand)
	if err != nil {
		return nil, xerrors.Errorf("getting wpost proving set: %w", err)
	}

	if len(sectors) == 0 {
		return nil, nil
	}

	mpow, tpow, err := GetPowerRaw(ctx, sm, lbst, maddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to get power: %w", err)
	}

	worker, err := sm.ResolveToKeyAddress(ctx, mas.GetWorker(), ts)
	if err != nil {
		return nil, xerrors.Errorf("resolving worker address: %w", err)
	}

	return &api.MiningBaseInfo{
		MinerPower:      mpow.QualityAdjPower,
		NetworkPower:    tpow.QualityAdjPower,
		Sectors:         sectors,
		WorkerKey:       worker,
		SectorSize:      mas.Info.SectorSize,
		PrevBeaconEntry: *prev,
		BeaconEntries:   entries,
	}, nil
}
