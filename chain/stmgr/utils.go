package stmgr

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/manifest"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/api"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/system"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var ErrMetadataNotFound = errors.New("actor metadata not found")

func GetReturnType(ctx context.Context, sm *StateManager, to address.Address, method abi.MethodNum, ts *types.TipSet) (cbg.CBORUnmarshaler, error) {
	act, err := sm.LoadActor(ctx, to, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load actor: %w", err)
	}

	m, found := sm.tsExec.NewActorRegistry().Methods[act.Code][method]
	if !found {
		return nil, fmt.Errorf("unknown method %d for actor %s: %w", method, act.Code, ErrMetadataNotFound)
	}

	return reflect.New(m.Ret.Elem()).Interface().(cbg.CBORUnmarshaler), nil
}

func GetParamType(ar *vm.ActorRegistry, actCode cid.Cid, method abi.MethodNum) (cbg.CBORUnmarshaler, error) {
	m, found := ar.Methods[actCode][method]
	if !found {
		return nil, fmt.Errorf("unknown method %d for actor %s: %w", method, actCode, ErrMetadataNotFound)
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
		return cid.Undef, nil, xerrors.Errorf("failed to compute base state: %w", err)
	}

	for i := ts.Height(); i < height; i++ {
		// Technically, the tipset we're passing in here should be ts+1, but that may not exist.
		base, err = sm.HandleStateForks(ctx, base, i, &InvocationTracer{trace: &trace}, ts)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("error handling state forks: %w", err)
		}

		// We intentionally don't run cron here, as we may be trying to look into the
		// future. It's not guaranteed to be accurate... but that's fine.
	}

	r := rand.NewStateRand(sm.cs, ts.Cids(), sm.beacon, sm.GetNetworkVersion)
	vmopt := &vm.VMOpts{
		StateBase:      base,
		Epoch:          height,
		Timestamp:      ts.MinTimestamp(),
		Rand:           r,
		Bstore:         sm.cs.StateBlockstore(),
		Actors:         sm.tsExec.NewActorRegistry(),
		Syscalls:       sm.Syscalls,
		CircSupplyCalc: sm.GetVMCirculatingSupply,
		NetworkVersion: sm.GetNetworkVersion(ctx, height),
		BaseFee:        ts.Blocks()[0].ParentBaseFee,
		LookbackState:  LookbackStateGetterForTipset(sm, ts),
		TipSetGetter:   TipSetGetterForTipset(sm.cs, ts),
		Tracing:        true,
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

		ir := &api.InvocResult{
			MsgCid:         msg.Cid(),
			Msg:            msg,
			MsgRct:         &ret.MessageReceipt,
			ExecutionTrace: ret.ExecutionTrace,
			Duration:       ret.Duration,
		}
		if ret.ActorErr != nil {
			ir.Error = ret.ActorErr.Error()
		}
		if ret.GasCosts != nil {
			ir.GasCost = MakeMsgGasCost(msg, ret)
		}
		trace = append(trace, ir)
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

func TipSetGetterForTipset(cs *store.ChainStore, ts *types.TipSet) vm.TipSetGetter {
	return func(ctx context.Context, round abi.ChainEpoch) (types.TipSetKey, error) {
		ts, err := cs.GetTipsetByHeight(ctx, round, ts, true)
		if err != nil {
			return types.EmptyTSK, err
		}
		return ts.Key(), nil
	}
}

func GetLookbackTipSetForRound(ctx context.Context, sm *StateManager, ts *types.TipSet, round abi.ChainEpoch) (*types.TipSet, cid.Cid, error) {
	var lbr abi.ChainEpoch
	lb := policy.GetWinningPoStSectorSetLookback(sm.GetNetworkVersion(ctx, round))
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

	lbts, err := sm.ChainStore().GetTipSetFromKey(ctx, nextTs.Parents())
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

func GetManifestData(ctx context.Context, st *state.StateTree) (*manifest.ManifestData, error) {
	wrapStore := gstStore.WrapStore(ctx, st.Store)

	systemActor, err := st.GetActor(system.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get system actor: %w", err)
	}
	systemActorState, err := system.Load(wrapStore, systemActor)
	if err != nil {
		return nil, xerrors.Errorf("failed to load system actor state: %w", err)
	}

	actorsManifestDataCid := systemActorState.GetBuiltinActors()

	var mfData manifest.ManifestData
	if err := wrapStore.Get(ctx, actorsManifestDataCid, &mfData); err != nil {
		return nil, xerrors.Errorf("error fetching data: %w", err)
	}

	return &mfData, nil
}
