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

	incomingTipSets chan *types.TipSet
	syncTargets     chan *types.TipSet
	syncResults     chan *syncResult

	activeSyncs map[types.TipSetKey]*types.TipSet

	syncState SyncerState

	doSync func(context.Context, *types.TipSet) error

	stop chan struct{}
}

type syncResult struct {
	ts      *types.TipSet
	success bool
}

const syncWorkerCount = 3

func NewSyncManager(sync SyncFunc) *SyncManager {
	return &SyncManager{
		bspThresh:       1,
		peerHeads:       make(map[peer.ID]*types.TipSet),
		syncTargets:     make(chan *types.TipSet),
		syncResults:     make(chan *syncResult),
		incomingTipSets: make(chan *types.TipSet),
		activeSyncs:     make(map[types.TipSetKey]*types.TipSet),
		doSync:          sync,
		stop:            make(chan struct{}),
	}
}

func (sm *SyncManager) Start() {
	go sm.syncScheduler()
	for i := 0; i < syncWorkerCount; i++ {
		go sm.syncWorker(i)
	}
}

func (sm *SyncManager) Stop() {
	close(sm.stop)
}

func (sm *SyncManager) SetPeerHead(p peer.ID, ts *types.TipSet) {
	log.Info("set peer head!")
	sm.lk.Lock()
	defer sm.lk.Unlock()
	sm.peerHeads[p] = ts

	if !sm.bootstrapped {
		log.Info("not bootstrapped")
		spc := sm.syncedPeerCount()
		if spc >= sm.bspThresh {
			log.Info("go time!")
			// Its go time!
			target, err := sm.selectSyncTarget()
			if err != nil {
				log.Error("failed to select sync target: ", err)
				return
			}

			sm.incomingTipSets <- target
			// TODO: is this the right place to say we're bootstrapped? probably want to wait until the sync finishes
			sm.bootstrapped = true
		}
		log.Infof("sync bootstrap has %d peers", spc)
		return
	}

	sm.incomingTipSets <- ts
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

	sbs.removeBucket(bestBuck)

	return bestBuck
}

func (sbs *syncBucketSet) removeBucket(toremove *syncTargetBucket) {
	nbuckets := make([]*syncTargetBucket, 0, len(sbs.buckets)-1)
	for _, b := range sbs.buckets {
		if b != toremove {
			nbuckets = append(nbuckets, b)
		}
	}
	sbs.buckets = nbuckets
}

func (sbs *syncBucketSet) PopRelated(ts *types.TipSet) *syncTargetBucket {
	for _, b := range sbs.buckets {
		if b.sameChainAs(ts) {
			sbs.removeBucket(b)
			return b
		}
	}
	return nil
}

func (sbs *syncBucketSet) Heaviest() *types.TipSet {
	// TODO: should also consider factoring in number of peers represented by each bucket here
	var bestTs *types.TipSet
	for _, b := range sbs.buckets {
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
	if stb == nil {
		return nil
	}

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
	var activeSyncTips syncBucketSet

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
			for _, acts := range sm.activeSyncs {
				if ts.Equals(acts) {
					break
				}

				if types.CidArrsEqual(ts.Parents(), acts.Cids()) {
					// sync this next, after that sync process finishes
					relatedToActiveSync = true
				}
			}

			// if this is related to an active sync process, immediately bucket it
			// we don't want to start a parallel sync process that duplicates work
			if relatedToActiveSync {
				log.Info("related to active sync")
				activeSyncTips.Insert(ts)
				continue
			}

			if nextSyncTarget != nil && nextSyncTarget.sameChainAs(ts) {
				log.Info("new tipset is part of our next sync target")
				nextSyncTarget.add(ts)
			} else {
				log.Info("insert into that queue!")
				syncQueue.Insert(ts)

				if nextSyncTarget == nil {
					nextSyncTarget = syncQueue.Pop()
					workerChan = sm.syncTargets
					log.Info("setting next sync target")
				}
			}
		case res := <-sm.syncResults:
			delete(sm.activeSyncs, res.ts.Key())
			relbucket := activeSyncTips.PopRelated(res.ts)
			if relbucket != nil {
				if res.success {
					if nextSyncTarget == nil {
						nextSyncTarget = relbucket
						workerChan = sm.syncTargets
					} else {
						syncQueue.buckets = append(syncQueue.buckets, relbucket)
					}
				} else {
					// TODO: this is the case where we try to sync a chain, and
					// fail, and we have more blocks on top of that chain that
					// have come in since.  The question is, should we try to
					// sync these? or just drop them?
				}
			}
		case workerChan <- nextSyncTarget.heaviestTipSet():
			hts := nextSyncTarget.heaviestTipSet()
			sm.activeSyncs[hts.Key()] = hts

			if len(syncQueue.buckets) > 0 {
				nextSyncTarget = syncQueue.Pop()
			} else {
				nextSyncTarget = nil
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
		case ts, ok := <-sm.syncTargets:
			if !ok {
				log.Info("sync manager worker shutting down")
				return
			}
			log.Info("sync worker go time!", ts.Cids())

			err := sm.doSync(context.TODO(), ts)
			if err != nil {
				log.Errorf("sync error: %+v", err)
			}

			sm.syncResults <- &syncResult{
				ts:      ts,
				success: err == nil,
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
