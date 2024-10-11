package lf3

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jpillora/backoff"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

const (
	// maxCheckProgressAttempts defines the maximum number of failed attempts
	// before we abandon the current lease and restart the participation process.
	//
	// The default backoff takes 12 attempts to reach a maximum delay of 1 minute.
	// Allowing for 13 failures results in approximately 2 minutes of backoff since
	// the lease was granted. Given a lease validity of up to 5 instances, this means
	// we would give up on checking the lease during its mid-validity period;
	// typically when we would try to renew the participation ticket. Hence, the value
	// to 13.
	ParticipationCheckProgressMaxAttempts = 13

	// ParticipationLeaseTerm is the number of instances the miner will attempt to lease from nodes.
	ParticipationLeaseTerm = 5
)

type F3ParticipationAPI interface {
	F3GetOrRenewParticipationTicket(ctx context.Context, minerID address.Address, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error) //perm:sign
	F3Participate(ctx context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error)
	F3GetProgress(ctx context.Context) (gpbft.Instant, error)
	F3GetManifest(ctx context.Context) (*manifest.Manifest, error)
}

type Participant struct {
	node                     F3ParticipationAPI
	participant              address.Address
	backoff                  *backoff.Backoff
	maxCheckProgressAttempts int
	previousTicket           api.F3ParticipationTicket
	leaseTerm                uint64

	runningCtx context.Context
	cancelCtx  context.CancelFunc
	errgrp     *errgroup.Group
}

func NewParticipant(ctx context.Context, node F3ParticipationAPI, participant dtypes.MinerAddress, backoff *backoff.Backoff, maxCheckProgress int, leaseTerm uint64) *Participant {
	runningCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	errgrp, runningCtx := errgroup.WithContext(runningCtx)
	return &Participant{
		node:                     node,
		participant:              address.Address(participant),
		backoff:                  backoff,
		maxCheckProgressAttempts: maxCheckProgress,
		leaseTerm:                leaseTerm,
		runningCtx:               runningCtx,
		cancelCtx:                cancel,
		errgrp:                   errgrp,
	}
}

func (p *Participant) Start(ctx context.Context) error {
	p.errgrp.Go(func() error {
		return p.run(p.runningCtx)
	})
	return nil
}

func (p *Participant) Stop(ctx context.Context) error {
	p.cancelCtx()
	return p.errgrp.Wait()
}

func (p *Participant) run(ctx context.Context) (_err error) {
	defer func() {
		if ctx.Err() != nil {
			_err = nil
		}
		if _err != nil {
			_err = fmt.Errorf("F3 participant stopped unexpectedly: %w", _err)
			log.Error(_err)
		}
	}()

	for ctx.Err() == nil {
		start := time.Now()
		ticket, err := p.tryGetF3ParticipationTicket(ctx)
		if err != nil {
			return err
		}
		lease, participating, err := p.tryF3Participate(ctx, ticket)
		if err != nil {
			return err
		}
		if participating {
			if err := p.awaitLeaseExpiry(ctx, lease); err != nil {
				return err
			}
		}
		const minPeriod = 500 * time.Millisecond
		if sinceLastLoop := time.Since(start); sinceLastLoop < minPeriod {
			select {
			case <-time.After(minPeriod - sinceLastLoop):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		log.Info("Renewing F3 participation")
	}
	return ctx.Err()
}

func (p *Participant) tryGetF3ParticipationTicket(ctx context.Context) (api.F3ParticipationTicket, error) {
	p.backoff.Reset()
	for ctx.Err() == nil {
		switch ticket, err := p.node.F3GetOrRenewParticipationTicket(ctx, p.participant, p.previousTicket, p.leaseTerm); {
		case ctx.Err() != nil:
			return api.F3ParticipationTicket{}, ctx.Err()
		case errors.Is(err, api.ErrF3Disabled):
			log.Errorw("Cannot participate in F3 as it is disabled.", "err", err)
			return api.F3ParticipationTicket{}, xerrors.Errorf("acquiring F3 participation ticket: %w", err)
		case err != nil:
			log.Errorw("Failed to acquire F3 participation ticket; retrying after backoff", "backoff", p.backoff.Duration(), "err", err)
			p.backOff(ctx)
			log.Debugw("Reattempting to acquire F3 participation ticket.", "attempts", p.backoff.Attempt())
			continue
		default:
			log.Debug("Successfully acquired F3 participation ticket")
			return ticket, nil
		}
	}
	return api.F3ParticipationTicket{}, ctx.Err()
}

func (p *Participant) tryF3Participate(ctx context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, bool, error) {
	p.backoff.Reset()
	for ctx.Err() == nil {
		switch lease, err := p.node.F3Participate(ctx, ticket); {
		case ctx.Err() != nil:
			return api.F3ParticipationLease{}, false, ctx.Err()
		case errors.Is(err, api.ErrF3Disabled):
			log.Errorw("Cannot participate in F3 as it is disabled.", "err", err)
			return api.F3ParticipationLease{}, false, xerrors.Errorf("attempting F3 participation with ticket: %w", err)
		case errors.Is(err, api.ErrF3ParticipationTicketExpired):
			log.Warnw("F3 participation ticket expired while attempting to participate. Acquiring a new ticket.", "attempts", p.backoff.Attempt(), "err", err)
			return api.F3ParticipationLease{}, false, nil
		case errors.Is(err, api.ErrF3ParticipationTicketStartBeforeExisting):
			log.Warnw("F3 participation ticket starts before the existing lease. Acquiring a new ticket.", "attempts", p.backoff.Attempt(), "err", err)
			return api.F3ParticipationLease{}, false, nil
		case errors.Is(err, api.ErrF3ParticipationTicketInvalid):
			log.Errorw("F3 participation ticket is not valid. Acquiring a new ticket after backoff.", "backoff", p.backoff.Duration(), "attempts", p.backoff.Attempt(), "err", err)
			p.backOff(ctx)
			return api.F3ParticipationLease{}, false, nil
		case errors.Is(err, api.ErrF3ParticipationIssuerMismatch):
			log.Warnw("Node is not the issuer of F3 participation ticket. Miner maybe load-balancing or node has changed. Retrying F3 participation after backoff.", "backoff", p.backoff.Duration(), "err", err)
			p.backOff(ctx)
			log.Debugw("Reattempting F3 participation with the same ticket.", "attempts", p.backoff.Attempt())
			continue
		case errors.Is(err, api.ErrF3NotReady):
			log.Warnw("F3 is not ready. Retrying F3 participation after backoff.", "backoff", p.backoff.Duration(), "err", err)
			p.backOff(ctx)
			continue
		case err != nil:
			log.Errorw("Unexpected error while attempting F3 participation. Retrying after backoff", "backoff", p.backoff.Duration(), "attempts", p.backoff.Attempt(), "err", err)
			p.backOff(ctx)
			continue
		default:
			log.Infow("Successfully acquired F3 participation lease.",
				"issuer", lease.Issuer,
				"not-before", lease.FromInstance,
				"not-after", lease.ToInstance(),
			)
			p.previousTicket = ticket
			return lease, true, nil
		}
	}
	return api.F3ParticipationLease{}, false, ctx.Err()
}

func (p *Participant) awaitLeaseExpiry(ctx context.Context, lease api.F3ParticipationLease) error {
	p.backoff.Reset()
	renewLeaseWithin := p.leaseTerm / 2
	for ctx.Err() == nil {
		manifest, err := p.node.F3GetManifest(ctx)
		switch {
		case errors.Is(err, api.ErrF3Disabled):
			log.Errorw("Cannot await F3 participation lease expiry as F3 is disabled.", "err", err)
			return xerrors.Errorf("awaiting F3 participation lease expiry: %w", err)
		case err != nil:
			if p.backoff.Attempt() > float64(p.maxCheckProgressAttempts) {
				log.Errorw("Too many failures while attempting to check F3  progress. Restarting participation.", "attempts", p.backoff.Attempt(), "err", err)
				return nil
			}
			log.Errorw("Failed to check F3 progress while awaiting lease expiry. Retrying after backoff.", "attempts", p.backoff.Attempt(), "backoff", p.backoff.Duration(), "err", err)
			p.backOff(ctx)
		case manifest == nil || manifest.NetworkName != lease.Network:
			// If we got an unexpected manifest, or no manifest, go back to the
			// beginning and try to get another ticket. Switching from having a manifest
			// to having no manifest can theoretically happen if the lotus node reboots
			// and has no static manifest.
			return nil
		}
		switch progress, err := p.node.F3GetProgress(ctx); {
		case errors.Is(err, api.ErrF3Disabled):
			log.Errorw("Cannot await F3 participation lease expiry as F3 is disabled.", "err", err)
			return xerrors.Errorf("awaiting F3 participation lease expiry: %w", err)
		case err != nil:
			if p.backoff.Attempt() > float64(p.maxCheckProgressAttempts) {
				log.Errorw("Too many failures while attempting to check F3  progress. Restarting participation.", "attempts", p.backoff.Attempt(), "err", err)
				return nil
			}
			log.Errorw("Failed to check F3 progress while awaiting lease expiry. Retrying after backoff.", "attempts", p.backoff.Attempt(), "backoff", p.backoff.Duration(), "err", err)
			p.backOff(ctx)
		case progress.ID+renewLeaseWithin >= lease.ToInstance():
			log.Infof("F3 progressed (%d) to within %d instances of lease expiry (%d). Renewing participation.", progress.ID, renewLeaseWithin, lease.ToInstance())
			return nil
		default:
			remainingInstanceLease := lease.ToInstance() - progress.ID
			waitTime := time.Duration(remainingInstanceLease-renewLeaseWithin) * manifest.CatchUpAlignment
			if waitTime == 0 {
				waitTime = 100 * time.Millisecond
			}
			log.Debugf("F3 participation lease is valid for further %d instances. Re-checking after %s.", remainingInstanceLease, waitTime)
			p.backOffFor(ctx, waitTime)
		}
	}
	return ctx.Err()
}

func (p *Participant) backOff(ctx context.Context) {
	p.backOffFor(ctx, p.backoff.Duration())
}

func (p *Participant) backOffFor(ctx context.Context, d time.Duration) {
	// Create a timer every time to avoid potential risk of deadlock or the need for
	// mutex despite the fact that f3Participator is never (and should never) be
	// called from multiple goroutines.
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
}
