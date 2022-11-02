package filter

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type MemPoolFilter struct {
	id         string
	maxResults int // maximum number of results to collect, 0 is unlimited

	mu        sync.Mutex
	collected []cid.Cid
	lastTaken time.Time
}

var _ Filter = (*MemPoolFilter)(nil)

func (f *MemPoolFilter) ID() string {
	return f.id
}

func (f *MemPoolFilter) CollectMessage(ctx context.Context, msg *types.SignedMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.maxResults > 0 && len(f.collected) == f.maxResults {
		copy(f.collected, f.collected[1:])
		f.collected = f.collected[:len(f.collected)-1]
	}
	f.collected = append(f.collected, msg.Cid())
}

func (f *MemPoolFilter) TakeCollectedMessages(context.Context) []cid.Cid {
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
	filters map[string]*MemPoolFilter
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
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, xerrors.Errorf("new uuid: %w", err)
	}

	f := &MemPoolFilter{
		id:         id.String(),
		maxResults: m.MaxFilterResults,
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
