package chain

import (
	"context"
	"sort"
	"sync"

	"github.com/filecoin-project/lotus/chain/types"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

const BootstrapPeerThreshold = 2

const (
	BSStateInit      = 0
	BSStateSelected  = 1
	BSStateScheduled = 2
	BSStateComplete  = 3
)

type SyncFunc func(context.Context, *types.TipSet) error

// SyncManager manages the chain synchronization process, both at bootstrap time
// and during ongoing operation.
//
// It receives candidate chain heads in the form of tipsets from peers,
// and schedules them onto sync workers, deduplicating processing for
// already-active syncs.
type SyncManager interface {
	// Start starts the SyncManager.
	Start()

	// Stop stops the SyncManager.
	Stop()

	// SetPeerHead informs the SyncManager that the supplied peer reported the
	// supplied tipset.
	SetPeerHead(ctx context.Context, p peer.ID, ts *types.TipSet)

	// State retrieves the state of the sync workers.
	State() []SyncerStateSnapshot
}

type syncManager struct {
	lk        sync.Mutex
	peerHeads map[peer.ID]*types.TipSet

	bssLk          sync.Mutex
	bootstrapState int

	bspThresh int

	incomingTipSets chan *types.TipSet
	syncTargets     chan *types.TipSet
	syncResults     chan *syncResult

	syncStates []*SyncerState

	// Normally this handler is set to `(*Syncer).Sync()`.
	doSync func(context.Context, *types.TipSet) error

	stop chan struct{}

	// Sync Scheduler fields
	activeSyncs    map[types.TipSetKey]*types.TipSet
	syncQueue      syncBucketSet
	activeSyncTips syncBucketSet
	nextSyncTarget *syncTargetBucket
	workerChan     chan *types.TipSet
}

var _ SyncManager = (*syncManager)(nil)

type syncResult struct {
	ts      *types.TipSet
	success bool
}

const syncWorkerCount = 3

func NewSyncManager(sync SyncFunc) SyncManager {
	sm := &syncManager{
		bspThresh:       1,
		peerHeads:       make(map[peer.ID]*types.TipSet),
		syncTargets:     make(chan *types.TipSet),
		syncResults:     make(chan *syncResult),
		syncStates:      make([]*SyncerState, syncWorkerCount),
		incomingTipSets: make(chan *types.TipSet),
		activeSyncs:     make(map[types.TipSetKey]*types.TipSet),
		doSync:          sync,
		stop:            make(chan struct{}),
	}
	for i := range sm.syncStates {
		sm.syncStates[i] = new(SyncerState)
	}
	return sm
}

func (sm *syncManager) Start() {
	go sm.syncScheduler()
	for i := 0; i < syncWorkerCount; i++ {
		go sm.syncWorker(i)
	}
}

func (sm *syncManager) Stop() {
	close(sm.stop)
}

func (sm *syncManager) SetPeerHead(ctx context.Context, p peer.ID, ts *types.TipSet) {
	sm.lk.Lock()
	defer sm.lk.Unlock()
	sm.peerHeads[p] = ts

	if sm.getBootstrapState() == BSStateInit {
		spc := sm.syncedPeerCount()
		if spc >= sm.bspThresh {
			// Its go time!
			target, err := sm.selectSyncTarget()
			if err != nil {
				log.Error("failed to select sync target: ", err)
				return
			}
			sm.setBootstrapState(BSStateSelected)

			sm.incomingTipSets <- target
		}
		log.Infof("sync bootstrap has %d peers", spc)
		return
	}

	sm.incomingTipSets <- ts
}

func (sm *syncManager) State() []SyncerStateSnapshot {
	ret := make([]SyncerStateSnapshot, 0, len(sm.syncStates))
	for _, s := range sm.syncStates {
		ret = append(ret, s.Snapshot())
	}
	return ret
}

type syncBucketSet struct {
	buckets []*syncTargetBucket
}

func newSyncTargetBucket(tipsets ...*types.TipSet) *syncTargetBucket {
	var stb syncTargetBucket
	for _, ts := range tipsets {
		stb.add(ts)
	}
	return &stb
}

func (sbs *syncBucketSet) RelatedToAny(ts *types.TipSet) bool {
	for _, b := range sbs.buckets {
		if b.sameChainAs(ts) {
			return true
		}
	}
	return false
}

func (sbs *syncBucketSet) Insert(ts *types.TipSet) {
	for _, b := range sbs.buckets {
		if b.sameChainAs(ts) {
			b.add(ts)
			return
		}
	}
	sbs.buckets = append(sbs.buckets, newSyncTargetBucket(ts))
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

func (sbs *syncBucketSet) Empty() bool {
	return len(sbs.buckets) == 0
}

type syncTargetBucket struct {
	tips  []*types.TipSet
	count int
}

func (stb *syncTargetBucket) sameChainAs(ts *types.TipSet) bool {
	for _, t := range stb.tips {
		if ts.Equals(t) {
			return true
		}
		if ts.Key() == t.Parents() {
			return true
		}
		if ts.Parents() == t.Key() {
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

func (sm *syncManager) selectSyncTarget() (*types.TipSet, error) {
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
		log.Warn("caution, multiple distinct chains seen during head selections")
		// TODO: we *could* refuse to sync here without user intervention.
		// For now, just select the best cluster
	}

	return buckets.Heaviest(), nil
}

func (sm *syncManager) syncScheduler() {

	for {
		select {
		case ts, ok := <-sm.incomingTipSets:
			if !ok {
				log.Info("shutting down sync scheduler")
				return
			}

			sm.scheduleIncoming(ts)
		case res := <-sm.syncResults:
			sm.scheduleProcessResult(res)
		case sm.workerChan <- sm.nextSyncTarget.heaviestTipSet():
			sm.scheduleWorkSent()
		case <-sm.stop:
			log.Info("sync scheduler shutting down")
			return
		}
	}
}

func (sm *syncManager) scheduleIncoming(ts *types.TipSet) {
	log.Debug("scheduling incoming tipset sync: ", ts.Cids())
	if sm.getBootstrapState() == BSStateSelected {
		sm.setBootstrapState(BSStateScheduled)
		sm.syncTargets <- ts
		return
	}

	var relatedToActiveSync bool
	for _, acts := range sm.activeSyncs {
		if ts.Equals(acts) {
			break
		}

		if ts.Parents() == acts.Key() {
			// sync this next, after that sync process finishes
			relatedToActiveSync = true
		}
	}

	if !relatedToActiveSync && sm.activeSyncTips.RelatedToAny(ts) {
		relatedToActiveSync = true
	}

	// if this is related to an active sync process, immediately bucket it
	// we don't want to start a parallel sync process that duplicates work
	if relatedToActiveSync {
		sm.activeSyncTips.Insert(ts)
		return
	}

	if sm.getBootstrapState() == BSStateScheduled {
		sm.syncQueue.Insert(ts)
		return
	}

	if sm.nextSyncTarget != nil && sm.nextSyncTarget.sameChainAs(ts) {
		sm.nextSyncTarget.add(ts)
	} else {
		sm.syncQueue.Insert(ts)

		if sm.nextSyncTarget == nil {
			sm.nextSyncTarget = sm.syncQueue.Pop()
			sm.workerChan = sm.syncTargets
		}
	}
}

func (sm *syncManager) scheduleProcessResult(res *syncResult) {
	if res.success && sm.getBootstrapState() != BSStateComplete {
		sm.setBootstrapState(BSStateComplete)
	}

	delete(sm.activeSyncs, res.ts.Key())
	relbucket := sm.activeSyncTips.PopRelated(res.ts)
	if relbucket != nil {
		if res.success {
			if sm.nextSyncTarget == nil {
				sm.nextSyncTarget = relbucket
				sm.workerChan = sm.syncTargets
			} else {
				sm.syncQueue.buckets = append(sm.syncQueue.buckets, relbucket)
			}
			return
		}
		// TODO: this is the case where we try to sync a chain, and
		// fail, and we have more blocks on top of that chain that
		// have come in since.  The question is, should we try to
		// sync these? or just drop them?
		log.Error("failed to sync chain but have new unconnected blocks from chain")
	}

	if sm.nextSyncTarget == nil && !sm.syncQueue.Empty() {
		next := sm.syncQueue.Pop()
		if next != nil {
			sm.nextSyncTarget = next
			sm.workerChan = sm.syncTargets
		}
	}
}

func (sm *syncManager) scheduleWorkSent() {
	hts := sm.nextSyncTarget.heaviestTipSet()
	sm.activeSyncs[hts.Key()] = hts

	if !sm.syncQueue.Empty() {
		sm.nextSyncTarget = sm.syncQueue.Pop()
	} else {
		sm.nextSyncTarget = nil
		sm.workerChan = nil
	}
}

func (sm *syncManager) syncWorker(id int) {
	ss := sm.syncStates[id]
	for {
		select {
		case ts, ok := <-sm.syncTargets:
			if !ok {
				log.Info("sync manager worker shutting down")
				return
			}

			ctx := context.WithValue(context.TODO(), syncStateKey{}, ss)
			err := sm.doSync(ctx, ts)
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

func (sm *syncManager) syncedPeerCount() int {
	var count int
	for _, ts := range sm.peerHeads {
		if ts.Height() > 0 {
			count++
		}
	}
	return count
}

func (sm *syncManager) getBootstrapState() int {
	sm.bssLk.Lock()
	defer sm.bssLk.Unlock()
	return sm.bootstrapState
}

func (sm *syncManager) setBootstrapState(v int) {
	sm.bssLk.Lock()
	defer sm.bssLk.Unlock()
	sm.bootstrapState = v
}

func (sm *syncManager) IsBootstrapped() bool {
	sm.bssLk.Lock()
	defer sm.bssLk.Unlock()
	return sm.bootstrapState == BSStateComplete
}
