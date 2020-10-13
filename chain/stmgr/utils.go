package stmgr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/actors/policy"

	cid "github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/rt"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	exported0 "github.com/filecoin-project/specs-actors/actors/builtin/exported"
	proof0 "github.com/filecoin-project/specs-actors/actors/runtime/proof"
	exported2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/exported"

	"github.com/filecoin-project/lotus/api"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func GetNetworkName(ctx context.Context, sm *StateManager, st cid.Cid) (dtypes.NetworkName, error) {
	act, err := sm.LoadActorRaw(ctx, init_.Address, st)
	if err != nil {
		return "", err
	}
	ias, err := init_.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return "", err
	}

	return ias.NetworkName()
}

func GetMinerWorkerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	state, err := sm.StateTree(st)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load state tree: %w", err)
	}
	act, err := state.GetActor(maddr)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}
	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	info, err := mas.Info()
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to load actor info: %w", err)
	}

	return vm.ResolveToKeyAddr(state, sm.cs.Store(ctx), info.Worker)
}

func GetPower(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (power.Claim, power.Claim, bool, error) {
	return GetPowerRaw(ctx, sm, ts.ParentState(), maddr)
}

func GetPowerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (power.Claim, power.Claim, bool, error) {
	act, err := sm.LoadActorRaw(ctx, power.Address, st)
	if err != nil {
		return power.Claim{}, power.Claim{}, false, xerrors.Errorf("(get sset) failed to load power actor state: %w", err)
	}

	pas, err := power.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return power.Claim{}, power.Claim{}, false, err
	}

	tpow, err := pas.TotalPower()
	if err != nil {
		return power.Claim{}, power.Claim{}, false, err
	}

	var mpow power.Claim
	var minpow bool
	if maddr != address.Undef {
		var found bool
		mpow, found, err = pas.MinerPower(maddr)
		if err != nil || !found {
			// TODO: return an error when not found?
			return power.Claim{}, power.Claim{}, false, err
		}

		minpow, err = pas.MinerNominalPowerMeetsConsensusMinimum(maddr)
		if err != nil {
			return power.Claim{}, power.Claim{}, false, err
		}
	}

	return mpow, tpow, minpow, nil
}

func PreCommitInfo(ctx context.Context, sm *StateManager, maddr address.Address, sid abi.SectorNumber, ts *types.TipSet) (*miner.SectorPreCommitOnChainInfo, error) {
	act, err := sm.LoadActor(ctx, maddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.GetPrecommittedSector(sid)
}

func MinerSectorInfo(ctx context.Context, sm *StateManager, maddr address.Address, sid abi.SectorNumber, ts *types.TipSet) (*miner.SectorOnChainInfo, error) {
	act, err := sm.LoadActor(ctx, maddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.GetSector(sid)
}

func GetMinerSectorSet(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address, snos *bitfield.BitField) ([]*miner.SectorOnChainInfo, error) {
	act, err := sm.LoadActor(ctx, maddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.LoadSectors(snos)
}

func GetSectorsForWinningPoSt(ctx context.Context, pv ffiwrapper.Verifier, sm *StateManager, st cid.Cid, maddr address.Address, rand abi.PoStRandomness) ([]proof0.SectorInfo, error) {
	act, err := sm.LoadActorRaw(ctx, maddr, st)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	// TODO (!!): Actor Update: Make this active sectors

	allSectors, err := miner.AllPartSectors(mas, miner.Partition.AllSectors)
	if err != nil {
		return nil, xerrors.Errorf("get all sectors: %w", err)
	}

	faultySectors, err := miner.AllPartSectors(mas, miner.Partition.FaultySectors)
	if err != nil {
		return nil, xerrors.Errorf("get faulty sectors: %w", err)
	}

	provingSectors, err := bitfield.SubtractBitField(allSectors, faultySectors) // TODO: This is wrong, as it can contain faaults, change to just ActiveSectors in an upgrade
	if err != nil {
		return nil, xerrors.Errorf("calc proving sectors: %w", err)
	}

	numProvSect, err := provingSectors.Count()
	if err != nil {
		return nil, xerrors.Errorf("failed to count bits: %w", err)
	}

	// TODO(review): is this right? feels fishy to me
	if numProvSect == 0 {
		return nil, nil
	}

	info, err := mas.Info()
	if err != nil {
		return nil, xerrors.Errorf("getting miner info: %w", err)
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

	iter, err := provingSectors.BitIterator()
	if err != nil {
		return nil, xerrors.Errorf("iterating over proving sectors: %w", err)
	}

	// Select winning sectors by _index_ in the all-sectors bitfield.
	selectedSectors := bitfield.New()
	prev := uint64(0)
	for _, n := range ids {
		sno, err := iter.Nth(n - prev)
		if err != nil {
			return nil, xerrors.Errorf("iterating over proving sectors: %w", err)
		}
		selectedSectors.Set(sno)
		prev = n
	}

	sectors, err := mas.LoadSectors(&selectedSectors)
	if err != nil {
		return nil, xerrors.Errorf("loading proving sectors: %w", err)
	}

	out := make([]proof0.SectorInfo, len(sectors))
	for i, sinfo := range sectors {
		out[i] = proof0.SectorInfo{
			SealProof:    spt,
			SectorNumber: sinfo.SectorNumber,
			SealedCID:    sinfo.SealedCID,
		}
	}

	return out, nil
}

func StateMinerInfo(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (*miner.MinerInfo, error) {
	act, err := sm.LoadActor(ctx, maddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	mi, err := mas.Info()
	if err != nil {
		return nil, err
	}

	return &mi, err
}

func GetMinerSlashed(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (bool, error) {
	act, err := sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return false, xerrors.Errorf("failed to load power actor: %w", err)
	}

	spas, err := power.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return false, xerrors.Errorf("failed to load power actor state: %w", err)
	}

	_, ok, err := spas.MinerPower(maddr)
	if err != nil {
		return false, xerrors.Errorf("getting miner power: %w", err)
	}

	if !ok {
		return true, nil
	}

	return false, nil
}

func GetStorageDeal(ctx context.Context, sm *StateManager, dealID abi.DealID, ts *types.TipSet) (*api.MarketDeal, error) {
	act, err := sm.LoadActor(ctx, market.Address, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load market actor: %w", err)
	}

	state, err := market.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load market actor state: %w", err)
	}

	proposals, err := state.Proposals()
	if err != nil {
		return nil, err
	}

	proposal, found, err := proposals.Get(dealID)

	if err != nil {
		return nil, err
	} else if !found {
		return nil, xerrors.Errorf("deal %d not found", dealID)
	}

	states, err := state.States()
	if err != nil {
		return nil, err
	}

	st, found, err := states.Get(dealID)
	if err != nil {
		return nil, err
	}

	if !found {
		st = market.EmptyDealState()
	}

	return &api.MarketDeal{
		Proposal: *proposal,
		State:    *st,
	}, nil
}

func ListMinerActors(ctx context.Context, sm *StateManager, ts *types.TipSet) ([]address.Address, error) {
	act, err := sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load power actor: %w", err)
	}

	powState, err := power.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load power actor state: %w", err)
	}

	return powState.ListAllMiners()
}

func ComputeState(ctx context.Context, sm *StateManager, height abi.ChainEpoch, msgs []*types.Message, ts *types.TipSet) (cid.Cid, []*api.InvocResult, error) {
	if ts == nil {
		ts = sm.cs.GetHeaviestTipSet()
	}

	base, trace, err := sm.ExecutionTrace(ctx, ts)
	if err != nil {
		return cid.Undef, nil, err
	}

	for i := ts.Height(); i < height; i++ {
		// handle state forks
		base, err = sm.handleStateForks(ctx, base, i, traceFunc(&trace), ts)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("error handling state forks: %w", err)
		}

		// TODO: should we also run cron here?
	}

	r := store.NewChainRand(sm.cs, ts.Cids())
	vmopt := &vm.VMOpts{
		StateBase:      base,
		Epoch:          height,
		Rand:           r,
		Bstore:         sm.cs.Blockstore(),
		Syscalls:       sm.cs.VMSys(),
		CircSupplyCalc: sm.GetVMCirculatingSupply,
		NtwkVersion:    sm.GetNtwkVersion,
		BaseFee:        ts.Blocks()[0].ParentBaseFee,
	}
	vmi, err := sm.newVM(ctx, vmopt)
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

func GetLookbackTipSetForRound(ctx context.Context, sm *StateManager, ts *types.TipSet, round abi.ChainEpoch) (*types.TipSet, error) {
	var lbr abi.ChainEpoch
	lb := policy.GetWinningPoStSectorSetLookback(sm.GetNtwkVersion(ctx, round))
	if round > lb {
		lbr = round - lb
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

func MinerGetBaseInfo(ctx context.Context, sm *StateManager, bcs beacon.Schedule, tsk types.TipSetKey, round abi.ChainEpoch, maddr address.Address, pv ffiwrapper.Verifier) (*api.MiningBaseInfo, error) {
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

	entries, err := beacon.BeaconEntriesForBlock(ctx, bcs, round, ts.Height(), *prev)
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

	act, err := sm.LoadActorRaw(ctx, maddr, lbst)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
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
		return nil, xerrors.Errorf("getting winning post proving set: %w", err)
	}

	if len(sectors) == 0 {
		return nil, nil
	}

	mpow, tpow, _, err := GetPowerRaw(ctx, sm, lbst, maddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to get power: %w", err)
	}

	info, err := mas.Info()
	if err != nil {
		return nil, err
	}

	worker, err := sm.ResolveToKeyAddress(ctx, info.Worker, ts)
	if err != nil {
		return nil, xerrors.Errorf("resolving worker address: %w", err)
	}

	// TODO: Not ideal performance...This method reloads miner and power state (already looked up here and in GetPowerRaw)
	eligible, err := MinerEligibleToMine(ctx, sm, maddr, ts, lbts)
	if err != nil {
		return nil, xerrors.Errorf("determining miner eligibility: %w", err)
	}

	return &api.MiningBaseInfo{
		MinerPower:        mpow.QualityAdjPower,
		NetworkPower:      tpow.QualityAdjPower,
		Sectors:           sectors,
		WorkerKey:         worker,
		SectorSize:        info.SectorSize,
		PrevBeaconEntry:   *prev,
		BeaconEntries:     entries,
		EligibleForMining: eligible,
	}, nil
}

type MethodMeta struct {
	Name string

	Params reflect.Type
	Ret    reflect.Type
}

var MethodsMap = map[cid.Cid]map[abi.MethodNum]MethodMeta{}

func init() {
	// TODO: combine with the runtime actor registry.
	var actors []rt.VMActor
	actors = append(actors, exported0.BuiltinActors()...)
	actors = append(actors, exported2.BuiltinActors()...)

	for _, actor := range actors {
		exports := actor.Exports()
		methods := make(map[abi.MethodNum]MethodMeta, len(exports))

		// Explicitly add send, it's special.
		// Note that builtin2.MethodSend = builtin0.MethodSend = 0.
		methods[builtin0.MethodSend] = MethodMeta{
			Name:   "Send",
			Params: reflect.TypeOf(new(abi.EmptyValue)),
			Ret:    reflect.TypeOf(new(abi.EmptyValue)),
		}

		// Iterate over exported methods. Some of these _may_ be nil and
		// must be skipped.
		for number, export := range exports {
			if export == nil {
				continue
			}

			ev := reflect.ValueOf(export)
			et := ev.Type()

			// Extract the method names using reflection. These
			// method names always match the field names in the
			// `builtin.Method*` structs (tested in the specs-actors
			// tests).
			fnName := runtime.FuncForPC(ev.Pointer()).Name()
			fnName = strings.TrimSuffix(fnName[strings.LastIndexByte(fnName, '.')+1:], "-fm")

			switch abi.MethodNum(number) {
			case builtin0.MethodSend:
				// Note that builtin2.MethodSend = builtin0.MethodSend = 0.
				panic("method 0 is reserved for Send")
			case builtin0.MethodConstructor:
				// Note that builtin2.MethodConstructor = builtin0.MethodConstructor = 1.
				if fnName != "Constructor" {
					panic("method 1 is reserved for Constructor")
				}
			}

			methods[abi.MethodNum(number)] = MethodMeta{
				Name:   fnName,
				Params: et.In(1),
				Ret:    et.Out(0),
			}
		}
		MethodsMap[actor.Code()] = methods
	}
}

func GetReturnType(ctx context.Context, sm *StateManager, to address.Address, method abi.MethodNum, ts *types.TipSet) (cbg.CBORUnmarshaler, error) {
	act, err := sm.LoadActor(ctx, to, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}

	m, found := MethodsMap[act.Code][method]
	if !found {
		return nil, fmt.Errorf("unknown method %d for actor %s", method, act.Code)
	}
	return reflect.New(m.Ret.Elem()).Interface().(cbg.CBORUnmarshaler), nil
}

func minerHasMinPower(ctx context.Context, sm *StateManager, addr address.Address, ts *types.TipSet) (bool, error) {
	pact, err := sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return false, xerrors.Errorf("loading power actor state: %w", err)
	}

	ps, err := power.Load(sm.cs.Store(ctx), pact)
	if err != nil {
		return false, err
	}

	return ps.MinerNominalPowerMeetsConsensusMinimum(addr)
}

func MinerEligibleToMine(ctx context.Context, sm *StateManager, addr address.Address, baseTs *types.TipSet, lookbackTs *types.TipSet) (bool, error) {
	hmp, err := minerHasMinPower(ctx, sm, addr, lookbackTs)

	// TODO: We're blurring the lines between a "runtime network version" and a "Lotus upgrade epoch", is that unavoidable?
	if sm.GetNtwkVersion(ctx, baseTs.Height()) <= network.Version3 {
		return hmp, err
	}

	if err != nil {
		return false, err
	}

	if !hmp {
		return false, nil
	}

	// Post actors v2, also check MinerEligibleForElection with base ts

	pact, err := sm.LoadActor(ctx, power.Address, baseTs)
	if err != nil {
		return false, xerrors.Errorf("loading power actor state: %w", err)
	}

	pstate, err := power.Load(sm.cs.Store(ctx), pact)
	if err != nil {
		return false, err
	}

	mact, err := sm.LoadActor(ctx, addr, baseTs)
	if err != nil {
		return false, xerrors.Errorf("loading miner actor state: %w", err)
	}

	mstate, err := miner.Load(sm.cs.Store(ctx), mact)
	if err != nil {
		return false, err
	}

	// Non-empty power claim.
	if claim, found, err := pstate.MinerPower(addr); err != nil {
		return false, err
	} else if !found {
		return false, err
	} else if claim.QualityAdjPower.LessThanEqual(big.Zero()) {
		return false, err
	}

	// No fee debt.
	if debt, err := mstate.FeeDebt(); err != nil {
		return false, err
	} else if !debt.IsZero() {
		return false, err
	}

	// No active consensus faults.
	if mInfo, err := mstate.Info(); err != nil {
		return false, err
	} else if baseTs.Height() <= mInfo.ConsensusFaultElapsed {
		return false, nil
	}

	return true, nil
}

func CheckTotalFIL(ctx context.Context, sm *StateManager, ts *types.TipSet) (abi.TokenAmount, error) {
	str, err := state.LoadStateTree(sm.ChainStore().Store(ctx), ts.ParentState())
	if err != nil {
		return abi.TokenAmount{}, err
	}

	sum := types.NewInt(0)
	err = str.ForEach(func(a address.Address, act *types.Actor) error {
		sum = types.BigAdd(sum, act.Balance)
		return nil
	})
	if err != nil {
		return abi.TokenAmount{}, err
	}

	return sum, nil
}
