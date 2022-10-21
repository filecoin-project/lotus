package filter

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
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

// TODO: implement MemPoolFilterManager reading from mempool

type MemPoolFilterManager struct {
	mu      sync.Mutex // guards mutations to filters
	filters map[string]*MemPoolFilter
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
