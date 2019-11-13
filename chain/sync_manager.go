package chain

import (
	"context"
	"sync"

	"github.com/filecoin-project/lotus/chain/types"
	peer "github.com/libp2p/go-libp2p-peer"
)

const BootstrapPeerThreshold = 2

type SyncFunc func(context.Context, *types.TipSet) error

type SyncManager struct {
	lk           sync.Mutex
	peerHeads    map[peer.ID]*types.TipSet
	bootstrapped bool

	bspThresh int

	syncTargets chan *types.TipSet

	doSync func(context.Context, *types.TipSet) error
}

func NewSyncManager(sync SyncFunc) *SyncManager {
	return &SyncManager{
		peerHeads:   make(map[peer.ID]*types.TipSet),
		syncTargets: make(chan *types.TipSet),
		doSync:      sync,
	}
}

func (sm *SyncManager) Start() {
	for i := 0; i < syncWorkerCount; i++ {
		go sm.syncWorker(i)
	}
}

func (sm *SyncManager) SetPeerHead(p peer.ID, ts *types.TipSet) {
	sm.lk.Lock()
	defer sm.lk.Unlock()
	sm.peerHeads[p] = ts

	if !sm.bootstrapped {
		spc := sm.syncedPeerCount()
		if spc >= sm.bspThresh {
			// Its go time!
		}
		log.Infof("sync bootstrap has %d peers", spc)
		return
	}

}

func (sm *SyncManager) syncWorker(id int) {
	for {
		select {
		case ts, ok := sm.syncTargets:
			if !ok {
				log.Info("sync manager worker shutting down")
				return
			}

			if err := sm.doSync(context.TODO(), ts); err != nil {
				log.Errorf("sync error: %+v", err)
			}
		}
	}
}

func (sm *SyncManager) syncedPeerCount() int {
	var count int
	for _, ts := range sm.peerHeads {
		if ts.Height() > 0 {
			count++
		}
	}
	return count
}

func (sm *SyncManager) IsBootstrapped() bool {
	sm.lk.Lock()
	defer sm.lk.Unlock()
	return sm.bootstrapped
}
