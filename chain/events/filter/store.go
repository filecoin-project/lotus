package filter

import (
	"context"
	"errors"
	"sync"
)

type Filter interface {
	ID() string
}

type FilterStore interface {
	Add(context.Context, Filter) error
	Get(context.Context, string) (Filter, error)
	Remove(context.Context, string) error
}

var (
	ErrFilterAlreadyRegistered = errors.New("filter already registered")
	ErrFilterNotFound          = errors.New("filter not found")
)

type memFilterStore struct {
	mu      sync.Mutex
	filters map[string]Filter
}

var _ FilterStore = (*memFilterStore)(nil)

func NewMemFilterStore() FilterStore {
	return &memFilterStore{
		filters: make(map[string]Filter),
	}
}

func (m *memFilterStore) Add(_ context.Context, f Filter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
