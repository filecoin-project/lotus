package filter

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

type TipSetFilter struct {
	id         types.FilterID
	maxResults int // maximum number of results to collect, 0 is unlimited
	ch         chan<- interface{}

	mu        sync.Mutex
	collected []types.TipSetKey
	lastTaken time.Time
}

var _ Filter = (*TipSetFilter)(nil)

func (f *TipSetFilter) ID() types.FilterID {
	return f.id
}

func (f *TipSetFilter) SetSubChannel(ch chan<- interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = ch
	f.collected = nil
}

func (f *TipSetFilter) ClearSubChannel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ch = nil
}

func (f *TipSetFilter) CollectTipSet(ctx context.Context, ts *types.TipSet) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// if we have a subscription channel then push tipset to it
	if f.ch != nil {
		f.ch <- ts
		return
	}

	if f.maxResults > 0 && len(f.collected) == f.maxResults {
		copy(f.collected, f.collected[1:])
		f.collected = f.collected[:len(f.collected)-1]
	}
	f.collected = append(f.collected, ts.Key())
}

func (f *TipSetFilter) TakeCollectedTipSets(context.Context) []types.TipSetKey {
	f.mu.Lock()
	collected := f.collected
	f.collected = nil
	f.lastTaken = time.Now().UTC()
	f.mu.Unlock()

	return collected
}

func (f *TipSetFilter) LastTaken() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastTaken
}

type TipSetFilterManager struct {
	MaxFilterResults int

	mu      sync.Mutex // guards mutations to filters
	filters map[types.FilterID]*TipSetFilter
}

func (m *TipSetFilterManager) Apply(ctx context.Context, from, to *types.TipSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filters) == 0 {
		return nil
	}

	// TODO: could run this loop in parallel with errgroup
	for _, f := range m.filters {
		f.CollectTipSet(ctx, to)
	}

	return nil
}

func (m *TipSetFilterManager) Revert(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (m *TipSetFilterManager) Install(ctx context.Context) (*TipSetFilter, error) {
	id, err := newFilterID()
	if err != nil {
		return nil, xerrors.Errorf("new filter id: %w", err)
	}

	f := &TipSetFilter{
		id:         id,
		maxResults: m.MaxFilterResults,
	}

	m.mu.Lock()
	if m.filters == nil {
		m.filters = make(map[types.FilterID]*TipSetFilter)
	}
	m.filters[id] = f
	m.mu.Unlock()

	return f, nil
}

func (m *TipSetFilterManager) Remove(ctx context.Context, id types.FilterID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, found := m.filters[id]; !found {
		return ErrFilterNotFound
	}
	delete(m.filters, id)
	return nil
}
