package shared

import (
	"context"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/jpillora/backoff"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"golang.org/x/xerrors"
)

var log = logging.Logger("data_transfer_network")

// The max number of attempts to open a stream
const defaultMaxStreamOpenAttempts = 5

// The min backoff time between retries
const defaultMinAttemptDuration = 1 * time.Second

// The max backoff time between retries
const defaultMaxAttemptDuration = 5 * time.Minute

// The multiplier in the backoff time for each retry
const defaultBackoffFactor = 5

type StreamOpener interface {
	NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error)
}

type RetryStreamOption func(*RetryStream)

// RetryParameters changes the default parameters around connection reopening
func RetryParameters(minDuration time.Duration, maxDuration time.Duration, attempts float64, backoffFactor float64) RetryStreamOption {
	return func(impl *RetryStream) {
		impl.maxStreamOpenAttempts = attempts
		impl.minAttemptDuration = minDuration
		impl.maxAttemptDuration = maxDuration
	}
}

type RetryStream struct {
	opener StreamOpener

	backoffFactor         float64
	maxStreamOpenAttempts float64
	minAttemptDuration    time.Duration
	maxAttemptDuration    time.Duration
}

func NewRetryStream(opener StreamOpener, options ...RetryStreamOption) *RetryStream {
	impl := &RetryStream{
		opener:                opener,
		backoffFactor:         defaultBackoffFactor,
		maxStreamOpenAttempts: defaultMaxStreamOpenAttempts,
		minAttemptDuration:    defaultMinAttemptDuration,
		maxAttemptDuration:    defaultMaxAttemptDuration,
	}
	impl.SetOptions(options...)
	return impl
}

func (impl *RetryStream) SetOptions(options ...RetryStreamOption) {
	for _, option := range options {
		option(impl)
	}
}

func (impl *RetryStream) OpenStream(ctx context.Context, id peer.ID, protocols []protocol.ID) (network.Stream, error) {
	b := &backoff.Backoff{
		Min:    impl.minAttemptDuration,
		Max:    impl.maxAttemptDuration,
		Factor: impl.maxStreamOpenAttempts,
		Jitter: true,
	}

	for {
		s, err := impl.opener.NewStream(ctx, id, protocols...)
		if err == nil {
			return s, err
		}

		// b.Attempt() starts from zero
		nAttempts := b.Attempt() + 1
		if nAttempts >= impl.maxStreamOpenAttempts {
			return nil, xerrors.Errorf("exhausted %d attempts but failed to open stream, err: %w", int(impl.maxStreamOpenAttempts), err)
		}

		duration := b.Duration()
		log.Warnf("failed to open stream to %s on attempt %.0f of %.0f, waiting %s to try again, err: %s",
			id, nAttempts, impl.maxStreamOpenAttempts, duration, err)

		ebt := time.NewTimer(duration)
		select {
		case <-ctx.Done():
			ebt.Stop()
			return nil, xerrors.Errorf("open stream to %s canceled by context", id)
		case <-ebt.C:
		}
	}
}
