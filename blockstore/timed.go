package blockstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/raulk/clock"
	"go.uber.org/multierr"
)

// TimedCacheBlockstore is a blockstore that keeps blocks for at least the
// specified caching interval before discarding them. Garbage collection must
// be started and stopped by calling Start/Stop.
//
// Under the covers, it's implemented with an active and an inactive blockstore
// that are rotated every cache time interval. This means all blocks will be
// stored at most 2x the cache interval.
//
// Create a new instance by calling the NewTimedCacheBlockstore constructor.
type TimedCacheBlockstore struct {
	mu               sync.RWMutex
	active, inactive MemBlockstore
	clock            clock.Clock
	interval         time.Duration
	closeCh          chan struct{}
	doneRotatingCh   chan struct{}
}

func NewTimedCacheBlockstore(interval time.Duration) *TimedCacheBlockstore {
	b := &TimedCacheBlockstore{
		active:   NewMemory(),
		inactive: NewMemory(),
		interval: interval,
		clock:    clock.New(),
	}
	return b
}

func (t *TimedCacheBlockstore) Start(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closeCh != nil {
		return fmt.Errorf("already started")
	}
	t.closeCh = make(chan struct{})

	// Create this timer before starting the goroutine. Otherwise, creating the timer will race
	// with adding time to the mock clock, and we could add time _first_, then stall waiting for
	// a timer that'll never fire.
	ticker := t.clock.Ticker(t.interval)
	go func() {
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

func (t *TimedCacheBlockstore) Stop(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closeCh == nil {
		return fmt.Errorf("not started")
	}
	select {
	case <-t.closeCh:
		// already closed
	default:
		close(t.closeCh)
	}
	return nil
}

func (t *TimedCacheBlockstore) rotate() {
	newBs := NewMemory()

	t.mu.Lock()
	t.inactive, t.active = t.active, newBs
	t.mu.Unlock()
}

func (t *TimedCacheBlockstore) Put(b blocks.Block) error {
	// Don't check the inactive set here. We want to keep this block for at
	// least one interval.
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active.Put(b)
}

func (t *TimedCacheBlockstore) PutMany(bs []blocks.Block) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active.PutMany(bs)
}

func (t *TimedCacheBlockstore) View(k cid.Cid, callback func([]byte) error) error {
	// The underlying blockstore is always a "mem" blockstore so there's no difference,
	// from a performance perspective, between view & get. So we call Get to avoid
	// calling an arbitrary callback while holding a lock.
	t.mu.RLock()
	block, err := t.active.Get(k)
	if err == ErrNotFound {
		block, err = t.inactive.Get(k)
	}
	t.mu.RUnlock()

	if err != nil {
		return err
	}
	return callback(block.RawData())
}

func (t *TimedCacheBlockstore) Get(k cid.Cid) (blocks.Block, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	b, err := t.active.Get(k)
	if err == ErrNotFound {
		b, err = t.inactive.Get(k)
	}
	return b, err
}

func (t *TimedCacheBlockstore) GetSize(k cid.Cid) (int, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	size, err := t.active.GetSize(k)
	if err == ErrNotFound {
		size, err = t.inactive.GetSize(k)
	}
	return size, err
}

func (t *TimedCacheBlockstore) Has(k cid.Cid) (bool, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if has, err := t.active.Has(k); err != nil {
		return false, err
	} else if has {
		return true, nil
	}
	return t.inactive.Has(k)
}

func (t *TimedCacheBlockstore) HashOnRead(_ bool) {
	// no-op
}

func (t *TimedCacheBlockstore) DeleteBlock(k cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return multierr.Combine(t.active.DeleteBlock(k), t.inactive.DeleteBlock(k))
}

func (t *TimedCacheBlockstore) DeleteMany(ks []cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return multierr.Combine(t.active.DeleteMany(ks), t.inactive.DeleteMany(ks))
}

func (t *TimedCacheBlockstore) AllKeysChan(_ context.Context) (<-chan cid.Cid, error) {
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
