package splitstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"

	"go.opencensus.io/stats"
)

var (
	// CompactionThreshold is the number of epochs that need to have elapsed
	// from the previously compacted epoch to trigger a new compaction.
	//
	//        |················· CompactionThreshold ··················|
	//        |                                             |
	// =======‖≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡‖------------------------»
	//        |                    |  chain -->             ↑__ current epoch
	//        | archived epochs ___↑
	//                             ↑________ CompactionBoundary
	//
	// === :: cold (already archived)
	// ≡≡≡ :: to be archived in this compaction
	// --- :: hot
	CompactionThreshold = 5 * build.Finality

	// CompactionBoundary is the number of epochs from the current epoch at which
	// we will walk the chain for live objects.
	CompactionBoundary = 4 * build.Finality

	// SyncGapTime is the time delay from a tipset's min timestamp before we decide
	// there is a sync gap
	SyncGapTime = time.Minute
)

var (
	// used to signal end of walk
	errStopWalk = errors.New("stop walk")
)

const (
	batchSize = 16384
)

func (s *SplitStore) HeadChange(_, apply []*types.TipSet) error {
	s.headChangeMx.Lock()
	defer s.headChangeMx.Unlock()

	// Revert only.
	if len(apply) == 0 {
		return nil
	}

	curTs := apply[len(apply)-1]
	epoch := curTs.Height()

	// NOTE: there is an implicit invariant assumption that HeadChange is invoked
	//       synchronously and no other HeadChange can be invoked while one is in
	//       progress.
	//       this is guaranteed by the chainstore, and it is pervasive in all lotus
	//       -- if that ever changes then all hell will break loose in general and
	//       we will have a rance to protectTipSets here.
	//      Reagrdless, we put a mutex in HeadChange just to be safe

	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		// we are currently compacting -- protect the new tipset(s)
		s.protectTipSets(apply)
		return nil
	}

	// check if we are actually closing first
	if atomic.LoadInt32(&s.closing) == 1 {
		atomic.StoreInt32(&s.compacting, 0)
		return nil
	}

	timestamp := time.Unix(int64(curTs.MinTimestamp()), 0)
	if time.Since(timestamp) > SyncGapTime {
		// don't attempt compaction before we have caught up syncing
		atomic.StoreInt32(&s.compacting, 0)
		return nil
	}

	if s.isNearUpgrade(epoch) {
		// we are near an upgrade epoch, suppress compaction
		atomic.StoreInt32(&s.compacting, 0)
		return nil
	}

	if epoch-s.baseEpoch > CompactionThreshold {
		// it's time to compact -- prepare the transaction and go!
		s.beginTxnProtect()
		go func() {
			defer atomic.StoreInt32(&s.compacting, 0)
			defer s.endTxnProtect()

			log.Info("compacting splitstore")
			start := time.Now()

			s.compact(curTs)

			log.Infow("compaction done", "took", time.Since(start))
		}()
	} else {
		// no compaction necessary
		atomic.StoreInt32(&s.compacting, 0)
	}

	return nil
}

func (s *SplitStore) isNearUpgrade(epoch abi.ChainEpoch) bool {
	for _, upgrade := range s.upgrades {
		if epoch >= upgrade.start && epoch <= upgrade.end {
			return true
		}
	}

	return false
}

// transactionally protect incoming tipsets
func (s *SplitStore) protectTipSets(apply []*types.TipSet) {
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	if !s.txnActive {
		return
	}

	var cids []cid.Cid
	for _, ts := range apply {
		cids = append(cids, ts.Cids()...)
	}

	s.trackTxnRefMany(cids)
}

// transactionally protect a view
func (s *SplitStore) protectView(c cid.Cid) {
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	if s.txnActive {
		s.trackTxnRef(c)
	}

	s.txnViewsMx.Lock()
	s.txnViews++
	s.txnViewsMx.Unlock()
}

func (s *SplitStore) viewDone() {
	s.txnViewsMx.Lock()
	defer s.txnViewsMx.Unlock()

	s.txnViews--
	if s.txnViews == 0 && s.txnViewsWaiting {
		s.txnViewsCond.Broadcast()
	}
}

func (s *SplitStore) viewWait() {
	s.txnViewsMx.Lock()
	defer s.txnViewsMx.Unlock()

	s.txnViewsWaiting = true
	for s.txnViews > 0 {
		s.txnViewsCond.Wait()
	}
	s.txnViewsWaiting = false
}

// transactionally protect a reference to an object
func (s *SplitStore) trackTxnRef(c cid.Cid) {
	if !s.txnActive {
		// not compacting
		return
	}

	if isUnitaryObject(c) {
		return
	}

	s.txnRefsMx.Lock()
	s.txnRefs[c] = struct{}{}
	s.txnRefsMx.Unlock()
}

// transactionally protect a batch of references
func (s *SplitStore) trackTxnRefMany(cids []cid.Cid) {
	if !s.txnActive {
		// not compacting
		return
	}

	s.txnRefsMx.Lock()
	defer s.txnRefsMx.Unlock()

	for _, c := range cids {
		if isUnitaryObject(c) {
			continue
		}

		s.txnRefs[c] = struct{}{}
	}

	return
}

// protect all pending transactional references
func (s *SplitStore) protectTxnRefs(markSet MarkSet) error {
	for {
		var txnRefs map[cid.Cid]struct{}

		s.txnRefsMx.Lock()
		if len(s.txnRefs) > 0 {
			txnRefs = s.txnRefs
			s.txnRefs = make(map[cid.Cid]struct{})
		}
		s.txnRefsMx.Unlock()

		if len(txnRefs) == 0 {
			return nil
		}

		log.Infow("protecting transactional references", "refs", len(txnRefs))
		count := 0
		workch := make(chan cid.Cid, len(txnRefs))
		startProtect := time.Now()

		for c := range txnRefs {
			mark, err := markSet.Has(c)
			if err != nil {
				return xerrors.Errorf("error checking markset: %w", err)
			}

			if mark {
				continue
			}

			workch <- c
			count++
		}
		close(workch)

		if count == 0 {
			return nil
		}

		workers := runtime.NumCPU() / 2
		if workers < 2 {
			workers = 2
		}
		if workers > count {
			workers = count
		}

		worker := func() error {
			for c := range workch {
				err := s.doTxnProtect(c, markSet)
				if err != nil {
					return xerrors.Errorf("error protecting transactional references to %s: %w", c, err)
				}
			}
			return nil
		}

		g := new(errgroup.Group)
		for i := 0; i < workers; i++ {
			g.Go(worker)
		}

		if err := g.Wait(); err != nil {
			return err
		}

		log.Infow("protecting transactional refs done", "took", time.Since(startProtect), "protected", count)
	}
}

// transactionally protect a reference by walking the object and marking.
// concurrent markings are short circuited by checking the markset.
func (s *SplitStore) doTxnProtect(root cid.Cid, markSet MarkSet) error {
	if err := s.checkClosing(); err != nil {
		return err
	}

	// Note: cold objects are deleted heaviest first, so the consituents of an object
	// cannot be deleted before the object itself.
	return s.walkObjectIncomplete(root, newTmpVisitor(),
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			visit, err := markSet.Visit(c)
			if err != nil {
				return xerrors.Errorf("error visiting object: %w", err)
			}

			if !visit {
				return errStopWalk
			}

			return nil
		},
		func(c cid.Cid) error {
			if s.txnMissing != nil {
				log.Warnf("missing object reference %s in %s", c, root)
				s.txnRefsMx.Lock()
				s.txnMissing[c] = struct{}{}
				s.txnRefsMx.Unlock()
			}
			return errStopWalk
		})
}

func (s *SplitStore) applyProtectors() error {
	s.mx.Lock()
	defer s.mx.Unlock()

	count := 0
	for _, protect := range s.protectors {
		err := protect(func(c cid.Cid) error {
			s.trackTxnRef(c)
			count++
			return nil
		})

		if err != nil {
			return xerrors.Errorf("error applynig protector: %w", err)
		}
	}

	if count > 0 {
		log.Infof("protected %d references through %d protectors", count, len(s.protectors))
	}

	return nil
}

// --- Compaction ---
// Compaction works transactionally with the following algorithm:
// - We prepare a transaction, whereby all i/o referenced objects through the API are tracked.
// - We walk the chain and mark reachable objects, keeping 4 finalities of state roots and messages and all headers all the way to genesis.
// - Once the chain walk is complete, we begin full transaction protection with concurrent marking; we walk and mark all references created during the chain walk. On the same time, all I/O through the API concurrently marks objects as live references.
// - We collect cold objects by iterating through the hotstore and checking the mark set; if an object is not marked, then it is candidate for purge.
// - When running with a coldstore, we next copy all cold objects to the coldstore.
// - At this point we are ready to begin purging:
//   - We sort cold objects heaviest first, so as to never delete the consituents of a DAG before the DAG itself (which would leave dangling references)
//   - We delete in small batches taking a lock; each batch is checked again for marks, from the concurrent transactional mark, so as to never delete anything live
// - We then end the transaction and compact/gc the hotstore.
func (s *SplitStore) compact(curTs *types.TipSet) {
	log.Info("waiting for active views to complete")
	start := time.Now()
	s.viewWait()
	log.Infow("waiting for active views done", "took", time.Since(start))

	start = time.Now()
	err := s.doCompact(curTs)
	took := time.Since(start).Milliseconds()
	stats.Record(s.ctx, metrics.SplitstoreCompactionTimeSeconds.M(float64(took)/1e3))

	if err != nil {
		log.Errorf("COMPACTION ERROR: %s", err)
	}
}

func (s *SplitStore) doCompact(curTs *types.TipSet) error {
	if s.checkpointExists() {
		// this really shouldn't happen, but if it somehow does, it means that the hotstore
		// might be potentially inconsistent; abort compaction and notify the user to intervene.
		return xerrors.Errorf("checkpoint exists; aborting compaction")
	}

	currentEpoch := curTs.Height()
	boundaryEpoch := currentEpoch - CompactionBoundary

	var inclMsgsEpoch abi.ChainEpoch
	inclMsgsRange := abi.ChainEpoch(s.cfg.HotStoreMessageRetention) * build.Finality
	if inclMsgsRange < boundaryEpoch {
		inclMsgsEpoch = boundaryEpoch - inclMsgsRange
	}

	log.Infow("running compaction", "currentEpoch", currentEpoch, "baseEpoch", s.baseEpoch, "boundaryEpoch", boundaryEpoch, "inclMsgsEpoch", inclMsgsEpoch, "compactionIndex", s.compactionIndex)

	markSet, err := s.markSetEnv.New("live", s.markSetSize)
	if err != nil {
		return xerrors.Errorf("error creating mark set: %w", err)
	}
	defer markSet.Close() //nolint:errcheck
	defer s.debug.Flush()

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 0. track all protected references at beginning of compaction; anything added later should
	//    be transactionally protected by the write
	log.Info("protecting references with registered protectors")
	err = s.applyProtectors()
	if err != nil {
		return err
	}

	// 1. mark reachable objects by walking the chain from the current epoch; we keep state roots
	//   and messages until the boundary epoch.
	log.Info("marking reachable objects")
	startMark := time.Now()

	count := new(int64)
	err = s.walkChain(curTs, boundaryEpoch, inclMsgsEpoch, &noopVisitor{},
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			visit, err := markSet.Visit(c)
			if err != nil {
				return xerrors.Errorf("error visiting object: %w", err)
			}

			if !visit {
				return errStopWalk
			}

			atomic.AddInt64(count, 1)
			return nil
		})

	if err != nil {
		return xerrors.Errorf("error marking: %w", err)
	}

	s.markSetSize = *count + *count>>2 // overestimate a bit

	log.Infow("marking done", "took", time.Since(startMark), "marked", *count)

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 1.1 protect transactional refs
	err = s.protectTxnRefs(markSet)
	if err != nil {
		return xerrors.Errorf("error protecting transactional refs: %w", err)
	}

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 2. iterate through the hotstore to collect cold objects
	log.Info("collecting cold objects")
	startCollect := time.Now()

	coldw, err := NewColdSetWriter(s.coldSetPath())
	if err != nil {
		return xerrors.Errorf("error creating coldset: %w", err)
	}
	defer coldw.Close() //nolint:errcheck

	// some stats for logging
	var hotCnt, coldCnt int

	err = s.hot.ForEachKey(func(c cid.Cid) error {
		// was it marked?
		mark, err := markSet.Has(c)
		if err != nil {
			return xerrors.Errorf("error checking mark set for %s: %w", c, err)
		}

		if mark {
			hotCnt++
			return nil
		}

		// it's cold, mark it as candidate for move
		if err := coldw.Write(c); err != nil {
			return xerrors.Errorf("error writing cid to coldstore: %w", err)
		}
		coldCnt++

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error collecting cold objects: %w", err)
	}

	if err := coldw.Close(); err != nil {
		return xerrors.Errorf("error closing coldset: %w", err)
	}

	log.Infow("cold collection done", "took", time.Since(startCollect))

	log.Infow("compaction stats", "hot", hotCnt, "cold", coldCnt)
	stats.Record(s.ctx, metrics.SplitstoreCompactionHot.M(int64(hotCnt)))
	stats.Record(s.ctx, metrics.SplitstoreCompactionCold.M(int64(coldCnt)))

	if err := s.checkClosing(); err != nil {
		return err
	}

	// now that we have collected cold objects, check for missing references from transactional i/o
	// and disable further collection of such references (they will not be acted upon as we can't
	// possibly delete objects we didn't have when we were collecting cold objects)
	s.waitForMissingRefs(markSet)

	if err := s.checkClosing(); err != nil {
		return err
	}

	coldr, err := NewColdSetReader(s.coldSetPath())
	if err != nil {
		return xerrors.Errorf("error opening coldset: %w", err)
	}
	defer coldr.Close() //nolint:errcheck

	// 3. copy the cold objects to the coldstore -- if we have one
	if !s.cfg.DiscardColdBlocks {
		log.Info("moving cold objects to the coldstore")
		startMove := time.Now()
		err = s.moveColdBlocks(coldr)
		if err != nil {
			return xerrors.Errorf("error moving cold objects: %w", err)
		}
		log.Infow("moving done", "took", time.Since(startMove))

		if err := s.checkClosing(); err != nil {
			return err
		}

		if err := coldr.Reset(); err != nil {
			return xerrors.Errorf("error resetting coldset: %w", err)
		}
	}

	// 4. Purge cold objects with checkpointing for recovery.
	// This is the critical section of compaction, whereby any cold object not in the markSet is
	// considered already deleted.
	// We delete cold objects in batches, holding the transaction lock, where we check the markSet
	// again for new references created by the VM.
	// After each batch, we write a checkpoint to disk; if the process is interrupted before completion,
	// the process will continue from the checkpoint in the next recovery.
	if err := s.beginCriticalSection(markSet); err != nil {
		return xerrors.Errorf("error beginning critical section: %w", err)
	}

	if err := s.checkClosing(); err != nil {
		return err
	}

	checkpoint, err := NewCheckpoint(s.checkpointPath())
	if err != nil {
		return xerrors.Errorf("error creating checkpoint: %w", err)
	}
	defer checkpoint.Close() //nolint:errcheck

	// 5. purge cold objects from the hotstore, taking protected references into account
	log.Info("purging cold objects from the hotstore")
	startPurge := time.Now()
	err = s.purge(coldr, checkpoint, markSet)
	if err != nil {
		return xerrors.Errorf("error purging cold objects: %w", err)
	}
	log.Infow("purging cold objects from hotstore done", "took", time.Since(startPurge))

	s.endCriticalSection()

	if err := checkpoint.Close(); err != nil {
		log.Warnf("error closing checkpoint: %s", err)
	}
	if err := os.Remove(s.checkpointPath()); err != nil {
		log.Warnf("error removing checkpoint: %s", err)
	}
	if err := coldr.Close(); err != nil {
		log.Warnf("error closing coldset: %s", err)
	}
	if err := os.Remove(s.coldSetPath()); err != nil {
		log.Warnf("error removing coldset: %s", err)
	}

	// we are done; do some housekeeping
	s.endTxnProtect()
	s.gcHotstore()

	err = s.setBaseEpoch(boundaryEpoch)
	if err != nil {
		return xerrors.Errorf("error saving base epoch: %w", err)
	}

	err = s.ds.Put(s.ctx, markSetSizeKey, int64ToBytes(s.markSetSize))
	if err != nil {
		return xerrors.Errorf("error saving mark set size: %w", err)
	}

	s.compactionIndex++
	err = s.ds.Put(s.ctx, compactionIndexKey, int64ToBytes(s.compactionIndex))
	if err != nil {
		return xerrors.Errorf("error saving compaction index: %w", err)
	}

	return nil
}

func (s *SplitStore) beginTxnProtect() {
	log.Info("preparing compaction transaction")

	s.txnLk.Lock()
	defer s.txnLk.Unlock()

	s.txnActive = true
	s.txnRefs = make(map[cid.Cid]struct{})
	s.txnMissing = make(map[cid.Cid]struct{})
}

func (s *SplitStore) beginCriticalSection(markSet MarkSet) error {
	log.Info("beginning critical section")

	if err := markSet.BeginCriticalSection(); err != nil {
		return xerrors.Errorf("error beginning critical section for markset: %w", err)
	}

	s.txnLk.Lock()
	s.txnMarkSet = markSet
	s.txnLk.Unlock()

	if err := s.protectTxnRefs(markSet); err != nil {
		return xerrors.Errorf("error protecting transactional references: %w", err)
	}

	return nil
}

func (s *SplitStore) endTxnProtect() {
	s.txnLk.Lock()
	defer s.txnLk.Unlock()

	if !s.txnActive {
		return
	}

	s.txnActive = false
	s.txnRefs = nil
	s.txnMissing = nil
	s.txnMarkSet = nil
}

func (s *SplitStore) endCriticalSection() {
	log.Info("ending critical section")

	s.txnLk.Lock()
	defer s.txnLk.Unlock()

	s.txnMarkSet.EndCriticalSection()
	s.txnMarkSet = nil
}

func (s *SplitStore) walkChain(ts *types.TipSet, inclState, inclMsgs abi.ChainEpoch,
	visitor ObjectVisitor, f func(cid.Cid) error) error {
	var walked ObjectVisitor
	var mx sync.Mutex
	// we copy the tipset first into a new slice, which allows us to reuse it in every epoch.
	toWalk := make([]cid.Cid, len(ts.Cids()))
	copy(toWalk, ts.Cids())
	walkCnt := new(int64)
	scanCnt := new(int64)

	stopWalk := func(_ cid.Cid) error { return errStopWalk }

	walkBlock := func(c cid.Cid) error {
		visit, err := walked.Visit(c)
		if err != nil {
			return err
		}
		if !visit {
			return nil
		}

		atomic.AddInt64(walkCnt, 1)

		if err := f(c); err != nil {
			return err
		}

		var hdr types.BlockHeader
		err = s.view(c, func(data []byte) error {
			return hdr.UnmarshalCBOR(bytes.NewBuffer(data))
		})

		if err != nil {
			return xerrors.Errorf("error unmarshaling block header (cid: %s): %w", c, err)
		}

		// message are retained if within the inclMsgs boundary
		if hdr.Height >= inclMsgs && hdr.Height > 0 {
			if inclMsgs < inclState {
				// we need to use walkObjectIncomplete here, as messages/receipts may be missing early on if we
				// synced from snapshot and have a long HotStoreMessageRetentionPolicy.
				if err := s.walkObjectIncomplete(hdr.Messages, visitor, f, stopWalk); err != nil {
					return xerrors.Errorf("error walking messages (cid: %s): %w", hdr.Messages, err)
				}

				if err := s.walkObjectIncomplete(hdr.ParentMessageReceipts, visitor, f, stopWalk); err != nil {
					return xerrors.Errorf("error walking messages receipts (cid: %s): %w", hdr.ParentMessageReceipts, err)
				}
			} else {
				if err := s.walkObject(hdr.Messages, visitor, f); err != nil {
					return xerrors.Errorf("error walking messages (cid: %s): %w", hdr.Messages, err)
				}

				if err := s.walkObject(hdr.ParentMessageReceipts, visitor, f); err != nil {
					return xerrors.Errorf("error walking message receipts (cid: %s): %w", hdr.ParentMessageReceipts, err)
				}
			}
		}

		// state is only retained if within the inclState boundary, with the exception of genesis
		if hdr.Height >= inclState || hdr.Height == 0 {
			if err := s.walkObject(hdr.ParentStateRoot, visitor, f); err != nil {
				return xerrors.Errorf("error walking state root (cid: %s): %w", hdr.ParentStateRoot, err)
			}
			atomic.AddInt64(scanCnt, 1)
		}

		if hdr.Height > 0 {
			mx.Lock()
			toWalk = append(toWalk, hdr.Parents...)
			mx.Unlock()
		}

		return nil
	}

	for len(toWalk) > 0 {
		// walking can take a while, so check this with every opportunity
		if err := s.checkClosing(); err != nil {
			return err
		}

		workers := len(toWalk)
		if workers > runtime.NumCPU()/2 {
			workers = runtime.NumCPU() / 2
		}
		if workers < 2 {
			workers = 2
		}

		// the walk is BFS, so we can reset the walked set in every iteration and avoid building up
		// a set that contains all blocks (1M epochs -> 5M blocks -> 200MB worth of memory and growing
		// over time)
		walked = newConcurrentVisitor()
		workch := make(chan cid.Cid, len(toWalk))
		for _, c := range toWalk {
			workch <- c
		}
		close(workch)
		toWalk = toWalk[:0]

		g := new(errgroup.Group)
		for i := 0; i < workers; i++ {
			g.Go(func() error {
				for c := range workch {
					if err := walkBlock(c); err != nil {
						return xerrors.Errorf("error walking block (cid: %s): %w", c, err)
					}
				}
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}
	}

	log.Infow("chain walk done", "walked", *walkCnt, "scanned", *scanCnt)

	return nil
}

func (s *SplitStore) walkObject(c cid.Cid, visitor ObjectVisitor, f func(cid.Cid) error) error {
	visit, err := visitor.Visit(c)
	if err != nil {
		return xerrors.Errorf("error visiting object: %w", err)
	}

	if !visit {
		return nil
	}

	if err := f(c); err != nil {
		if err == errStopWalk {
			return nil
		}

		return err
	}

	if c.Prefix().Codec != cid.DagCBOR {
		return nil
	}

	// check this before recursing
	if err := s.checkClosing(); err != nil {
		return err
	}

	var links []cid.Cid
	err = s.view(c, func(data []byte) error {
		return cbg.ScanForLinks(bytes.NewReader(data), func(c cid.Cid) {
			links = append(links, c)
		})
	})

	if err != nil {
		return xerrors.Errorf("error scanning linked block (cid: %s): %w", c, err)
	}

	for _, c := range links {
		err := s.walkObject(c, visitor, f)
		if err != nil {
			return xerrors.Errorf("error walking link (cid: %s): %w", c, err)
		}
	}

	return nil
}

// like walkObject, but the object may be potentially incomplete (references missing)
func (s *SplitStore) walkObjectIncomplete(c cid.Cid, visitor ObjectVisitor, f, missing func(cid.Cid) error) error {
	visit, err := visitor.Visit(c)
	if err != nil {
		return xerrors.Errorf("error visiting object: %w", err)
	}

	if !visit {
		return nil
	}

	// occurs check -- only for DAGs
	if c.Prefix().Codec == cid.DagCBOR {
		has, err := s.has(c)
		if err != nil {
			return xerrors.Errorf("error occur checking %s: %w", c, err)
		}

		if !has {
			err = missing(c)
			if err == errStopWalk {
				return nil
			}

			return err
		}
	}

	if err := f(c); err != nil {
		if err == errStopWalk {
			return nil
		}

		return err
	}

	if c.Prefix().Codec != cid.DagCBOR {
		return nil
	}

	// check this before recursing
	if err := s.checkClosing(); err != nil {
		return err
	}

	var links []cid.Cid
	err = s.view(c, func(data []byte) error {
		return cbg.ScanForLinks(bytes.NewReader(data), func(c cid.Cid) {
			links = append(links, c)
		})
	})

	if err != nil {
		return xerrors.Errorf("error scanning linked block (cid: %s): %w", c, err)
	}

	for _, c := range links {
		err := s.walkObjectIncomplete(c, visitor, f, missing)
		if err != nil {
			return xerrors.Errorf("error walking link (cid: %s): %w", c, err)
		}
	}

	return nil
}

// internal version used by walk
func (s *SplitStore) view(c cid.Cid, cb func([]byte) error) error {
	if isIdentiyCid(c) {
		data, err := decodeIdentityCid(c)
		if err != nil {
			return err
		}

		return cb(data)
	}

	err := s.hot.View(s.ctx, c, cb)
	switch err {
	case bstore.ErrNotFound:
		return s.cold.View(s.ctx, c, cb)

	default:
		return err
	}
}

func (s *SplitStore) has(c cid.Cid) (bool, error) {
	if isIdentiyCid(c) {
		return true, nil
	}

	has, err := s.hot.Has(s.ctx, c)

	if has || err != nil {
		return has, err
	}

	return s.cold.Has(s.ctx, c)
}

func (s *SplitStore) get(c cid.Cid) (blocks.Block, error) {
	blk, err := s.hot.Get(s.ctx, c)
	switch err {
	case nil:
		return blk, nil
	case bstore.ErrNotFound:
		return s.cold.Get(s.ctx, c)
	default:
		return nil, err
	}
}

func (s *SplitStore) getSize(c cid.Cid) (int, error) {
	sz, err := s.hot.GetSize(s.ctx, c)
	switch err {
	case nil:
		return sz, nil
	case bstore.ErrNotFound:
		return s.cold.GetSize(s.ctx, c)
	default:
		return 0, err
	}
}

func (s *SplitStore) moveColdBlocks(coldr *ColdSetReader) error {
	batch := make([]blocks.Block, 0, batchSize)

	err := coldr.ForEach(func(c cid.Cid) error {
		if err := s.checkClosing(); err != nil {
			return err
		}

		blk, err := s.hot.Get(s.ctx, c)
		if err != nil {
			if err == bstore.ErrNotFound {
				log.Warnf("hotstore missing block %s", c)
				return nil
			}

			return xerrors.Errorf("error retrieving block %s from hotstore: %w", c, err)
		}

		batch = append(batch, blk)
		if len(batch) == batchSize {
			err = s.cold.PutMany(s.ctx, batch)
			if err != nil {
				return xerrors.Errorf("error putting batch to coldstore: %w", err)
			}
			batch = batch[:0]
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error iterating coldset: %w", err)
	}

	if len(batch) > 0 {
		err := s.cold.PutMany(s.ctx, batch)
		if err != nil {
			return xerrors.Errorf("error putting batch to coldstore: %w", err)
		}
	}

	return nil
}

func (s *SplitStore) purge(coldr *ColdSetReader, checkpoint *Checkpoint, markSet MarkSet) error {
	batch := make([]cid.Cid, 0, batchSize)
	deadCids := make([]cid.Cid, 0, batchSize)

	var purgeCnt, liveCnt int
	defer func() {
		log.Infow("purged cold objects", "purged", purgeCnt, "live", liveCnt)
	}()

	deleteBatch := func() error {
		pc, lc, err := s.purgeBatch(batch, deadCids, checkpoint, markSet)

		purgeCnt += pc
		liveCnt += lc
		batch = batch[:0]

		return err
	}

	err := coldr.ForEach(func(c cid.Cid) error {
		batch = append(batch, c)
		if len(batch) == batchSize {
			return deleteBatch()
		}

		return nil
	})

	if err != nil {
		return err
	}

	if len(batch) > 0 {
		return deleteBatch()
	}

	return nil
}

func (s *SplitStore) purgeBatch(batch, deadCids []cid.Cid, checkpoint *Checkpoint, markSet MarkSet) (purgeCnt int, liveCnt int, err error) {
	if err := s.checkClosing(); err != nil {
		return 0, 0, err
	}

	s.txnLk.Lock()
	defer s.txnLk.Unlock()

	for _, c := range batch {
		has, err := markSet.Has(c)
		if err != nil {
			return 0, 0, xerrors.Errorf("error checking markset for liveness: %w", err)
		}

		if has {
			liveCnt++
			continue
		}

		deadCids = append(deadCids, c)
	}

	if len(deadCids) == 0 {
		if err := checkpoint.Set(batch[len(batch)-1]); err != nil {
			return 0, 0, xerrors.Errorf("error setting checkpoint: %w", err)
		}

		return 0, liveCnt, nil
	}

	if err := s.hot.DeleteMany(s.ctx, deadCids); err != nil {
		return 0, liveCnt, xerrors.Errorf("error purging cold objects: %w", err)
	}

	s.debug.LogDelete(deadCids)
	purgeCnt = len(deadCids)

	if err := checkpoint.Set(batch[len(batch)-1]); err != nil {
		return purgeCnt, liveCnt, xerrors.Errorf("error setting checkpoint: %w", err)
	}

	return purgeCnt, liveCnt, nil
}

func (s *SplitStore) coldSetPath() string {
	return filepath.Join(s.path, "coldset")
}

func (s *SplitStore) checkpointPath() string {
	return filepath.Join(s.path, "checkpoint")
}

func (s *SplitStore) checkpointExists() bool {
	_, err := os.Stat(s.checkpointPath())
	return err == nil
}

func (s *SplitStore) completeCompaction() error {
	checkpoint, last, err := OpenCheckpoint(s.checkpointPath())
	if err != nil {
		return xerrors.Errorf("error opening checkpoint: %w", err)
	}
	defer checkpoint.Close() //nolint:errcheck

	coldr, err := NewColdSetReader(s.coldSetPath())
	if err != nil {
		return xerrors.Errorf("error opening coldset: %w", err)
	}
	defer coldr.Close() //nolint:errcheck

	markSet, err := s.markSetEnv.Recover("live")
	if err != nil {
		return xerrors.Errorf("error recovering markset: %w", err)
	}
	defer markSet.Close() //nolint:errcheck

	// PURGE
	log.Info("purging cold objects from the hotstore")
	startPurge := time.Now()
	err = s.completePurge(coldr, checkpoint, last, markSet)
	if err != nil {
		return xerrors.Errorf("error purging cold objects: %w", err)
	}
	log.Infow("purging cold objects from hotstore done", "took", time.Since(startPurge))

	markSet.EndCriticalSection()

	if err := checkpoint.Close(); err != nil {
		log.Warnf("error closing checkpoint: %s", err)
	}
	if err := os.Remove(s.checkpointPath()); err != nil {
		log.Warnf("error removing checkpoint: %s", err)
	}
	if err := coldr.Close(); err != nil {
		log.Warnf("error closing coldset: %s", err)
	}
	if err := os.Remove(s.coldSetPath()); err != nil {
		log.Warnf("error removing coldset: %s", err)
	}

	// Note: at this point we can start the splitstore; a compaction should run on
	//       the first head change, which will trigger gc on the hotstore.
	//       We don't mind the second (back-to-back) compaction as the head will
	//       have advanced during marking and coldset accumulation.
	return nil
}

func (s *SplitStore) completePurge(coldr *ColdSetReader, checkpoint *Checkpoint, start cid.Cid, markSet MarkSet) error {
	if !start.Defined() {
		return s.purge(coldr, checkpoint, markSet)
	}

	seeking := true
	batch := make([]cid.Cid, 0, batchSize)
	deadCids := make([]cid.Cid, 0, batchSize)

	var purgeCnt, liveCnt int
	defer func() {
		log.Infow("purged cold objects", "purged", purgeCnt, "live", liveCnt)
	}()

	deleteBatch := func() error {
		pc, lc, err := s.purgeBatch(batch, deadCids, checkpoint, markSet)

		purgeCnt += pc
		liveCnt += lc
		batch = batch[:0]

		return err
	}

	err := coldr.ForEach(func(c cid.Cid) error {
		if seeking {
			if start.Equals(c) {
				seeking = false
			}

			return nil
		}

		batch = append(batch, c)
		if len(batch) == batchSize {
			return deleteBatch()
		}

		return nil
	})

	if err != nil {
		return err
	}

	if len(batch) > 0 {
		return deleteBatch()
	}

	return nil
}

// I really don't like having this code, but we seem to have some occasional DAG references with
// missing constituents. During testing in mainnet *some* of these references *sometimes* appeared
// after a little bit.
// We need to figure out where they are coming from and eliminate that vector, but until then we
// have this gem[TM].
// My best guess is that they are parent message receipts or yet to be computed state roots; magik
// thinks the cause may be block validation.
func (s *SplitStore) waitForMissingRefs(markSet MarkSet) {
	s.txnLk.Lock()
	missing := s.txnMissing
	s.txnMissing = nil
	s.txnLk.Unlock()

	if len(missing) == 0 {
		return
	}

	log.Info("waiting for missing references")
	start := time.Now()
	count := 0
	defer func() {
		log.Infow("waiting for missing references done", "took", time.Since(start), "marked", count)
	}()

	for i := 0; i < 3 && len(missing) > 0; i++ {
		if err := s.checkClosing(); err != nil {
			return
		}

		wait := time.Duration(i) * time.Minute
		log.Infof("retrying for %d missing references in %s (attempt: %d)", len(missing), wait, i+1)
		if wait > 0 {
			time.Sleep(wait)
		}

		towalk := missing
		visitor := newTmpVisitor()
		missing = make(map[cid.Cid]struct{})

		for c := range towalk {
			err := s.walkObjectIncomplete(c, visitor,
				func(c cid.Cid) error {
					if isUnitaryObject(c) {
						return errStopWalk
					}

					visit, err := markSet.Visit(c)
					if err != nil {
						return xerrors.Errorf("error visiting object: %w", err)
					}

					if !visit {
						return errStopWalk
					}

					count++
					return nil
				},
				func(c cid.Cid) error {
					missing[c] = struct{}{}
					return errStopWalk
				})

			if err != nil {
				log.Warnf("error marking: %s", err)
			}
		}
	}

	if len(missing) > 0 {
		log.Warnf("still missing %d references", len(missing))
		for c := range missing {
			log.Warnf("unresolved missing reference: %s", c)
		}
	}
}
