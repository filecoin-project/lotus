package storage

import (
	"context"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-storage/storage"

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

func NewWindowedPoStScheduler(api storageMinerApi, sb storage.Prover, actor address.Address, worker address.Address) (*WindowPoStScheduler, error) {
	mi, err := api.StateMinerInfo(context.TODO(), actor, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("getting sector size: %w", err)
	}

	spt, err := ffiwrapper.SealProofTypeFromSectorSize(mi.SectorSize)
	if err != nil {
		return nil, err
	}

	rt, err := spt.RegisteredWindowPoStProof()
	if err != nil {
		return nil, err
	}

	return &WindowPoStScheduler{api: api, prover: sb, actor: actor, worker: worker, proofType: rt}, nil
}

type Deadline struct {
	provingPeriodStart abi.ChainEpoch
	deadlineIdx        uint64
	challengeEpoch     abi.ChainEpoch
}

func (d *Deadline) Equals(other *Deadline) bool {
	if d == nil || other == nil {
		return d == other
	}

	return d.provingPeriodStart == other.provingPeriodStart &&
		d.deadlineIdx == other.deadlineIdx
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

	mi, err := s.api.StateMinerInfo(ctx, s.actor, newLowest.Key())
	if err != nil {
		return err
	}

	newDeadline := deadlineInfo(mi, newLowest)

	if !s.activeDeadline.Equals(newDeadline) {
		s.abortActivePoSt()
	}

	return nil
}

func (s *WindowPoStScheduler) update(ctx context.Context, new *types.TipSet) error {
	if new == nil {
		return xerrors.Errorf("no new tipset in WindowPoStScheduler.update")
	}

	mi, err := s.api.StateMinerInfo(ctx, s.actor, new.Key())
	if err != nil {
		return err
	}

	di := deadlineInfo(mi, new)
	if s.activeDeadline.Equals(di) {
		return nil // already working on this deadline
	}
	if di == nil {
		return nil // not proving anything yet
	}

	s.abortActivePoSt()

	if di.challengeEpoch+StartConfidence >= new.Height() {
		log.Info("not starting windowPost yet, waiting for startconfidence", di.challengeEpoch, di.challengeEpoch+StartConfidence, new.Height())
		return nil
	}

	/*s.failLk.Lock()
	if s.failed > 0 {
		s.failed = 0
		s.activeEPS = 0
	}
	s.failLk.Unlock()*/

	s.doPost(ctx, di, new)

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

func deadlineInfo(mi miner.MinerInfo, new *types.TipSet) *Deadline {
	pps, nonNegative := provingPeriodStart(mi, new.Height())
	if !nonNegative {
		return nil // proving didn't start yet
	}

	deadlineIdx, challengeEpoch := miner.ComputeCurrentDeadline(pps, new.Height())

	return &Deadline{
		provingPeriodStart: pps,
		deadlineIdx:        deadlineIdx,
		challengeEpoch:     challengeEpoch,
	}
}

func provingPeriodStart(mi miner.MinerInfo, currEpoch abi.ChainEpoch) (period abi.ChainEpoch, nonNegative bool) {
	return (&miner.State{Info: mi}).ProvingPeriodStart(currEpoch)
}
