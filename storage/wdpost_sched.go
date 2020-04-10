package storage

import (
	"context"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

const StartConfidence = 4 // TODO: config

type WindowPoStScheduler struct {
	api       storageMinerApi
	prover    storage.Prover
	proofType abi.RegisteredProof

	actor  address.Address
	worker address.Address

	cur *types.TipSet

	// if a post is in progress, this indicates for which ElectionPeriodStart
	activeDeadline *Deadline
	abort          context.CancelFunc

	//failed abi.ChainEpoch // eps
	//failLk sync.Mutex
}

func NewWindowedPoStScheduler(api storageMinerApi, sb storage.Prover, actor address.Address, worker address.Address, rt abi.RegisteredProof) *WindowPoStScheduler {
	return &WindowPoStScheduler{api: api, prover: sb, actor: actor, worker: worker, proofType: rt}
}

const ProvingDeadlineEpochs = (30 * 60) / build.BlockDelay
const ProvingPeriodDeadlines = 48
const ProvingPeriodEpochs = ProvingDeadlineEpochs * ProvingDeadlineEpochs

type Deadline struct {
	// ID
	start abi.ChainEpoch
}

func (Deadline) Equals(other Deadline) bool {
	panic("maybe equal")
}

type abiPartition uint64

func (s *WindowPoStScheduler) getCurrentDeadline(ts *types.TipSet) (Deadline, error) {
	return Deadline{}, nil
}

func (s *WindowPoStScheduler) getDeadlinePartitions(ts *types.TipSet, d Deadline) ([]abiPartition, error) {
	return nil, nil
}

func (s *WindowPoStScheduler) getPartitionSectors(ts *types.TipSet, partition []abiPartition) ([]abi.SectorInfo, error) {
	// TODO: maybe make this per partition
	return nil, nil
}

func (s *WindowPoStScheduler) Run(ctx context.Context) {
	defer s.abortActivePoSt()

	var notifs <-chan []*store.HeadChange
	var err error
	var gotCur bool

	// not fine to panic after this point
	for {
		if notifs == nil {
			notifs, err = s.api.ChainNotify(ctx)
			if err != nil {
				log.Errorf("ChainNotify error: %+v")

				time.Sleep(10 * time.Second)
				continue
			}

			gotCur = false
		}

		select {
		case changes, ok := <-notifs:
			if !ok {
				log.Warn("WindowPoStScheduler notifs channel closed")
				notifs = nil
				continue
			}

			if !gotCur {
				if len(changes) != 1 {
					log.Errorf("expected first notif to have len = 1")
					continue
				}
				if changes[0].Type != store.HCCurrent {
					log.Errorf("expected first notif to tell current ts")
					continue
				}

				if err := s.update(ctx, changes[0].Val); err != nil {
					log.Errorf("%+v", err)
				}

				gotCur = true
				continue
			}

			ctx, span := trace.StartSpan(ctx, "WindowPoStScheduler.headChange")

			var lowest, highest *types.TipSet = s.cur, nil

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

			if err := s.revert(ctx, lowest); err != nil {
				log.Error("handling head reverts in fallbackPost sched: %+v", err)
			}
			if err := s.update(ctx, highest); err != nil {
				log.Error("handling head updates in fallbackPost sched: %+v", err)
			}

			span.End()
		case <-ctx.Done():
			return
		}
	}
}

func (s *WindowPoStScheduler) revert(ctx context.Context, newLowest *types.TipSet) error {
	if s.cur == newLowest {
		return nil
	}
	s.cur = newLowest

	newDeadline, err := s.getCurrentDeadline(newLowest)
	if err != nil {
		return err
	}

	if !s.activeDeadline.Equals(newDeadline) {
		s.abortActivePoSt()
	}

	return nil
}

func (s *WindowPoStScheduler) update(ctx context.Context, new *types.TipSet) error {
	if new == nil {
		return xerrors.Errorf("no new tipset in WindowPoStScheduler.update")
	}
	shouldPost, newDeadline, err := s.shouldPost(ctx, new)
	if err != nil {
		return err
	}
	if !shouldPost {
		return nil
	}

	/*s.failLk.Lock()
	if s.failed > 0 {
		s.failed = 0
		s.activeEPS = 0
	}
	s.failLk.Unlock()*/

	s.abortActivePoSt()

	if newDeadline != nil {
		s.doPost(ctx, newDeadline, new)
	}

	return nil
}

func (s *WindowPoStScheduler) abortActivePoSt() {
	if s.activeDeadline == nil {
		return // noop
	}

	if s.abort != nil {
		s.abort()
	}

	log.Warnf("Aborting Fallback PoSt (Deadline: %+v)", s.activeDeadline)

	s.activeDeadline = nil
	s.abort = nil
}

func (s *WindowPoStScheduler) shouldPost(ctx context.Context, ts *types.TipSet) (bool, *Deadline, error) {
	// call getCurrentDeadline, set activeDeadline if needed
	panic("todo check actor state for post in the deadline")
	return true, nil, nil
}
