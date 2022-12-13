package shared_testutil

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"

	provider "github.com/filecoin-project/index-provider"
	"github.com/filecoin-project/index-provider/metadata"
)

type MockIndexProvider struct {
	provider.Interface

	lk       sync.Mutex
	callback provider.MultihashLister
	notifs   map[string]metadata.Metadata
}

func NewMockIndexProvider() *MockIndexProvider {
	return &MockIndexProvider{
		notifs: make(map[string]metadata.Metadata),
	}

}

func (m *MockIndexProvider) RegisterMultihashLister(cb provider.MultihashLister) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.callback = cb
}

func (m *MockIndexProvider) NotifyPut(ctx context.Context, contextID []byte, metadata metadata.Metadata) (cid.Cid, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.notifs[string(contextID)] = metadata

	return cid.Undef, nil
}

func (m *MockIndexProvider) NotifyRemove(ctx context.Context, contextID []byte) (cid.Cid, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	return cid.Undef, nil
}

func (m *MockIndexProvider) GetNotifs() map[string]metadata.Metadata {
	m.lk.Lock()
	defer m.lk.Unlock()

	return m.notifs
}
