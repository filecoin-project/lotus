package filter

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

type Filter interface {
	ID() types.FilterID
	LastTaken() time.Time
	SetSubChannel(chan<- interface{})
	ClearSubChannel()
}

type FilterStore interface {
	Add(context.Context, Filter) error
	Get(context.Context, types.FilterID) (Filter, error)
	Remove(context.Context, types.FilterID) error
	NotTakenSince(when time.Time) []Filter // returns a list of filters that have not had their collected results taken
}

var (
	ErrFilterAlreadyRegistered = errors.New("filter already registered")
	ErrFilterNotFound          = errors.New("filter not found")
	ErrMaximumNumberOfFilters  = errors.New("maximum number of filters registered")
)

func newFilterID() (types.FilterID, error) {
	rawid, err := uuid.NewRandom()
	if err != nil {
		return types.FilterID{}, xerrors.Errorf("new uuid: %w", err)
	}
	id := types.FilterID{}
	copy(id[:], rawid[:]) // uuid is 16 bytes, the last 16 bytes are zeroed
	return id, nil
}

type memFilterStore struct {
	max     int
	mu      sync.Mutex
	filters map[types.FilterID]Filter
}

var _ FilterStore = (*memFilterStore)(nil)

func NewMemFilterStore(maxFilters int) FilterStore {
	return &memFilterStore{
		max:     maxFilters,
		filters: make(map[types.FilterID]Filter),
	}
}

func (m *memFilterStore) Add(_ context.Context, f Filter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.filters) >= m.max {
		return ErrMaximumNumberOfFilters
	}

	if _, exists := m.filters[f.ID()]; exists {
		return ErrFilterAlreadyRegistered
	}
	m.filters[f.ID()] = f
	return nil
}

func (m *memFilterStore) Get(_ context.Context, id types.FilterID) (Filter, error) {
	m.mu.Lock()
	f, found := m.filters[id]
	m.mu.Unlock()
	if !found {
		return nil, ErrFilterNotFound
	}
	return f, nil
}

func (m *memFilterStore) Remove(_ context.Context, id types.FilterID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.filters[id]; !exists {
		return ErrFilterNotFound
	}
	delete(m.filters, id)
	return nil
}

func (m *memFilterStore) NotTakenSince(when time.Time) []Filter {
	m.mu.Lock()
	defer m.mu.Unlock()

	var res []Filter
	for _, f := range m.filters {
		if f.LastTaken().Before(when) {
			res = append(res, f)
		}
	}

	return res
}
