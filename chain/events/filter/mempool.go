package filter

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

type MemPoolFilter struct {
	id string

	mu        sync.Mutex
	collected []cid.Cid
}

var _ Filter = (*MemPoolFilter)(nil)

func (f *MemPoolFilter) ID() string {
	return f.id
}

func (f *MemPoolFilter) CollectMessage(ctx context.Context, ts *types.TipSet) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// f.collected = append(f.collected, ts.Key())
}

func (f *MemPoolFilter) TakeCollectedMessages(context.Context) []cid.Cid {
	f.mu.Lock()
	collected := f.collected
	f.collected = nil
	f.mu.Unlock()

	return collected
}

type MemPoolFilterManager struct {
	mu      sync.Mutex // guards mutations to filters
	filters map[string]*MemPoolFilter
}

func (m MemPoolFilterManager) Apply(ctx context.Context, from, to *types.TipSet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filters) == 0 {
		return nil
	}

	// TODO: could run this loop in parallel with errgroup
	// for _, f := range m.filters {
	// 	// f.CollectTipSet(ctx, to)
	// }

	return nil
}

func (m *MemPoolFilterManager) Revert(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (m *MemPoolFilterManager) Install(ctx context.Context) (*MemPoolFilter, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("new uuid: %w", err)
	}

	f := &MemPoolFilter{
		id: id.String(),
	}

	m.mu.Lock()
	m.filters[id.String()] = f
	m.mu.Unlock()

	return f, nil
}

func (m *MemPoolFilterManager) Remove(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, found := m.filters[id]; !found {
		return ErrFilterNotFound
	}
	delete(m.filters, id)
	return nil
}
