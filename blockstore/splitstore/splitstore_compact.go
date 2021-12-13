package splitstore

import (
	"bytes"
	"errors"
	"runtime"
	"sort"
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

	defaultColdPurgeSize = 7_000_000
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
func (s *SplitStore) protectTxnRefs(markSet MarkSetVisitor) error {
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
func (s *SplitStore) doTxnProtect(root cid.Cid, markSet MarkSetVisitor) error {
	if err := s.checkClosing(); err != nil {
		return err
	}

	// Note: cold objects are deleted heaviest first, so the consituents of an object
	// cannot be deleted before the object itself.
	return s.walkObjectIncomplete(root, tmpVisitor(),
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
	currentEpoch := curTs.Height()
	boundaryEpoch := currentEpoch - CompactionBoundary

	var inclMsgsEpoch abi.ChainEpoch
	inclMsgsRange := abi.ChainEpoch(s.cfg.HotStoreMessageRetention) * build.Finality
	if inclMsgsRange < boundaryEpoch {
		inclMsgsEpoch = boundaryEpoch - inclMsgsRange
	}

	log.Infow("running compaction", "currentEpoch", currentEpoch, "baseEpoch", s.baseEpoch, "boundaryEpoch", boundaryEpoch, "inclMsgsEpoch", inclMsgsEpoch, "compactionIndex", s.compactionIndex)

	markSet, err := s.markSetEnv.CreateVisitor("live", s.markSetSize)
	if err != nil {
		return xerrors.Errorf("error creating mark set: %w", err)
	}
	defer markSet.Close() //nolint:errcheck
	defer s.debug.Flush()

	if err := s.checkClosing(); err != nil {
		return err
	}

	// we are ready for concurrent marking
	s.beginTxnMarking(markSet)

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

	var count int64
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

			count++
			return nil
		})

	if err != nil {
		return xerrors.Errorf("error marking: %w", err)
	}

	s.markSetSize = count + count>>2 // overestimate a bit

	log.Infow("marking done", "took", time.Since(startMark), "marked", count)

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

	// some stats for logging
	var hotCnt, coldCnt int

	cold := make([]cid.Cid, 0, s.coldPurgeSize)
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
		cold = append(cold, c)
		coldCnt++

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error collecting cold objects: %w", err)
	}

	log.Infow("cold collection done", "took", time.Since(startCollect))

	if coldCnt > 0 {
		s.coldPurgeSize = coldCnt + coldCnt>>2 // overestimate a bit
	}

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

	// 3. copy the cold objects to the coldstore -- if we have one
	if !s.cfg.DiscardColdBlocks {
		log.Info("moving cold objects to the coldstore")
		startMove := time.Now()
		err = s.moveColdBlocks(cold)
		if err != nil {
			return xerrors.Errorf("error moving cold objects: %w", err)
		}
		log.Infow("moving done", "took", time.Since(startMove))

		if err := s.checkClosing(); err != nil {
			return err
		}
	}

	// 4. sort cold objects so that the dags with most references are deleted first
	//    this ensures that we can't refer to a dag with its consituents already deleted, ie
	//    we lave no dangling references.
	log.Info("sorting cold objects")
	startSort := time.Now()
	err = s.sortObjects(cold)
	if err != nil {
		return xerrors.Errorf("error sorting objects: %w", err)
	}
	log.Infow("sorting done", "took", time.Since(startSort))

	// 4.1 protect transactional refs once more
	//     strictly speaking, this is not necessary as purge will do it before deleting each
	//     batch.  however, there is likely a largish number of references accumulated during
	//     ths sort and this protects before entering pruge context.
	err = s.protectTxnRefs(markSet)
	if err != nil {
		return xerrors.Errorf("error protecting transactional refs: %w", err)
	}

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 5. purge cold objects from the hotstore, taking protected references into account
	log.Info("purging cold objects from the hotstore")
	startPurge := time.Now()
	err = s.purge(cold, markSet)
	if err != nil {
		return xerrors.Errorf("error purging cold blocks: %w", err)
	}
	log.Infow("purging cold objects from hotstore done", "took", time.Since(startPurge))

	// we are done; do some housekeeping
	s.endTxnProtect()
	s.gcHotstore()

	err = s.setBaseEpoch(boundaryEpoch)
	if err != nil {
		return xerrors.Errorf("error saving base epoch: %w", err)
	}

	err = s.ds.Put(markSetSizeKey, int64ToBytes(s.markSetSize))
	if err != nil {
		return xerrors.Errorf("error saving mark set size: %w", err)
	}

	s.compactionIndex++
	err = s.ds.Put(compactionIndexKey, int64ToBytes(s.compactionIndex))
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

func (s *SplitStore) beginTxnMarking(markSet MarkSetVisitor) {
	markSet.SetConcurrent()
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
}

func (s *SplitStore) walkChain(ts *types.TipSet, inclState, inclMsgs abi.ChainEpoch,
	visitor ObjectVisitor, f func(cid.Cid) error) error {
	var walked *cid.Set
	toWalk := ts.Cids()
	walkCnt := 0
	scanCnt := 0

	stopWalk := func(_ cid.Cid) error { return errStopWalk }

	walkBlock := func(c cid.Cid) error {
		if !walked.Visit(c) {
			return nil
		}

		walkCnt++

		if err := f(c); err != nil {
			return err
		}

		var hdr types.BlockHeader
		err := s.view(c, func(data []byte) error {
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
			scanCnt++
		}

		if hdr.Height > 0 {
			toWalk = append(toWalk, hdr.Parents...)
		}

		return nil
	}

	for len(toWalk) > 0 {
		// walking can take a while, so check this with every opportunity
		if err := s.checkClosing(); err != nil {
			return err
		}

		// the walk is BFS, so we can reset the walked set in every iteration and avoid building up
		// a set that contains all blocks (1M epochs -> 5M blocks -> 200MB worth of memory and growing
		// over time)
		walked = cid.NewSet()
		walking := toWalk
		toWalk = nil
		for _, c := range walking {
			if err := walkBlock(c); err != nil {
				return xerrors.Errorf("error walking block (cid: %s): %w", c, err)
			}
		}
	}

	log.Infow("chain walk done", "walked", walkCnt, "scanned", scanCnt)

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

	err := s.hot.View(c, cb)
	switch err {
	case bstore.ErrNotFound:
		return s.cold.View(c, cb)

	default:
		return err
	}
}

func (s *SplitStore) has(c cid.Cid) (bool, error) {
	if isIdentiyCid(c) {
		return true, nil
	}

	has, err := s.hot.Has(c)

	if has || err != nil {
		return has, err
	}

	return s.cold.Has(c)
}

func (s *SplitStore) moveColdBlocks(cold []cid.Cid) error {
	batch := make([]blocks.Block, 0, batchSize)

	for _, c := range cold {
		if err := s.checkClosing(); err != nil {
			return err
		}

		blk, err := s.hot.Get(c)
		if err != nil {
			if err == bstore.ErrNotFound {
				log.Warnf("hotstore missing block %s", c)
				continue
			}

			return xerrors.Errorf("error retrieving block %s from hotstore: %w", c, err)
		}

		batch = append(batch, blk)
		if len(batch) == batchSize {
			err = s.cold.PutMany(batch)
			if err != nil {
				return xerrors.Errorf("error putting batch to coldstore: %w", err)
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		err := s.cold.PutMany(batch)
		if err != nil {
			return xerrors.Errorf("error putting batch to coldstore: %w", err)
		}
	}

	return nil
}

// sorts a slice of objects heaviest first -- it's a little expensive but worth the
// guarantee that we don't leave dangling references behind, e.g. if we die in the middle
// of a purge.
func (s *SplitStore) sortObjects(cids []cid.Cid) error {
	// we cache the keys to avoid making a gazillion of strings
	keys := make(map[cid.Cid]string)
	key := func(c cid.Cid) string {
		s, ok := keys[c]
		if !ok {
			s = string(c.Hash())
			keys[c] = s
		}
		return s
	}

	// compute sorting weights as the cumulative number of DAG links
	weights := make(map[string]int)
	for _, c := range cids {
		// this can take quite a while, so check for shutdown with every opportunity
		if err := s.checkClosing(); err != nil {
			return err
		}

		w := s.getObjectWeight(c, weights, key)
		weights[key(c)] = w
	}

	// sort!
	sort.Slice(cids, func(i, j int) bool {
		wi := weights[key(cids[i])]
		wj := weights[key(cids[j])]
		if wi == wj {
			return bytes.Compare(cids[i].Hash(), cids[j].Hash()) > 0
		}

		return wi > wj
	})

	return nil
}

func (s *SplitStore) getObjectWeight(c cid.Cid, weights map[string]int, key func(cid.Cid) string) int {
	w, ok := weights[key(c)]
	if ok {
		return w
	}

	// we treat block headers specially to avoid walking the entire chain
	var hdr types.BlockHeader
	err := s.view(c, func(data []byte) error {
		return hdr.UnmarshalCBOR(bytes.NewBuffer(data))
	})
	if err == nil {
		w1 := s.getObjectWeight(hdr.ParentStateRoot, weights, key)
		weights[key(hdr.ParentStateRoot)] = w1

		w2 := s.getObjectWeight(hdr.Messages, weights, key)
		weights[key(hdr.Messages)] = w2

		return 1 + w1 + w2
	}

	var links []cid.Cid
	err = s.view(c, func(data []byte) error {
		return cbg.ScanForLinks(bytes.NewReader(data), func(c cid.Cid) {
			links = append(links, c)
		})
	})
	if err != nil {
		return 1
	}

	w = 1
	for _, c := range links {
		// these are internal refs, so dags will be dags
		if c.Prefix().Codec != cid.DagCBOR {
			w++
			continue
		}

		wc := s.getObjectWeight(c, weights, key)
		weights[key(c)] = wc

		w += wc
	}

	return w
}

func (s *SplitStore) purgeBatch(cids []cid.Cid, deleteBatch func([]cid.Cid) error) error {
	if len(cids) == 0 {
		return nil
	}

	// we don't delete one giant batch of millions of objects, but rather do smaller batches
	// so that we don't stop the world for an extended period of time
	done := false
	for i := 0; !done; i++ {
		start := i * batchSize
		end := start + batchSize
		if end >= len(cids) {
			end = len(cids)
			done = true
		}

		err := deleteBatch(cids[start:end])
		if err != nil {
			return xerrors.Errorf("error deleting batch: %w", err)
		}
	}

	return nil
}

func (s *SplitStore) purge(cids []cid.Cid, markSet MarkSetVisitor) error {
	deadCids := make([]cid.Cid, 0, batchSize)
	var purgeCnt, liveCnt int
	defer func() {
		log.Infow("purged cold objects", "purged", purgeCnt, "live", liveCnt)
	}()

	return s.purgeBatch(cids,
		func(cids []cid.Cid) error {
			deadCids := deadCids[:0]

			for {
				if err := s.checkClosing(); err != nil {
					return err
				}

				s.txnLk.Lock()
				if len(s.txnRefs) == 0 {
					// keep the lock!
					break
				}

				// unlock and protect
				s.txnLk.Unlock()

				err := s.protectTxnRefs(markSet)
				if err != nil {
					return xerrors.Errorf("error protecting transactional refs: %w", err)
				}
			}

			defer s.txnLk.Unlock()

			for _, c := range cids {
				live, err := markSet.Has(c)
				if err != nil {
					return xerrors.Errorf("error checking for liveness: %w", err)
				}

				if live {
					liveCnt++
					continue
				}

				deadCids = append(deadCids, c)
			}

			err := s.hot.DeleteMany(deadCids)
			if err != nil {
				return xerrors.Errorf("error purging cold objects: %w", err)
			}

			s.debug.LogDelete(deadCids)

			purgeCnt += len(deadCids)
			return nil
		})
}

// I really don't like having this code, but we seem to have some occasional DAG references with
// missing constituents. During testing in mainnet *some* of these references *sometimes* appeared
// after a little bit.
// We need to figure out where they are coming from and eliminate that vector, but until then we
// have this gem[TM].
// My best guess is that they are parent message receipts or yet to be computed state roots; magik
// thinks the cause may be block validation.
func (s *SplitStore) waitForMissingRefs(markSet MarkSetVisitor) {
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
		visitor := tmpVisitor()
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
