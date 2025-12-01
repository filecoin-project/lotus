package consensus

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	exported0 "github.com/filecoin-project/specs-actors/actors/builtin/exported"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"
	exported2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/exported"
	exported3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/exported"
	exported4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/exported"
	exported5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/exported"
	exported6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/exported"
	exported7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/exported"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/cron"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/metrics"
)

func NewActorRegistry() *vm.ActorRegistry {
	inv := vm.NewActorRegistry()

	inv.Register(actorstypes.Version0, vm.ActorsVersionPredicate(actorstypes.Version0), builtin.MakeRegistryLegacy(exported0.BuiltinActors()))
	inv.Register(actorstypes.Version2, vm.ActorsVersionPredicate(actorstypes.Version2), builtin.MakeRegistryLegacy(exported2.BuiltinActors()))
	inv.Register(actorstypes.Version3, vm.ActorsVersionPredicate(actorstypes.Version3), builtin.MakeRegistryLegacy(exported3.BuiltinActors()))
	inv.Register(actorstypes.Version4, vm.ActorsVersionPredicate(actorstypes.Version4), builtin.MakeRegistryLegacy(exported4.BuiltinActors()))
	inv.Register(actorstypes.Version5, vm.ActorsVersionPredicate(actorstypes.Version5), builtin.MakeRegistryLegacy(exported5.BuiltinActors()))
	inv.Register(actorstypes.Version6, vm.ActorsVersionPredicate(actorstypes.Version6), builtin.MakeRegistryLegacy(exported6.BuiltinActors()))
	inv.Register(actorstypes.Version7, vm.ActorsVersionPredicate(actorstypes.Version7), builtin.MakeRegistryLegacy(exported7.BuiltinActors()))
	inv.Register(actorstypes.Version8, vm.ActorsVersionPredicate(actorstypes.Version8), builtin.MakeRegistry(actorstypes.Version8))
	inv.Register(actorstypes.Version9, vm.ActorsVersionPredicate(actorstypes.Version9), builtin.MakeRegistry(actorstypes.Version9))
	inv.Register(actorstypes.Version10, vm.ActorsVersionPredicate(actorstypes.Version10), builtin.MakeRegistry(actorstypes.Version10))
	inv.Register(actorstypes.Version11, vm.ActorsVersionPredicate(actorstypes.Version11), builtin.MakeRegistry(actorstypes.Version11))
	inv.Register(actorstypes.Version12, vm.ActorsVersionPredicate(actorstypes.Version12), builtin.MakeRegistry(actorstypes.Version12))
	inv.Register(actorstypes.Version13, vm.ActorsVersionPredicate(actorstypes.Version13), builtin.MakeRegistry(actorstypes.Version13))
	inv.Register(actorstypes.Version14, vm.ActorsVersionPredicate(actorstypes.Version14), builtin.MakeRegistry(actorstypes.Version14))
	inv.Register(actorstypes.Version15, vm.ActorsVersionPredicate(actorstypes.Version15), builtin.MakeRegistry(actorstypes.Version15))
	inv.Register(actorstypes.Version16, vm.ActorsVersionPredicate(actorstypes.Version16), builtin.MakeRegistry(actorstypes.Version16))
	inv.Register(actorstypes.Version17, vm.ActorsVersionPredicate(actorstypes.Version17), builtin.MakeRegistry(actorstypes.Version17))
	inv.Register(actorstypes.Version18, vm.ActorsVersionPredicate(actorstypes.Version18), builtin.MakeRegistry(actorstypes.Version18))

	return inv
}

type TipSetExecutor struct {
	reward RewardFunc
}

func NewTipSetExecutor(r RewardFunc) *TipSetExecutor {
	return &TipSetExecutor{reward: r}
}

func (t *TipSetExecutor) NewActorRegistry() *vm.ActorRegistry {
	return NewActorRegistry()
}

type FilecoinBlockMessages struct {
	store.BlockMessages

	WinCount int64
}

func (t *TipSetExecutor) ApplyBlocks(ctx context.Context,
	sm *stmgr.StateManager,
	parentEpoch abi.ChainEpoch,
	pstate cid.Cid,
	bms []FilecoinBlockMessages,
	epoch abi.ChainEpoch,
	r rand.Rand,
	em stmgr.ExecMonitor,
	vmTracing bool,
	baseFee abi.TokenAmount,
	ts *types.TipSet) (cid.Cid, cid.Cid, error) {
	done := metrics.Timer(ctx, metrics.VMApplyBlocksTotal)
	defer done()

	partDone := metrics.Timer(ctx, metrics.VMApplyEarly)
	defer func() {
		partDone()
	}()

	ctx = blockstore.WithHotView(ctx)
	makeVm := func(base cid.Cid, e abi.ChainEpoch, timestamp uint64) (vm.Interface, error) {
		vmopt := &vm.VMOpts{
			StateBase:      base,
			Epoch:          e,
			Timestamp:      timestamp,
			Rand:           r,
			Bstore:         sm.ChainStore().StateBlockstore(),
			Actors:         NewActorRegistry(),
			Syscalls:       sm.Syscalls,
			CircSupplyCalc: sm.GetVMCirculatingSupply,
			NetworkVersion: sm.GetNetworkVersion(ctx, e),
			BaseFee:        baseFee,
			LookbackState:  stmgr.LookbackStateGetterForTipset(sm, ts),
			TipSetGetter:   stmgr.TipSetGetterForTipset(sm.ChainStore(), ts),
			Tracing:        vmTracing,
			ReturnEvents:   sm.ChainStore().IsStoringEvents(),
			ExecutionLane:  vm.ExecutionLanePriority,
		}

		return sm.VMConstructor()(ctx, vmopt)
	}

	var cronGas int64

	runCron := func(vmCron vm.Interface, epoch abi.ChainEpoch) error {
		cronMsg := &types.Message{
			To:         cron.Address,
			From:       builtin.SystemActorAddr,
			Nonce:      uint64(epoch),
			Value:      types.NewInt(0),
			GasFeeCap:  types.NewInt(0),
			GasPremium: types.NewInt(0),
			GasLimit:   buildconstants.BlockGasLimit * 10000, // Make super sure this is never too little
			Method:     cron.Methods.EpochTick,
			Params:     nil,
		}
		ret, err := vmCron.ApplyImplicitMessage(ctx, cronMsg)
		if err != nil {
			return xerrors.Errorf("running cron: %w", err)
		}

		if !ret.ExitCode.IsSuccess() {
			return xerrors.Errorf("cron failed with exit code %d: %w", ret.ExitCode, ret.ActorErr)
		}

		cronGas += ret.GasUsed

		if em != nil {
			if err := em.MessageApplied(ctx, ts, cronMsg.Cid(), cronMsg, ret, true); err != nil {
				return xerrors.Errorf("callback failed on cron message: %w", err)
			}
		}

		return nil
	}

	// May get filled with the genesis block header if there are null rounds
	// for which to backfill cron execution.
	var genesis *types.BlockHeader

	// There were null rounds in between the current epoch and the parent epoch.
	for i := parentEpoch; i < epoch; i++ {
		var err error
		if i > parentEpoch {
			if genesis == nil {
				if genesis, err = sm.ChainStore().GetGenesis(ctx); err != nil {
					return cid.Undef, cid.Undef, xerrors.Errorf("failed to get genesis when backfilling null rounds: %w", err)
				}
			}

			ts := genesis.Timestamp + buildconstants.BlockDelaySecs*(uint64(i))
			vmCron, err := makeVm(pstate, i, ts)
			if err != nil {
				return cid.Undef, cid.Undef, xerrors.Errorf("making cron vm: %w", err)
			}

			// run cron for null rounds if any
			if err = runCron(vmCron, i); err != nil {
				return cid.Undef, cid.Undef, xerrors.Errorf("running cron: %w", err)
			}

			pstate, err = vmCron.Flush(ctx)
			if err != nil {
				return cid.Undef, cid.Undef, xerrors.Errorf("flushing cron vm: %w", err)
			}
		}

		// handle state forks
		// XXX: The state tree
		pstate, err = sm.HandleStateForks(ctx, pstate, i, em, ts)
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("error handling state forks: %w", err)
		}
	}

	vmEarlyDuration := partDone()
	earlyCronGas := cronGas
	cronGas = 0
	partDone = metrics.Timer(ctx, metrics.VMApplyMessages)

	vmi, err := makeVm(pstate, epoch, ts.MinTimestamp())
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("making vm: %w", err)
	}

	var (
		receipts      []*types.MessageReceipt
		storingEvents = sm.ChainStore().IsStoringEvents()
		events        [][]types.Event
		processedMsgs = make(map[cid.Cid]struct{})
	)

	var msgGas int64

	for _, b := range bms {
		penalty := types.NewInt(0)
		gasReward := big.Zero()

		for _, cm := range append(b.BlsMessages, b.SecpkMessages...) {
			m := cm.VMMessage()
			if _, found := processedMsgs[m.Cid()]; found {
				continue
			}
			r, err := vmi.ApplyMessage(ctx, cm)
			if err != nil {
				return cid.Undef, cid.Undef, err
			}

			msgGas += r.GasUsed

			receipts = append(receipts, &r.MessageReceipt)
			gasReward = big.Add(gasReward, r.GasCosts.MinerTip)
			penalty = big.Add(penalty, r.GasCosts.MinerPenalty)

			if storingEvents {
				// Appends nil when no events are returned to preserve positional alignment.
				events = append(events, r.Events)
			}

			if em != nil {
				if err := em.MessageApplied(ctx, ts, cm.Cid(), m, r, false); err != nil {
					return cid.Undef, cid.Undef, err
				}
			}
			processedMsgs[m.Cid()] = struct{}{}
		}

		params := &reward.AwardBlockRewardParams{
			Miner:     b.Miner,
			Penalty:   penalty,
			GasReward: gasReward,
			WinCount:  b.WinCount,
		}
		rErr := t.reward(ctx, vmi, em, epoch, ts, params)
		if rErr != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("error applying reward: %w", rErr)
		}
	}

	vmMsgDuration := partDone()
	partDone = metrics.Timer(ctx, metrics.VMApplyCron)

	if err := runCron(vmi, epoch); err != nil {
		return cid.Cid{}, cid.Cid{}, err
	}

	vmCronDuration := partDone()
	partDone = metrics.Timer(ctx, metrics.VMApplyFlush)

	rectarr := blockadt.MakeEmptyArray(sm.ChainStore().ActorStore(ctx))
	for i, receipt := range receipts {
		if err := rectarr.Set(uint64(i), receipt); err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to build receipts amt: %w", err)
		}
	}
	rectroot, err := rectarr.Root()
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("failed to build receipts amt: %w", err)
	}

	// Slice will be empty if not storing events.
	for i, evs := range events {
		if len(evs) == 0 {
			continue
		}
		switch root, err := t.StoreEventsAMT(ctx, sm.ChainStore(), evs); {
		case err != nil:
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to store events amt: %w", err)
		case i >= len(receipts):
			return cid.Undef, cid.Undef, xerrors.Errorf("assertion failed: receipt and events array lengths inconsistent")
		case receipts[i].EventsRoot == nil:
			return cid.Undef, cid.Undef, xerrors.Errorf("assertion failed: VM returned events with no events root")
		case root != *receipts[i].EventsRoot:
			return cid.Undef, cid.Undef, xerrors.Errorf("assertion failed: returned events AMT root does not match derived")
		}
	}

	st, err := vmi.Flush(ctx)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("vm flush failed: %w", err)
	}

	vmFlushDuration := partDone()
	partDone = func() time.Duration { return time.Duration(0) }

	log.Infow(
		"ApplyBlocks stats",
		"earlyMs", vmEarlyDuration.Milliseconds(),
		"earlyCronGas", earlyCronGas,
		"vmMsgMs", vmMsgDuration.Milliseconds(),
		"msgGas", msgGas,
		"vmCronMs", vmCronDuration.Milliseconds(),
		"cronGas", cronGas,
		"vmFlushMs", vmFlushDuration.Milliseconds(),
		"totalMs", (vmEarlyDuration + vmMsgDuration + vmCronDuration + vmFlushDuration).Milliseconds(),
		"totalGas", earlyCronGas+msgGas+cronGas,
		"epoch", epoch,
		"tsk", ts.Key(),
	)

	stats.Record(ctx,
		metrics.VMSends.M(int64(atomic.LoadUint64(&vm.StatSends))),
		metrics.VMApplied.M(int64(atomic.LoadUint64(&vm.StatApplied))),
		metrics.VMApplyBlocksTotalGas.M(earlyCronGas+msgGas+cronGas),
		metrics.VMApplyEarlyGas.M(earlyCronGas),
		metrics.VMApplyMessagesGas.M(msgGas),
		metrics.VMApplyCronGas.M(cronGas),
	)

	return st, rectroot, nil
}

func (t *TipSetExecutor) ExecuteTipSet(ctx context.Context,
	sm *stmgr.StateManager,
	ts *types.TipSet,
	em stmgr.ExecMonitor,
	vmTracing bool) (stateroot cid.Cid, rectsroot cid.Cid, err error) {
	ctx, span := trace.StartSpan(ctx, "computeTipSetState")
	defer span.End()

	blks := ts.Blocks()

	for i := 0; i < len(blks); i++ {
		for j := i + 1; j < len(blks); j++ {
			if blks[i].Miner == blks[j].Miner {
				return cid.Undef, cid.Undef,
					xerrors.Errorf("duplicate miner in a tipset (%s %s)",
						blks[i].Miner, blks[j].Miner)
			}
		}
	}

	if ts.Height() == 0 {
		// NB: This is here because the process that executes blocks requires that the
		// block miner reference a valid miner in the state tree. Unless we create some
		// magical genesis miner, this won't work properly, so we short circuit here
		// This avoids the question of 'who gets paid the genesis block reward'
		return blks[0].ParentStateRoot, blks[0].ParentMessageReceipts, nil
	}

	var parentEpoch abi.ChainEpoch
	pstate := blks[0].ParentStateRoot
	if blks[0].Height > 0 {
		parent, err := sm.ChainStore().GetBlock(ctx, blks[0].Parents[0])
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("getting parent block: %w", err)
		}

		parentEpoch = parent.Height
	}

	r := rand.NewStateRand(sm.ChainStore(), ts.Cids(), sm.Beacon(), sm.GetNetworkVersion)

	blkmsgs, err := sm.ChainStore().BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("getting block messages for tipset: %w", err)
	}
	fbmsgs := make([]FilecoinBlockMessages, len(blkmsgs))
	for i := range fbmsgs {
		fbmsgs[i].BlockMessages = blkmsgs[i]
		fbmsgs[i].WinCount = ts.Blocks()[i].ElectionProof.WinCount
	}
	baseFee := blks[0].ParentBaseFee

	return t.ApplyBlocks(ctx, sm, parentEpoch, pstate, fbmsgs, blks[0].Height, r, em, vmTracing, baseFee, ts)
}

func (t *TipSetExecutor) StoreEventsAMT(ctx context.Context, cs *store.ChainStore, events []types.Event) (cid.Cid, error) {
	cst := cbor.NewCborStore(cs.ChainBlockstore())
	objs := make([]cbg.CBORMarshaler, len(events))
	for i := 0; i < len(events); i++ {
		objs[i] = &events[i]
	}
	return amt4.FromArray(ctx, cst, objs, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
}

var _ stmgr.Executor = &TipSetExecutor{}
