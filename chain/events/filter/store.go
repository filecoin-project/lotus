package filter

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Filter interface {
	ID() string
	LastTaken() time.Time
	SetSubChannel(chan<- interface{})
	ClearSubChannel()
}

type FilterStore interface {
	Add(context.Context, Filter) error
	Get(context.Context, string) (Filter, error)
	Remove(context.Context, string) error
	NotTakenSince(when time.Time) []Filter // returns a list of filters that have not had their collected results taken
}

var (
	ErrFilterAlreadyRegistered = errors.New("filter already registered")
	ErrFilterNotFound          = errors.New("filter not found")
	ErrMaximumNumberOfFilters  = errors.New("maximum number of filters registered")
)

type memFilterStore struct {
	max     int
	mu      sync.Mutex
	filters map[string]Filter
}

var _ FilterStore = (*memFilterStore)(nil)

func NewMemFilterStore(maxFilters int) FilterStore {
	return &memFilterStore{
		max:     maxFilters,
		filters: make(map[string]Filter),
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

func (m *memFilterStore) Get(_ context.Context, id string) (Filter, error) {
	m.mu.Lock()
	f, found := m.filters[id]
	m.mu.Unlock()
	if !found {
		return nil, ErrFilterNotFound
	}
	return f, nil
}

func (m *memFilterStore) Remove(_ context.Context, id string) error {
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
