package filter

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type MemPoolFilter struct {
	id         types.FilterID
	maxResults int // maximum number of results to collect, 0 is unlimited
	ch         chan<- interface{}

	mu        sync.Mutex
	collected []*types.SignedMessage
	lastTaken time.Time
}

var _ Filter = (*MemPoolFilter)(nil)

func (f *MemPoolFilter) ID() types.FilterID {
	return f.id
}

func (f *MemPoolFilter) SetSubChannel(ch chan<- interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = ch
	f.collected = nil
}

func (f *MemPoolFilter) ClearSubChannel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = nil
}

func (f *MemPoolFilter) CollectMessage(ctx context.Context, msg *types.SignedMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// if we have a subscription channel then push message to it
	if f.ch != nil {
		f.ch <- msg
		return
	}

	if f.maxResults > 0 && len(f.collected) == f.maxResults {
		copy(f.collected, f.collected[1:])
		f.collected = f.collected[:len(f.collected)-1]
	}
	f.collected = append(f.collected, msg)
}

func (f *MemPoolFilter) TakeCollectedMessages(context.Context) []*types.SignedMessage {
	f.mu.Lock()
	collected := f.collected
	f.collected = nil
	f.lastTaken = time.Now().UTC()
	f.mu.Unlock()

	return collected
}

func (f *MemPoolFilter) LastTaken() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastTaken
}

type MemPoolFilterManager struct {
	MaxFilterResults int

	mu      sync.Mutex // guards mutations to filters
	filters map[types.FilterID]*MemPoolFilter
}

func (m *MemPoolFilterManager) WaitForMpoolUpdates(ctx context.Context, ch <-chan api.MpoolUpdate) {
	for {
		select {
		case <-ctx.Done():
			return
		case u := <-ch:
			m.processUpdate(ctx, u)
		}
	}
}

func (m *MemPoolFilterManager) processUpdate(ctx context.Context, u api.MpoolUpdate) {
	// only process added messages
	if u.Type == api.MpoolRemove {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.filters) == 0 {
		return
	}

	// TODO: could run this loop in parallel with errgroup if we expect large numbers of filters
	for _, f := range m.filters {
		f.CollectMessage(ctx, u.Message)
	}
}

func (m *MemPoolFilterManager) Install(ctx context.Context) (*MemPoolFilter, error) {
	id, err := newFilterID()
	if err != nil {
		return nil, xerrors.Errorf("new filter id: %w", err)
	}

	f := &MemPoolFilter{
		id:         id,
		maxResults: m.MaxFilterResults,
	}

	m.mu.Lock()
	if m.filters == nil {
		m.filters = make(map[types.FilterID]*MemPoolFilter)
	}
	m.filters[id] = f
	m.mu.Unlock()

	return f, nil
}

func (m *MemPoolFilterManager) Remove(ctx context.Context, id types.FilterID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, found := m.filters[id]; !found {
		return ErrFilterNotFound
	}
	delete(m.filters, id)
	return nil
}
