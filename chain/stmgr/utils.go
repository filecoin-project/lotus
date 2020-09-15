package stmgr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"

	saruntime "github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/proof"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/cron"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"
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

func GetPower(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (power.Claim, power.Claim, error) {
	return GetPowerRaw(ctx, sm, ts.ParentState(), maddr)
}

func GetPowerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (power.Claim, power.Claim, error) {
	act, err := sm.LoadActorRaw(ctx, builtin.StoragePowerActorAddr, st)
	if err != nil {
		return power.Claim{}, power.Claim{}, xerrors.Errorf("(get sset) failed to load power actor state: %w", err)
	}

	mas, err := power.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return power.Claim{}, power.Claim{}, err
	}

	tpow, err := mas.TotalPower()
	if err != nil {
		return power.Claim{}, power.Claim{}, err
	}

	var mpow power.Claim
	if maddr != address.Undef {
		mpow, err = mas.MinerPower(maddr)
		if err != nil {
			return power.Claim{}, power.Claim{}, err
		}
	}

	return mpow, tpow, nil
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

func GetMinerSectorSet(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address, filter *bitfield.BitField, filterOut bool) ([]*miner.ChainSectorInfo, error) {
	act, err := sm.LoadActor(ctx, maddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.LoadSectorsFromSet(filter, filterOut)
}

func GetSectorsForWinningPoSt(ctx context.Context, pv ffiwrapper.Verifier, sm *StateManager, st cid.Cid, maddr address.Address, rand abi.PoStRandomness) ([]proof.SectorInfo, error) {
	act, err := sm.LoadActorRaw(ctx, maddr, st)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	// TODO (!!): This was partition.Sectors-partition.Faults originally, which was likely very wrong, and will need to be an upgrade
	//  ^ THE CURRENT THING HERE WON'T SYNC v

	provingSectors, err := miner.AllPartSectors(mas, miner.Partition.ActiveSectors)
	if err != nil {
		return nil, xerrors.Errorf("merge partition proving sets: %w", err)
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

	sectors, err := mas.LoadSectorsFromSet(&provingSectors, false)
	if err != nil {
		return nil, xerrors.Errorf("loading proving sectors: %w", err)
	}

	out := make([]proof.SectorInfo, len(ids))
	for i, n := range ids {
		s := sectors[n]

		out[i] = proof.SectorInfo{
			SealProof:    spt,
			SectorNumber: s.ID,
			SealedCID:    s.Info.SealedCID,
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
	act, err := sm.LoadActor(ctx, builtin.StoragePowerActorAddr, ts)
	if err != nil {
		return false, xerrors.Errorf("failed to load power actor: %w", err)
	}

	spas, err := power.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return false, xerrors.Errorf("failed to load power actor state: %w", err)
	}

	store := sm.cs.Store(ctx)

	claims, err := adt.AsMap(store, spas.Claims)
	if err != nil {
		return false, err
	}

	ok, err := claims.Get(abi.AddrKey(maddr), nil)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}

	return false, nil
}

func GetStorageDeal(ctx context.Context, sm *StateManager, dealID abi.DealID, ts *types.TipSet) (*api.MarketDeal, error) {
	act, err := sm.LoadActor(ctx, builtin.StorageMarketActorAddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load market actor: %w", err)
	}

	state, err := market.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load market actor state: %w", err)
	}

	store := sm.ChainStore().Store(ctx)

	da, err := adt.AsArray(store, state.Proposals)
	if err != nil {
		return nil, err
	}

	var dp market.DealProposal
	if found, err := da.Get(uint64(dealID), &dp); err != nil {
		return nil, err
	} else if !found {
		return nil, xerrors.Errorf("deal %d not found", dealID)
	}

	sa, err := market.AsDealStateArray(store, state.States)
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
	act, err := sm.LoadActor(ctx, builtin.StoragePowerActorAddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load power actor: %w", err)
	}

	state, err := power.Load(sm.cs.Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load power actor state: %w", err)
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

func LoadSectorsFromSet(ctx context.Context, bs blockstore.Blockstore, ssc cid.Cid, filter *bitfield.BitField, filterOut bool) ([]*miner.ChainSectorInfo, error) {
	a, err := adt.AsArray(store.ActorStore(ctx, bs), ssc)
	if err != nil {
		return nil, err
	}

	var sset []*miner.ChainSectorInfo
	var v cbg.Deferred
	if err := a.ForEach(&v, func(i int64) error {
		if filter != nil {
			set, err := filter.IsSet(uint64(i))
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
		sset = append(sset, &miner.ChainSectorInfo{
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

	r := store.NewChainRand(sm.cs, ts.Cids())
	vmopt := &vm.VMOpts{
		StateBase:      base,
		Epoch:          height,
		Rand:           r,
		Bstore:         sm.cs.Blockstore(),
		Syscalls:       sm.cs.VMSys(),
		CircSupplyCalc: sm.GetCirculatingSupply,
		NtwkVersion:    sm.GetNtwkVersion,
		BaseFee:        ts.Blocks()[0].ParentBaseFee,
	}
	vmi, err := vm.NewVM(ctx, vmopt)
	if err != nil {
		return cid.Undef, nil, err
	}

	for i := ts.Height(); i < height; i++ {
		// handle state forks
		err = sm.handleStateForks(ctx, vmi.StateTree(), i, ts)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("error handling state forks: %w", err)
		}

		// TODO: should we also run cron here?
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
		return nil, xerrors.Errorf("getting wpost proving set: %w", err)
	}

	if len(sectors) == 0 {
		return nil, nil
	}

	mpow, tpow, err := GetPowerRaw(ctx, sm, lbst, maddr)
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

	hmp, err := MinerHasMinPower(ctx, sm, maddr, lbts)
	if err != nil {
		return nil, xerrors.Errorf("determining if miner has min power failed: %w", err)
	}

	return &api.MiningBaseInfo{
		MinerPower:      mpow.QualityAdjPower,
		NetworkPower:    tpow.QualityAdjPower,
		Sectors:         sectors,
		WorkerKey:       worker,
		SectorSize:      info.SectorSize,
		PrevBeaconEntry: *prev,
		BeaconEntries:   entries,
		HasMinPower:     hmp,
	}, nil
}

type MethodMeta struct {
	Name string

	Params reflect.Type
	Ret    reflect.Type
}

var MethodsMap = map[cid.Cid]map[abi.MethodNum]MethodMeta{}

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
		exports := m[1].(saruntime.Invokee).Exports()
		methods := make(map[abi.MethodNum]MethodMeta, len(exports))

		// Explicitly add send, it's special.
		methods[builtin.MethodSend] = MethodMeta{
			Name:   "Send",
			Params: reflect.TypeOf(new(abi.EmptyValue)),
			Ret:    reflect.TypeOf(new(abi.EmptyValue)),
		}

		// Learn method names from the builtin.Methods* structs.
		rv := reflect.ValueOf(m[0])
		rt := rv.Type()
		nf := rt.NumField()
		methodToName := make([]string, len(exports))
		for i := 0; i < nf; i++ {
			name := rt.Field(i).Name
			number := rv.Field(i).Interface().(abi.MethodNum)
			methodToName[number] = name
		}

		// Iterate over exported methods. Some of these _may_ be nil and
		// must be skipped.
		for number, export := range exports {
			if export == nil {
				continue
			}

			ev := reflect.ValueOf(export)
			et := ev.Type()

			// Make sure the method name is correct.
			// This is just a nice sanity check.
			fnName := runtime.FuncForPC(ev.Pointer()).Name()
			fnName = strings.TrimSuffix(fnName[strings.LastIndexByte(fnName, '.')+1:], "-fm")
			mName := methodToName[number]
			if mName != fnName {
				panic(fmt.Sprintf(
					"actor method name is %s but exported method name is %s",
					fnName, mName,
				))
			}

			switch abi.MethodNum(number) {
			case builtin.MethodSend:
				panic("method 0 is reserved for Send")
			case builtin.MethodConstructor:
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
		MethodsMap[c] = methods
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

func MinerHasMinPower(ctx context.Context, sm *StateManager, addr address.Address, ts *types.TipSet) (bool, error) {
	pact, err := sm.LoadActor(ctx, builtin.StoragePowerActorAddr, ts)
	if err != nil {
		return false, xerrors.Errorf("loading power actor state: %w", err)
	}

	ps, err := power.Load(sm.cs.Store(ctx), pact)
	if err != nil {
		return false, err
	}

	return ps.MinerNominalPowerMeetsConsensusMinimum(addr)
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
