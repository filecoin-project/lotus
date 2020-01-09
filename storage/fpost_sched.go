package storage

import (
	"context"
	"sync"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

const Inactive = 0

const StartConfidence = 4 // TODO: config

type fpostScheduler struct {
	api storageMinerApi
	sb  *sectorbuilder.SectorBuilder

	actor  address.Address
	worker address.Address

	cur *types.TipSet

	// if a post is in progress, this indicates for which ElectionPeriodStart
	activeEPS uint64
	abort     context.CancelFunc

	failed uint64 // eps
	failLk sync.Mutex
}

func (s *fpostScheduler) run(ctx context.Context) {
	notifs, err := s.api.ChainNotify(ctx)
	if err != nil {
		return
	}

	current := <-notifs
	if len(current) != 1 {
		panic("expected first notif to have len = 1")
	}
	if current[0].Type != store.HCCurrent {
		panic("expected first notif to tell current ts")
	}

	if err := s.update(ctx, current[0].Val); err != nil {
		panic(err)
	}

	defer s.abortActivePoSt()

	// not fine to panic after this point
	for {
		select {
		case changes, ok := <-notifs:
			if !ok {
				log.Warn("fpostScheduler notifs channel closed")
				return
			}

			ctx, span := trace.StartSpan(ctx, "fpostScheduler.headChange")

			var lowest, highest *types.TipSet = s.cur, nil

			for _, change := range changes {
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

func (s *fpostScheduler) revert(ctx context.Context, newLowest *types.TipSet) error {
	if s.cur == newLowest {
		return nil
	}
	s.cur = newLowest

	newEPS, _, err := s.shouldFallbackPost(ctx, newLowest)
	if err != nil {
		return err
	}

	if newEPS != s.activeEPS {
		s.abortActivePoSt()
	}

	return nil
}

func (s *fpostScheduler) update(ctx context.Context, new *types.TipSet) error {
	newEPS, start, err := s.shouldFallbackPost(ctx, new)
	if err != nil {
		return err
	}

	s.failLk.Lock()
	if s.failed > 0 {
		s.failed = 0
		s.activeEPS = 0
	}
	s.failLk.Unlock()

	if newEPS == s.activeEPS {
		return nil
	}

	s.abortActivePoSt()

	if newEPS != Inactive && start {
		s.doPost(ctx, newEPS, new)
	}

	return nil
}

func (s *fpostScheduler) abortActivePoSt() {
	if s.activeEPS == Inactive {
		return // noop
	}

	if s.abort != nil {
		s.abort()
	}

	log.Warnf("Aborting Fallback PoSt (EPS: %d)", s.activeEPS)

	s.activeEPS = Inactive
	s.abort = nil
}

func (s *fpostScheduler) shouldFallbackPost(ctx context.Context, ts *types.TipSet) (uint64, bool, error) {
	eps, err := s.api.StateMinerElectionPeriodStart(ctx, s.actor, ts)
	if err != nil {
		return 0, false, xerrors.Errorf("getting ElectionPeriodStart: %w", err)
	}

	if ts.Height() >= eps+build.FallbackPoStDelay {
		return eps, ts.Height() >= eps+build.FallbackPoStDelay+StartConfidence, nil
	}
	return 0, false, nil
}
