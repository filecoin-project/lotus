package timedbs

import (
	"context"
	"fmt"
	"sync"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/raulk/clock"
	"go.uber.org/multierr"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/blockstore"
)

// TimedCacheBS is a blockstore that keeps blocks for at least the specified
// caching interval before discarding them. Garbage collection must be started
// and stopped by calling Start/Stop.
//
// Under the covers, it's implemented with an active and an inactive blockstore
// that are rotated every cache time interval. This means all blocks will be
// stored at most 2x the cache interval.
type TimedCacheBS struct {
	mu               sync.RWMutex
	active, inactive blockstore.MemStore
	clock            clock.Clock
	interval         time.Duration
	closeCh          chan struct{}
	doneRotatingCh   chan struct{}
}

func NewTimedCacheBS(cacheTime time.Duration) *TimedCacheBS {
	return &TimedCacheBS{
		active:   blockstore.NewTemporary(),
		inactive: blockstore.NewTemporary(),
		interval: cacheTime,
		clock:    build.Clock,
	}
}

func (t *TimedCacheBS) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closeCh != nil {
		return fmt.Errorf("already started")
	}
	t.closeCh = make(chan struct{})
	go func() {
		ticker := t.clock.Ticker(t.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.rotate()
				if t.doneRotatingCh != nil {
					t.doneRotatingCh <- struct{}{}
				}
			case <-t.closeCh:
				return
			}
		}
	}()
	return nil
}

func (t *TimedCacheBS) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closeCh == nil {
		return fmt.Errorf("not started started")
	}
	select {
	case <-t.closeCh:
		// already closed
	default:
		close(t.closeCh)
	}
	return nil
}

func (t *TimedCacheBS) rotate() {
	newBs := blockstore.NewTemporary()

	t.mu.Lock()
	t.inactive, t.active = t.active, newBs
	t.mu.Unlock()
}

func (t *TimedCacheBS) Put(b blocks.Block) error {
	// Don't check the inactive set here. We want to keep this block for at
	// least one interval.
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active.Put(b)
}

func (t *TimedCacheBS) PutMany(bs []blocks.Block) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active.PutMany(bs)
}

func (t *TimedCacheBS) Get(k cid.Cid) (blocks.Block, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	b, err := t.active.Get(k)
	if err == blockstore.ErrNotFound {
		b, err = t.inactive.Get(k)
	}
	return b, err
}

func (t *TimedCacheBS) GetSize(k cid.Cid) (int, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	size, err := t.active.GetSize(k)
	if err == blockstore.ErrNotFound {
		size, err = t.inactive.GetSize(k)
	}
	return size, err
}

func (t *TimedCacheBS) Has(k cid.Cid) (bool, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if has, err := t.active.Has(k); err != nil {
		return false, err
	} else if has {
		return true, nil
	}
	return t.inactive.Has(k)
}

func (t *TimedCacheBS) HashOnRead(_ bool) {
	// no-op
}

func (t *TimedCacheBS) DeleteBlock(k cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return multierr.Combine(t.active.DeleteBlock(k), t.inactive.DeleteBlock(k))
}

func (t *TimedCacheBS) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ch := make(chan cid.Cid, len(t.active)+len(t.inactive))
	for c := range t.active {
		ch <- c
	}
	for c := range t.inactive {
		if _, ok := t.active[c]; ok {
			continue
		}
		ch <- c
	}
	close(ch)
	return ch, nil
}
