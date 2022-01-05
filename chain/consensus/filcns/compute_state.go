package filcns

import (
	"context"
	"sync/atomic"

	"github.com/filecoin-project/lotus/chain/rand"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	/* inline-gen template
	   {{range .actorVersions}}
	   	exported{{.}} "github.com/filecoin-project/specs-actors{{import .}}actors/builtin/exported"{{end}}

	/* inline-gen start */

	exported0 "github.com/filecoin-project/specs-actors/actors/builtin/exported"
	exported2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/exported"
	exported3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/exported"
	exported4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/exported"
	exported5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/exported"
	exported6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/exported"
	exported7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/exported"

	/* inline-gen end */

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/cron"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/metrics"
)

func NewActorRegistry() *vm.ActorRegistry {
	inv := vm.NewActorRegistry()

	// TODO: define all these properties on the actors themselves, in specs-actors.
	/* inline-gen template
	{{range .actorVersions}}
	inv.Register(vm.ActorsVersionPredicate(actors.Version{{.}}), exported{{.}}.BuiltinActors()...){{end}}

	/* inline-gen start */

	inv.Register(vm.ActorsVersionPredicate(actors.Version0), exported0.BuiltinActors()...)
	inv.Register(vm.ActorsVersionPredicate(actors.Version2), exported2.BuiltinActors()...)
	inv.Register(vm.ActorsVersionPredicate(actors.Version3), exported3.BuiltinActors()...)
	inv.Register(vm.ActorsVersionPredicate(actors.Version4), exported4.BuiltinActors()...)
	inv.Register(vm.ActorsVersionPredicate(actors.Version5), exported5.BuiltinActors()...)
	inv.Register(vm.ActorsVersionPredicate(actors.Version6), exported6.BuiltinActors()...)
	inv.Register(vm.ActorsVersionPredicate(actors.Version7), exported7.BuiltinActors()...)

	/* inline-gen end */

	return inv
}

type TipSetExecutor struct{}

func NewTipSetExecutor() *TipSetExecutor {
	return &TipSetExecutor{}
}

func (t *TipSetExecutor) NewActorRegistry() *vm.ActorRegistry {
	return NewActorRegistry()
}

type FilecoinBlockMessages struct {
	store.BlockMessages

	WinCount int64
}

func (t *TipSetExecutor) ApplyBlocks(ctx context.Context, sm *stmgr.StateManager, parentEpoch abi.ChainEpoch, pstate cid.Cid, bms []FilecoinBlockMessages, epoch abi.ChainEpoch, r vm.Rand, em stmgr.ExecMonitor, baseFee abi.TokenAmount, ts *types.TipSet) (cid.Cid, cid.Cid, error) {
	done := metrics.Timer(ctx, metrics.VMApplyBlocksTotal)
	defer done()

	partDone := metrics.Timer(ctx, metrics.VMApplyEarly)
	defer func() {
		partDone()
	}()

	makeVmWithBaseState := func(base cid.Cid) (*vm.VM, error) {
		vmopt := &vm.VMOpts{
			StateBase:      base,
			Epoch:          epoch,
			Rand:           r,
			Bstore:         sm.ChainStore().StateBlockstore(),
			Actors:         NewActorRegistry(),
			Syscalls:       sm.Syscalls,
			CircSupplyCalc: sm.GetVMCirculatingSupply,
			NtwkVersion:    sm.GetNtwkVersion,
			BaseFee:        baseFee,
			LookbackState:  stmgr.LookbackStateGetterForTipset(sm, ts),
		}

		return sm.VMConstructor()(ctx, vmopt)
	}

	vmi, err := makeVmWithBaseState(pstate)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("making vm: %w", err)
	}

	runCron := func(epoch abi.ChainEpoch) error {
		cronMsg := &types.Message{
			To:         cron.Address,
			From:       builtin.SystemActorAddr,
			Nonce:      uint64(epoch),
			Value:      types.NewInt(0),
			GasFeeCap:  types.NewInt(0),
			GasPremium: types.NewInt(0),
			GasLimit:   build.BlockGasLimit * 10000, // Make super sure this is never too little
			Method:     cron.Methods.EpochTick,
			Params:     nil,
		}
		ret, err := vmi.ApplyImplicitMessage(ctx, cronMsg)
		if err != nil {
			return err
		}
		if em != nil {
			if err := em.MessageApplied(ctx, ts, cronMsg.Cid(), cronMsg, ret, true); err != nil {
				return xerrors.Errorf("callback failed on cron message: %w", err)
			}
		}
		if ret.ExitCode != 0 {
			return xerrors.Errorf("CheckProofSubmissions exit was non-zero: %d", ret.ExitCode)
		}

		return nil
	}

	for i := parentEpoch; i < epoch; i++ {
		if i > parentEpoch {
			// run cron for null rounds if any
			if err := runCron(i); err != nil {
				return cid.Undef, cid.Undef, err
			}

			pstate, err = vmi.Flush(ctx)
			if err != nil {
				return cid.Undef, cid.Undef, xerrors.Errorf("flushing vm: %w", err)
			}
		}

		// handle state forks
		// XXX: The state tree
		newState, err := sm.HandleStateForks(ctx, pstate, i, em, ts)
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("error handling state forks: %w", err)
		}

		if pstate != newState {
			vmi, err = makeVmWithBaseState(newState)
			if err != nil {
				return cid.Undef, cid.Undef, xerrors.Errorf("making vm: %w", err)
			}
		}

		if err = vmi.SetBlockHeight(ctx, i+1); err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("error advancing vm an epoch: %w", err)
		}

		pstate = newState
	}

	partDone()
	partDone = metrics.Timer(ctx, metrics.VMApplyMessages)

	var receipts []cbg.CBORMarshaler
	processedMsgs := make(map[cid.Cid]struct{})
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

			receipts = append(receipts, &r.MessageReceipt)
			gasReward = big.Add(gasReward, r.GasCosts.MinerTip)
			penalty = big.Add(penalty, r.GasCosts.MinerPenalty)

			if em != nil {
				if err := em.MessageApplied(ctx, ts, cm.Cid(), m, r, false); err != nil {
					return cid.Undef, cid.Undef, err
				}
			}
			processedMsgs[m.Cid()] = struct{}{}
		}

		params, err := actors.SerializeParams(&reward.AwardBlockRewardParams{
			Miner:     b.Miner,
			Penalty:   penalty,
			GasReward: gasReward,
			WinCount:  b.WinCount,
		})
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to serialize award params: %w", err)
		}

		rwMsg := &types.Message{
			From:       builtin.SystemActorAddr,
			To:         reward.Address,
			Nonce:      uint64(epoch),
			Value:      types.NewInt(0),
			GasFeeCap:  types.NewInt(0),
			GasPremium: types.NewInt(0),
			GasLimit:   1 << 30,
			Method:     reward.Methods.AwardBlockReward,
			Params:     params,
		}
		ret, actErr := vmi.ApplyImplicitMessage(ctx, rwMsg)
		if actErr != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("failed to apply reward message for miner %s: %w", b.Miner, actErr)
		}
		if em != nil {
			if err := em.MessageApplied(ctx, ts, rwMsg.Cid(), rwMsg, ret, true); err != nil {
				return cid.Undef, cid.Undef, xerrors.Errorf("callback failed on reward message: %w", err)
			}
		}

		if ret.ExitCode != 0 {
			return cid.Undef, cid.Undef, xerrors.Errorf("reward application message failed (exit %d): %s", ret.ExitCode, ret.ActorErr)
		}
	}

	partDone()
	partDone = metrics.Timer(ctx, metrics.VMApplyCron)

	if err := runCron(epoch); err != nil {
		return cid.Cid{}, cid.Cid{}, err
	}

	partDone()
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

	st, err := vmi.Flush(ctx)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("vm flush failed: %w", err)
	}

	stats.Record(ctx, metrics.VMSends.M(int64(atomic.LoadUint64(&vm.StatSends))),
		metrics.VMApplied.M(int64(atomic.LoadUint64(&vm.StatApplied))))

	return st, rectroot, nil
}

func (t *TipSetExecutor) ExecuteTipSet(ctx context.Context, sm *stmgr.StateManager, ts *types.TipSet, em stmgr.ExecMonitor) (stateroot cid.Cid, rectsroot cid.Cid, err error) {
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

	var parentEpoch abi.ChainEpoch
	pstate := blks[0].ParentStateRoot
	if blks[0].Height > 0 {
		parent, err := sm.ChainStore().GetBlock(blks[0].Parents[0])
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("getting parent block: %w", err)
		}

		parentEpoch = parent.Height
	}

	r := rand.NewStateRand(sm.ChainStore(), ts.Cids(), sm.Beacon())

	blkmsgs, err := sm.ChainStore().BlockMsgsForTipset(ts)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("getting block messages for tipset: %w", err)
	}
	fbmsgs := make([]FilecoinBlockMessages, len(blkmsgs))
	for i := range fbmsgs {
		fbmsgs[i].BlockMessages = blkmsgs[i]
		fbmsgs[i].WinCount = ts.Blocks()[i].ElectionProof.WinCount
	}
	baseFee := blks[0].ParentBaseFee

	return t.ApplyBlocks(ctx, sm, parentEpoch, pstate, fbmsgs, blks[0].Height, r, em, baseFee, ts)
}

var _ stmgr.Executor = &TipSetExecutor{}
