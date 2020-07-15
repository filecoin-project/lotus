package stmgr

import (
	"bytes"
	"context"
	"os"
	"reflect"

	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt "github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/cron"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
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
		return "", xerrors.Errorf("(get sset) failed to load init actor state: %w", err)
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

	info, err := mas.GetInfo(sm.cs.Store(ctx))
	if err != nil {
		return address.Address{}, err
	}

	return vm.ResolveToKeyAddr(state, cst, info.Worker)
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

	notProving, err := bitfield.MultiMerge(mas.Faults, mas.Recoveries)
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

func MinerSectorInfo(ctx context.Context, sm *StateManager, maddr address.Address, sid abi.SectorNumber, ts *types.TipSet) (*miner.SectorOnChainInfo, error) {
	var mas miner.State
	_, err := sm.LoadActorState(ctx, maddr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	sectorInfo, ok, err := mas.GetSector(sm.cs.Store(ctx), sid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, xerrors.New("sector not found")
	}

	return sectorInfo, nil
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

	cst := cbor.NewCborStore(sm.cs.Blockstore())
	var deadlines miner.Deadlines
	if err := cst.Get(ctx, mas.Deadlines, &deadlines); err != nil {
		return nil, xerrors.Errorf("failed to load deadlines: %w", err)
	}

	notProving, err := bitfield.MultiMerge(mas.Faults, mas.Recoveries)
	if err != nil {
		return nil, xerrors.Errorf("failed to union faults and recoveries: %w", err)
	}

	allSectors, err := bitfield.MultiMerge(append(deadlines.Due[:], mas.NewSectors)...)
	if err != nil {
		return nil, xerrors.Errorf("merging deadline bitfields failed: %w", err)
	}

	provingSectors, err := bitfield.SubtractBitField(allSectors, notProving)
	if err != nil {
		return nil, xerrors.Errorf("failed to subtract non-proving sectors from set: %w", err)
	}

	numProvSect, err := provingSectors.Count()
	if err != nil {
		return nil, xerrors.Errorf("failed to count bits: %w", err)
	}

	// TODO(review): is this right? feels fishy to me
	if numProvSect == 0 {
		return nil, nil
	}

	info, err := mas.GetInfo(sm.cs.Store(ctx))
	if err != nil {
		return nil, err
	}

	spt, err := ffiwrapper.SealProofTypeFromSectorSize(info.SectorSize)
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

	ids, err := pv.GenerateWinningPoStSectorChallenge(ctx, wpt, abi.ActorID(mid), rand, numProvSect)
	if err != nil {
		return nil, xerrors.Errorf("generating winning post challenges: %w", err)
	}

	sectors, err := provingSectors.All(miner.SectorsMax)
	if err != nil {
		return nil, xerrors.Errorf("failed to enumerate all sector IDs: %w", err)
	}

	sectorAmt, err := amt.LoadAMT(ctx, cst, mas.Sectors)
	if err != nil {
		return nil, xerrors.Errorf("failed to load sectors amt: %w", err)
	}

	out := make([]abi.SectorInfo, len(ids))
	for i, n := range ids {
		sid := sectors[n]

		var sinfo miner.SectorOnChainInfo
		if err := sectorAmt.Get(ctx, sid, &sinfo); err != nil {
			return nil, xerrors.Errorf("failed to get sector %d: %w", sid, err)
		}

		out[i] = abi.SectorInfo{
			SealProof:    spt,
			SectorNumber: sinfo.SectorNumber,
			SealedCID:    sinfo.SealedCID,
		}
	}

	return out, nil
}

func StateMinerInfo(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (*miner.MinerInfo, error) {
	var mas miner.State
	_, err := sm.LoadActorStateRaw(ctx, maddr, &mas, ts.ParentState())
	if err != nil {
		return nil, xerrors.Errorf("(get ssize) failed to load miner actor state: %w", err)
	}

	return mas.GetInfo(sm.cs.Store(ctx))
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

func GetStorageDeal(ctx context.Context, sm *StateManager, dealID abi.DealID, ts *types.TipSet) (*api.MarketDeal, error) {
	var state market.State
	if _, err := sm.LoadActorState(ctx, builtin.StorageMarketActorAddr, &state, ts); err != nil {
		return nil, err
	}

	da, err := amt.LoadAMT(ctx, cbor.NewCborStore(sm.ChainStore().Blockstore()), state.Proposals)
	if err != nil {
		return nil, err
	}

	var dp market.DealProposal
	if err := da.Get(ctx, uint64(dealID), &dp); err != nil {
		return nil, err
	}

	sa, err := market.AsDealStateArray(sm.ChainStore().Store(ctx), state.States)
	if err != nil {
		return nil, err
	}

	st, found, err := sa.Get(dealID)
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
	notProving, err := bitfield.MultiMerge(mas.Faults, mas.Recoveries)
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

	info, err := mas.GetInfo(sm.cs.Store(ctx))
	if err != nil {
		return nil, err
	}

	worker, err := sm.ResolveToKeyAddress(ctx, info.Worker, ts)
	if err != nil {
		return nil, xerrors.Errorf("resolving worker address: %w", err)
	}

	return &api.MiningBaseInfo{
		MinerPower:      mpow.QualityAdjPower,
		NetworkPower:    tpow.QualityAdjPower,
		Sectors:         sectors,
		WorkerKey:       worker,
		SectorSize:      info.SectorSize,
		PrevBeaconEntry: *prev,
		BeaconEntries:   entries,
	}, nil
}

func (sm *StateManager) CirculatingSupply(ctx context.Context, ts *types.TipSet) (abi.TokenAmount, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	st, _, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return big.Zero(), err
	}

	r := store.NewChainRand(sm.cs, ts.Cids(), ts.Height())
	vmi, err := vm.NewVM(st, ts.Height(), r, sm.cs.Blockstore(), sm.cs.VMSys())
	if err != nil {
		return big.Zero(), err
	}

	unsafeVM := &vm.UnsafeVM{VM: vmi}
	rt := unsafeVM.MakeRuntime(ctx, &types.Message{
		GasLimit: 1_000_000_000,
		From:     builtin.SystemActorAddr,
	}, builtin.SystemActorAddr, 0, 0, 0)

	return rt.TotalFilCircSupply(), nil
}

type methodMeta struct {
	Name string

	Params reflect.Type
	Ret    reflect.Type
}

var MethodsMap = map[cid.Cid][]methodMeta{}

func init() {
	cidToMethods := map[cid.Cid][2]interface{}{
		// builtin.SystemActorCodeID:        {builtin.MethodsSystem, system.Actor{} }- apparently it doesn't have methods
		builtin.InitActorCodeID:             {builtin.MethodsInit, init_.Actor{}},
		builtin.CronActorCodeID:             {builtin.MethodsCron, cron.Actor{}},
		builtin.AccountActorCodeID:          {builtin.MethodsAccount, account.Actor{}},
		builtin.StoragePowerActorCodeID:     {builtin.MethodsPower, power.Actor{}},
		builtin.StorageMinerActorCodeID:     {builtin.MethodsMiner, miner.Actor{}},
		builtin.StorageMarketActorCodeID:    {builtin.MethodsMarket, market.Actor{}},
		builtin.PaymentChannelActorCodeID:   {builtin.MethodsPaych, paych.Actor{}},
		builtin.MultisigActorCodeID:         {builtin.MethodsMultisig, multisig.Actor{}},
		builtin.RewardActorCodeID:           {builtin.MethodsReward, reward.Actor{}},
		builtin.VerifiedRegistryActorCodeID: {builtin.MethodsVerifiedRegistry, verifreg.Actor{}},
	}

	for c, m := range cidToMethods {
		rt := reflect.TypeOf(m[0])
		nf := rt.NumField()

		MethodsMap[c] = append(MethodsMap[c], methodMeta{
			Name:   "Send",
			Params: reflect.TypeOf(new(adt.EmptyValue)),
			Ret:    reflect.TypeOf(new(adt.EmptyValue)),
		})

		exports := m[1].(abi.Invokee).Exports()
		for i := 0; i < nf; i++ {
			export := reflect.TypeOf(exports[i+1])

			MethodsMap[c] = append(MethodsMap[c], methodMeta{
				Name:   rt.Field(i).Name,
				Params: export.In(1),
				Ret:    export.Out(0),
			})
		}
	}
}

func GetReturnType(ctx context.Context, sm *StateManager, to address.Address, method abi.MethodNum, ts *types.TipSet) (cbg.CBORUnmarshaler, error) {
	act, err := sm.GetActor(to, ts)
	if err != nil {
		return nil, err
	}

	m := MethodsMap[act.Code][method]
	return reflect.New(m.Ret.Elem()).Interface().(cbg.CBORUnmarshaler), nil
}
