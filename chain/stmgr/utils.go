package stmgr

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/rt"

	exported0 "github.com/filecoin-project/specs-actors/actors/builtin/exported"
	exported2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/exported"
	exported3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/exported"
	exported4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/exported"
	exported5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/exported"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

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
	actors = append(actors, exported3.BuiltinActors()...)
	actors = append(actors, exported4.BuiltinActors()...)
	actors = append(actors, exported5.BuiltinActors()...)

	for _, actor := range actors {
		exports := actor.Exports()
		methods := make(map[abi.MethodNum]MethodMeta, len(exports))

		// Explicitly add send, it's special.
		methods[builtin.MethodSend] = MethodMeta{
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

func GetParamType(actCode cid.Cid, method abi.MethodNum) (cbg.CBORUnmarshaler, error) {
	m, found := MethodsMap[actCode][method]
	if !found {
		return nil, fmt.Errorf("unknown method %d for actor %s", method, actCode)
	}
	return reflect.New(m.Params.Elem()).Interface().(cbg.CBORUnmarshaler), nil
}

func GetNetworkName(ctx context.Context, sm *StateManager, st cid.Cid) (dtypes.NetworkName, error) {
	act, err := sm.LoadActorRaw(ctx, init_.Address, st)
	if err != nil {
		return "", err
	}
	ias, err := init_.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return "", err
	}

	return ias.NetworkName()
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
		// Technically, the tipset we're passing in here should be ts+1, but that may not exist.
		base, err = sm.handleStateForks(ctx, base, i, &InvocationTracer{trace: &trace}, ts)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("error handling state forks: %w", err)
		}

		// We intentionally don't run cron here, as we may be trying to look into the
		// future. It's not guaranteed to be accurate... but that's fine.
	}

	r := store.NewChainRand(sm.cs, ts.Cids())
	vmopt := &vm.VMOpts{
		StateBase:      base,
		Epoch:          height,
		Rand:           r,
		Bstore:         sm.cs.StateBlockstore(),
		Syscalls:       sm.syscalls,
		CircSupplyCalc: sm.GetVMCirculatingSupply,
		NtwkVersion:    sm.GetNtwkVersion,
		BaseFee:        ts.Blocks()[0].ParentBaseFee,
		LookbackState:  LookbackStateGetterForTipset(sm, ts),
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

func LookbackStateGetterForTipset(sm *StateManager, ts *types.TipSet) vm.LookbackStateGetter {
	return func(ctx context.Context, round abi.ChainEpoch) (*state.StateTree, error) {
		_, st, err := GetLookbackTipSetForRound(ctx, sm, ts, round)
		if err != nil {
			return nil, err
		}
		return sm.StateTree(st)
	}
}

func GetLookbackTipSetForRound(ctx context.Context, sm *StateManager, ts *types.TipSet, round abi.ChainEpoch) (*types.TipSet, cid.Cid, error) {
	var lbr abi.ChainEpoch
	lb := policy.GetWinningPoStSectorSetLookback(sm.GetNtwkVersion(ctx, round))
	if round > lb {
		lbr = round - lb
	}

	// more null blocks than our lookback
	if lbr >= ts.Height() {
		// This should never happen at this point, but may happen before
		// network version 3 (where the lookback was only 10 blocks).
		st, _, err := sm.TipSetState(ctx, ts)
		if err != nil {
			return nil, cid.Undef, err
		}
		return ts, st, nil
	}

	// Get the tipset after the lookback tipset, or the next non-null one.
	nextTs, err := sm.ChainStore().GetTipsetByHeight(ctx, lbr+1, ts, false)
	if err != nil {
		return nil, cid.Undef, xerrors.Errorf("failed to get lookback tipset+1: %w", err)
	}

	if lbr > nextTs.Height() {
		return nil, cid.Undef, xerrors.Errorf("failed to find non-null tipset %s (%d) which is known to exist, found %s (%d)", ts.Key(), ts.Height(), nextTs.Key(), nextTs.Height())

	}

	lbts, err := sm.ChainStore().GetTipSetFromKey(nextTs.Parents())
	if err != nil {
		return nil, cid.Undef, xerrors.Errorf("failed to resolve lookback tipset: %w", err)
	}

	return lbts, nextTs.ParentState(), nil
}

func CheckTotalFIL(ctx context.Context, cs *store.ChainStore, ts *types.TipSet) (abi.TokenAmount, error) {
	str, err := state.LoadStateTree(cs.ActorStore(ctx), ts.ParentState())
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

func MakeMsgGasCost(msg *types.Message, ret *vm.ApplyRet) api.MsgGasCost {
	return api.MsgGasCost{
		Message:            msg.Cid(),
		GasUsed:            big.NewInt(ret.GasUsed),
		BaseFeeBurn:        ret.GasCosts.BaseFeeBurn,
		OverEstimationBurn: ret.GasCosts.OverEstimationBurn,
		MinerPenalty:       ret.GasCosts.MinerPenalty,
		MinerTip:           ret.GasCosts.MinerTip,
		Refund:             ret.GasCosts.Refund,
		TotalCost:          big.Sub(msg.RequiredFunds(), ret.GasCosts.Refund),
	}
}

func (sm *StateManager) ListAllActors(ctx context.Context, ts *types.TipSet) ([]address.Address, error) {
	stateTree, err := sm.StateTree(sm.parentState(ts))
	if err != nil {
		return nil, err
	}

	var out []address.Address
	err = stateTree.ForEach(func(addr address.Address, act *types.Actor) error {
		out = append(out, addr)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}
