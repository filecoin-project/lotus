package chain

import (
	"context"
	"sort"
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

	asLk        sync.Mutex
	activeSyncs map[types.TipSetKey]*types.TipSet
	queuedSyncs map[types.TipSetKey]*types.TipSet

	syncState SyncerState

	doSync func(context.Context, *types.TipSet) error

	stop chan struct{}
}

func NewSyncManager(sync SyncFunc) *SyncManager {
	return &SyncManager{
		peerHeads:   make(map[peer.ID]*types.TipSet),
		syncTargets: make(chan *types.TipSet),
		activeSyncs: make([]*types.TipSet, syncWorkerCount),
		doSync:      sync,
		stop:        make(chan struct{}),
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
			target, err := sm.selectSyncTarget()
			if err != nil {
				log.Error("failed to select sync target: ", err)
				return
			}

			sm.asLk.Lock()
			sm.activeSyncs[target.Key()] = target
			sm.asLk.Unlock()
			sm.syncTargets <- target
			sm.bootstrapped = true
		}
		log.Infof("sync bootstrap has %d peers", spc)
		return
	}

}

type syncBucketSet struct {
	buckets []*syncTargetBucket
}

func (sbs *syncBucketSet) Insert(ts *types.TipSet) {
	for _, b := range sbs.buckets {
		if b.sameChainAs(ts) {
			b.add(ts)
			return
		}
	}
	sbs.buckets = append(sbs.buckets, &syncTargetBucket{
		tips:  []*types.TipSet{ts},
		count: 1,
	})
}

func (sbs *syncBucketSet) Pop() *syncTargetBucket {
	var bestBuck *syncTargetBucket
	var bestTs *types.TipSet
	for _, b := range sbs.buckets {
		hts := b.heaviestTipSet()
		if bestBuck == nil || bestTs.ParentWeight().LessThan(hts.ParentWeight()) {
			bestBuck = b
			bestTs = hts
		}
	}
	nbuckets := make([]*syncTargetBucket, len(sbs.buckets)-1)
	return bestBuck
}

func (sbs *syncBucketSet) Heaviest() *types.TipSet {
	// TODO: should also consider factoring in number of peers represented by each bucket here
	var bestTs *types.TipSet
	for _, b := range buckets {
		bhts := b.heaviestTipSet()
		if bestTs == nil || bhts.ParentWeight().GreaterThan(bestTs.ParentWeight()) {
			bestTs = bhts
		}
	}
	return bestTs
}

type syncTargetBucket struct {
	tips  []*types.TipSet
	count int
}

func newSyncTargetBucket(tipsets ...*types.TipSet) *syncTargetBucket {
	var stb syncTargetBucket
	for _, ts := range tipsets {
		stb.add(ts)
	}
	return &stb
}

func (stb *syncTargetBucket) sameChainAs(ts *types.TipSet) bool {
	for _, t := range stb.tips {
		if ts.Equals(t) {
			return true
		}
		if types.CidArrsEqual(ts.Cids(), t.Parents()) {
			return true
		}
		if types.CidArrsEqual(ts.Parents(), t.Cids()) {
			return true
		}
	}
	return false
}

func (stb *syncTargetBucket) add(ts *types.TipSet) {
	stb.count++

	for _, t := range stb.tips {
		if t.Equals(ts) {
			return
		}
	}

	stb.tips = append(stb.tips, ts)
}

func (stb *syncTargetBucket) heaviestTipSet() *types.TipSet {
	var best *types.TipSet
	for _, ts := range stb.tips {
		if best == nil || ts.ParentWeight().GreaterThan(best.ParentWeight()) {
			best = ts
		}
	}
	return best
}

func (sm *SyncManager) selectSyncTarget() (*types.TipSet, error) {
	var buckets syncBucketSet

	var peerHeads []*types.TipSet
	for _, ts := range sm.peerHeads {
		peerHeads = append(peerHeads, ts)
	}
	sort.Slice(peerHeads, func(i, j int) bool {
		return peerHeads[i].Height() < peerHeads[j].Height()
	})

	for _, ts := range peerHeads {
		buckets.Insert(ts)
	}

	if len(buckets.buckets) > 1 {
		log.Warning("caution, multiple distinct chains seen during head selections")
		// TODO: we *could* refuse to sync here without user intervention.
		// For now, just select the best cluster
	}

	return buckets.Heaviest(), nil
}

func (sm *SyncManager) syncScheduler() {
	var syncQueue syncBucketSet

	var nextSyncTarget *syncTargetBucket
	var workerChan chan *types.TipSet

	for {
		select {
		case ts, ok := <-sm.incomingTipSets:
			if !ok {
				log.Info("shutting down sync scheduler")
				return
			}

			var relatedToActiveSync bool
			sm.asLk.Lock()
			for _, acts := range sm.activeSyncs {
				if ts.Equals(acts) {
					break
				}

				if types.CidArrsEqual(ts.Parents(), acts.Cids()) {
					// sync this next, after that sync process finishes
					relatedToActiveSync = true
				}
			}
			sm.asLk.Unlock()

			// if this is related to an active sync process, immediately bucket it
			// we don't want to start a parallel sync process that duplicates work
			if relatedToActiveSync {
				syncQueue.Insert(ts)
			}

			if nextSyncTarget != nil && nextSyncTarget.sameChainAs(ts) {
				nextSyncTarget.add(ts)
			} else {
				syncQueue.Insert(ts)

				if nextSyncTarget == nil {
					nextSyncTarget = syncQueue.Pop()
					workerChan = workerChanVal
				}
			}
		case workerChan <- nextSyncTarget.heaviestTipSet():
			if len(syncQueue.buckets) > 0 {
				nextSyncTarget = syncQueue.Pop()
			} else {
				workerChan = nil
			}
		case <-sm.stop:
			log.Info("sync scheduler shutting down")
			return
		}
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

			sm.asLk.Lock()
			delete(sm.activeSyncs, ts.Key())
			sm.asLk.Unlock()
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
