package filter

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

type TipSetFilter struct {
	id string

	mu        sync.Mutex
	collected []types.TipSetKey
}

var _ Filter = (*TipSetFilter)(nil)

func (f *TipSetFilter) ID() string {
	return f.id
}

func (f *TipSetFilter) CollectTipSet(ctx context.Context, ts *types.TipSet) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.collected = append(f.collected, ts.Key())
}

func (f *TipSetFilter) TakeCollectedTipSets(context.Context) []types.TipSetKey {
	f.mu.Lock()
	collected := f.collected
	f.collected = nil
	f.mu.Unlock()

	return collected
}

// TODO: implement TipSetFilterManager hooking into events as observer

type TipSetFilterManager struct {
	mu      sync.Mutex // guards mutations to filters
	filters map[string]*TipSetFilter
}

func (m TipSetFilterManager) Apply(ctx context.Context, from, to *types.TipSet) error {
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
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("new uuid: %w", err)
	}

	f := &TipSetFilter{
		id: id.String(),
	}

	m.mu.Lock()
	m.filters[id.String()] = f
	m.mu.Unlock()

	return f, nil
}

func (m *TipSetFilterManager) Remove(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, found := m.filters[id]; !found {
		return ErrFilterNotFound
	}
	delete(m.filters, id)
	return nil
}
