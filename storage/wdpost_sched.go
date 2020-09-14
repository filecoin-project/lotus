package storage

import (
	"context"
	"time"

	"github.com/filecoin-project/go-state-types/dline"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/node/config"

	"go.opencensus.io/trace"
)

type WindowPoStScheduler struct {
	api              storageMinerApi
	feeCfg           config.MinerFeeConfig
	prover           storage.Prover
	faultTracker     sectorstorage.FaultTracker
	proofType        abi.RegisteredPoStProof
	partitionSectors uint64
	state            *stateMachine

	actor  address.Address
	worker address.Address

	currentHighest *types.TipSet

	evtTypes [4]journal.EventType

	// failed abi.ChainEpoch // eps
	// failLk sync.Mutex
}

func NewWindowedPoStScheduler(api storageMinerApi, fc config.MinerFeeConfig, sb storage.Prover, ft sectorstorage.FaultTracker, actor address.Address, worker address.Address) (*WindowPoStScheduler, error) {
	mi, err := api.StateMinerInfo(context.TODO(), actor, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("getting sector size: %w", err)
	}

	rt, err := mi.SealProofType.RegisteredWindowPoStProof()
	if err != nil {
		return nil, err
	}

	return &WindowPoStScheduler{
		api:              api,
		feeCfg:           fc,
		prover:           sb,
		faultTracker:     ft,
		proofType:        rt,
		partitionSectors: mi.WindowPoStPartitionSectors,

		actor:  actor,
		worker: worker,
		evtTypes: [...]journal.EventType{
			evtTypeWdPoStScheduler:  journal.J.RegisterEventType("wdpost", "scheduler"),
			evtTypeWdPoStProofs:     journal.J.RegisterEventType("wdpost", "proofs_processed"),
			evtTypeWdPoStRecoveries: journal.J.RegisterEventType("wdpost", "recoveries_processed"),
			evtTypeWdPoStFaults:     journal.J.RegisterEventType("wdpost", "faults_processed"),
		},
	}, nil
}

func deadlineEquals(a, b *dline.Info) bool {
	if a == nil || b == nil {
		return b == a
	}

	return a.PeriodStart == b.PeriodStart && a.Index == b.Index && a.Challenge == b.Challenge
}

type stateMachineAPIImpl struct {
	storageMinerApi
	*WindowPoStScheduler
}

func (s *WindowPoStScheduler) Run(ctx context.Context) {
	// Initialize state machine
	smImpl := &stateMachineAPIImpl{storageMinerApi: s.api, WindowPoStScheduler: s}
	s.state = newStateMachine(smImpl, s.actor, nil)
	defer s.state.Shutdown()

	var notifs <-chan []*api.HeadChange
	var err error
	var gotCur bool

	// not fine to panic after this point
	for {
		if notifs == nil {
			notifs, err = s.api.ChainNotify(ctx)
			if err != nil {
				log.Errorf("ChainNotify error: %+v", err)

				build.Clock.Sleep(10 * time.Second)
				continue
			}

			gotCur = false
		}

		select {
		case changes, ok := <-notifs:
			if !ok {
				log.Warn("window post scheduler notifs channel closed")
				notifs = nil
				continue
			}

			if !gotCur {
				if len(changes) != 1 {
					log.Errorf("expected first notif to have len = 1")
					continue
				}
				chg := changes[0]
				if chg.Type != store.HCCurrent {
					log.Errorf("expected first notif to tell current ts")
					continue
				}

				if err := s.update(ctx, nil, chg.Val); err != nil {
					log.Errorf("%+v", err)
				}

				gotCur = true
				continue
			}

			ctx, span := trace.StartSpan(ctx, "WindowPoStScheduler.headChange")

			var lowest, highest *types.TipSet = nil, nil

			for _, change := range changes {
				if change.Val == nil {
					log.Errorf("change.Val was nil")
				}
				switch change.Type {
				case store.HCRevert:
					lowest = change.Val
				case store.HCApply:
					highest = change.Val
				}
			}

			if err := s.update(ctx, lowest, highest); err != nil {
				log.Errorf("handling head updates in window post sched: %+v", err)
			}

			span.End()
		case <-ctx.Done():
			return
		}
	}
}

func (s *WindowPoStScheduler) update(ctx context.Context, newLowest, newHighest *types.TipSet) error {
	if newHighest == nil {
		return xerrors.Errorf("no new tipset in window post WindowPoStScheduler.update")
	}

	// Check if the chain reverted to a previous proving deadline period as
	// part of a reorg
	reorged := false
	if newLowest != nil {
		// Get the current deadline period
		currentDeadline, err := s.api.StateMinerProvingDeadline(ctx, s.actor, s.currentHighest.Key())
		if err != nil {
			return err
		}

		// Get the lowest deadline period reached as part of the reorg
		lowestDeadline, err := s.api.StateMinerProvingDeadline(ctx, s.actor, newLowest.Key())
		if err != nil {
			return err
		}

		// If the reorg deadline is lower than the current deadline, we need to
		// resubmit PoST
		reorged = !deadlineEquals(currentDeadline, lowestDeadline)
	}

	s.currentHighest = newHighest

	return s.state.HeadChange(ctx, newHighest, reorged)
}

// onAbort is called when generating proofs or submitting proofs is aborted
func (s *WindowPoStScheduler) onAbort(ts *types.TipSet, deadline *dline.Info, state PoSTStatus) {
	journal.J.RecordEvent(s.evtTypes[evtTypeWdPoStScheduler], func() interface{} {
		c := evtCommon{}
		if ts != nil {
			c.Deadline = deadline
			c.Height = ts.Height()
			c.TipSet = ts.Cids()
		}
		return WdPoStSchedulerEvt{
			evtCommon: c,
			State:     SchedulerStateAborted,
		}
	})
}

// getEvtCommon populates and returns common attributes from state, for a
// WdPoSt journal event.
func (s *WindowPoStScheduler) getEvtCommon(err error) evtCommon {
	c := evtCommon{Error: err}
	currentTS, currentDeadline := s.state.CurrentTSDL()
	if currentTS != nil {
		c.Deadline = currentDeadline
		c.Height = s.state.currentTS.Height()
		c.TipSet = s.state.currentTS.Cids()
	}
	return c
}
