package splitstore

import (
	"bytes"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"
)

var (
	// PruneOnline is a prune option that instructs PruneChain to use online gc for reclaiming space;
	// there is no value associated with this option.
	PruneOnlineGC = "splitstore.PruneOnlineGC"

	// PruneMoving is a prune option that instructs PruneChain to use moving gc for reclaiming space;
	// the value associated with this option is the path of the new coldstore.
	PruneMovingGC = "splitstore.PruneMovingGC"

	// PruneRetainState is a prune option that instructs PruneChain as to how many finalities worth
	// of state to retain in the coldstore.
	// The value is an integer:
	// - if it is -1 then all state objects reachable from the chain will be retained in the coldstore.
	//   this is useful for garbage collecting side-chains and other garbage in archival nodes.
	//   This is the (safe) default.
	// - if it is 0 then no state objects that are unreachable within the compaction boundary will
	//   be retained in the coldstore.
	// - if it is a positive integer, then it's the number of finalities past the compaction boundary
	//   for which chain-reachable state objects are retained.
	PruneRetainState = "splitstore.PruneRetainState"

	// PruneThreshold is the number of epochs that need to have elapsed
	// from the previously pruned epoch to trigger a new prune
	PruneThreshold = 7 * policy.ChainFinality
)

// GCHotStore runs online GC on the chain state in the hotstore according the to options specified
func (s *SplitStore) GCHotStore(opts api.HotGCOpts) error {
	if opts.Moving {
		gcOpts := []bstore.BlockstoreGCOption{bstore.WithFullGC(true)}
		return s.gcBlockstore(s.hot, gcOpts)
	}

	gcOpts := []bstore.BlockstoreGCOption{bstore.WithThreshold(opts.Threshold)}
	var err error
	if opts.Periodic {
		err = s.gcBlockstore(s.hot, gcOpts)
	} else {
		err = s.gcBlockstoreOnce(s.hot, gcOpts)
	}
	return err
}

// PruneChain instructs the SplitStore to prune chain state in the coldstore, according to the
// options specified.
func (s *SplitStore) PruneChain(opts api.PruneOpts) error {
	retainState := opts.RetainState

	var gcOpts []bstore.BlockstoreGCOption
	if opts.MovingGC {
		gcOpts = append(gcOpts, bstore.WithFullGC(true))
	}
	doGC := func() error { return s.gcBlockstore(s.cold, gcOpts) }

	var retainStateP func(int64) bool
	switch {
	case retainState > 0:
		retainStateP = func(depth int64) bool {
			return depth <= int64(CompactionBoundary)+retainState*int64(policy.ChainFinality)
		}
	case retainState < 0:
		retainStateP = func(_ int64) bool { return true }
	default:
		retainStateP = func(depth int64) bool {
			return depth <= int64(CompactionBoundary)
		}
	}

	if _, ok := s.cold.(bstore.BlockstoreIterator); !ok {
		return xerrors.Errorf("coldstore does not support efficient iteration")
	}

	return s.pruneChain(retainStateP, doGC)
}

func (s *SplitStore) pruneChain(retainStateP func(int64) bool, doGC func() error) error {
	// inhibit compaction while we are setting up
	s.headChangeMx.Lock()
	defer s.headChangeMx.Unlock()

	// take the compaction lock; fail if there is a compaction in progress
	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		return xerrors.Errorf("compaction, prune or warmup in progress")
	}

	// check if we are actually closing first
	if atomic.LoadInt32(&s.closing) == 1 {
		atomic.StoreInt32(&s.compacting, 0)
		return errClosing
	}

	// ensure that we have compacted at least once
	if s.compactionIndex == 0 {
		atomic.StoreInt32(&s.compacting, 0)
		return xerrors.Errorf("splitstore has not compacted yet")
	}

	// get the current tipset
	curTs := s.chain.GetHeaviestTipSet()

	// begin the transaction and go
	s.beginTxnProtect()
	s.compactType = cold
	go func() {
		defer atomic.StoreInt32(&s.compacting, 0)
		defer s.endTxnProtect()

		log.Info("pruning splitstore")
		start := time.Now()

		s.prune(curTs, retainStateP, doGC)

		log.Infow("prune done", "took", time.Since(start))
	}()

	return nil
}

func (s *SplitStore) prune(curTs *types.TipSet, retainStateP func(int64) bool, doGC func() error) {
	log.Debug("waiting for active views to complete")
	start := time.Now()
	s.viewWait()
	log.Debugw("waiting for active views done", "took", time.Since(start))

	err := s.doPrune(curTs, retainStateP, doGC)
	if err != nil {
		log.Errorf("PRUNE ERROR: %s", err)
	}
}

func (s *SplitStore) doPrune(curTs *types.TipSet, retainStateP func(int64) bool, doGC func() error) error {
	currentEpoch := curTs.Height()
	boundaryEpoch := currentEpoch - CompactionBoundary

	log.Infow("running prune", "currentEpoch", currentEpoch, "pruneEpoch", s.pruneEpoch)

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

	// 1. mark reachable objects by walking the chain from the current epoch; we keep all messages
	//    and chain headers; state and receipts are retained only if it is within retention policy scope
	log.Info("marking reachable objects")
	startMark := time.Now()

	count := new(int64)
	err = s.walkChainDeep(curTs, retainStateP,
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			mark, err := markSet.Has(c)
			if err != nil {
				return xerrors.Errorf("error checking markset: %w", err)
			}

			if mark {
				return errStopWalk
			}

			atomic.AddInt64(count, 1)
			return markSet.Mark(c)
		})

	if err != nil {
		return xerrors.Errorf("error marking: %w", err)
	}

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

	// 2. iterate through the coldstore to collect dead objects
	log.Info("collecting dead objects")
	startCollect := time.Now()

	deadw, err := NewColdSetWriter(s.discardSetPath())
	if err != nil {
		return xerrors.Errorf("error creating coldset: %w", err)
	}
	defer deadw.Close() //nolint:errcheck

	// some stats for logging
	var liveCnt, deadCnt int

	err = s.cold.(bstore.BlockstoreIterator).ForEachKey(func(c cid.Cid) error {
		// was it marked?
		mark, err := markSet.Has(c)
		if err != nil {
			return xerrors.Errorf("error checking mark set for %s: %w", c, err)
		}

		if mark {
			liveCnt++
			return nil
		}

		// Note: a possible optimization here is to also purge objects that are in the hotstore
		//       but this needs special care not to purge genesis state, so we don't bother (yet)

		// it's dead in the coldstore, mark it as candidate for purge

		if err := deadw.Write(c); err != nil {
			return xerrors.Errorf("error writing cid to coldstore: %w", err)
		}
		deadCnt++

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error dead objects: %w", err)
	}

	if err := deadw.Close(); err != nil {
		return xerrors.Errorf("error closing deadset: %w", err)
	}

	stats.Record(s.ctx, metrics.SplitstoreCompactionDead.M(int64(deadCnt)))

	log.Infow("dead collection done", "took", time.Since(startCollect))
	log.Infow("prune stats", "live", liveCnt, "dead", deadCnt)

	if err := s.checkClosing(); err != nil {
		return err
	}

	// now that we have collected dead objects, check for missing references from transactional i/o
	// this is carried over from hot compaction for completeness
	s.waitForMissingRefs(markSet)

	if err := s.checkClosing(); err != nil {
		return err
	}

	deadr, err := NewColdSetReader(s.discardSetPath())
	if err != nil {
		return xerrors.Errorf("error opening deadset: %w", err)
	}
	defer deadr.Close() //nolint:errcheck

	// 3. Purge dead objects with checkpointing for recovery.
	// This is the critical section of prune, whereby any dead object not in the markSet is
	// considered already deleted.
	// We delete dead objects in batches, holding the transaction lock, where we check the markSet
	// again for new references created by the caller.
	// After each batch we write a checkpoint to disk; if the process is interrupted before completion
	// the process will continue from the checkpoint in the next recovery.
	if err := s.beginCriticalSection(markSet); err != nil {
		return xerrors.Errorf("error beginning critical section: %w", err)
	}

	if err := s.checkClosing(); err != nil {
		return err
	}

	checkpoint, err := NewCheckpoint(s.pruneCheckpointPath())
	if err != nil {
		return xerrors.Errorf("error creating checkpoint: %w", err)
	}
	defer checkpoint.Close() //nolint:errcheck

	log.Info("purging dead objects from the coldstore")
	startPurge := time.Now()
	err = s.purge(deadr, checkpoint, markSet)
	if err != nil {
		return xerrors.Errorf("error purging dead objects: %w", err)
	}
	log.Infow("purging dead objects from coldstore done", "took", time.Since(startPurge))

	s.endCriticalSection()

	if err := checkpoint.Close(); err != nil {
		log.Warnf("error closing checkpoint: %s", err)
	}
	if err := os.Remove(s.pruneCheckpointPath()); err != nil {
		log.Warnf("error removing checkpoint: %s", err)
	}
	if err := deadr.Close(); err != nil {
		log.Warnf("error closing discard set: %s", err)
	}
	if err := os.Remove(s.discardSetPath()); err != nil {
		log.Warnf("error removing discard set: %s", err)
	}

	// we are done; do some housekeeping
	s.endTxnProtect()
	err = doGC()
	if err != nil {
		log.Warnf("error garbage collecting cold store: %s", err)
	}

	if err := s.setPruneEpoch(boundaryEpoch); err != nil {
		return xerrors.Errorf("error saving prune base epoch: %w", err)
	}

	s.pruneIndex++
	err = s.ds.Put(s.ctx, pruneIndexKey, int64ToBytes(s.pruneIndex))
	if err != nil {
		return xerrors.Errorf("error saving prune index: %w", err)
	}

	return nil
}

func (s *SplitStore) completePrune() error {
	checkpoint, last, err := OpenCheckpoint(s.pruneCheckpointPath())
	if err != nil {
		return xerrors.Errorf("error opening checkpoint: %w", err)
	}
	defer checkpoint.Close() //nolint:errcheck

	deadr, err := NewColdSetReader(s.discardSetPath())
	if err != nil {
		return xerrors.Errorf("error opening deadset: %w", err)
	}
	defer deadr.Close() //nolint:errcheck

	markSet, err := s.markSetEnv.Recover("live")
	if err != nil {
		return xerrors.Errorf("error recovering markset: %w", err)
	}
	defer markSet.Close() //nolint:errcheck

	// PURGE!
	s.compactType = cold
	log.Info("purging dead objects from the coldstore")
	startPurge := time.Now()
	err = s.completePurge(deadr, checkpoint, last, markSet)
	if err != nil {
		return xerrors.Errorf("error purgin dead objects: %w", err)
	}
	log.Infow("purging dead objects from the coldstore done", "took", time.Since(startPurge))

	markSet.EndCriticalSection()
	s.compactType = none

	if err := checkpoint.Close(); err != nil {
		log.Warnf("error closing checkpoint: %s", err)
	}
	if err := os.Remove(s.pruneCheckpointPath()); err != nil {
		log.Warnf("error removing checkpoint: %s", err)
	}
	if err := deadr.Close(); err != nil {
		log.Warnf("error closing deadset: %s", err)
	}
	if err := os.Remove(s.discardSetPath()); err != nil {
		log.Warnf("error removing deadset: %s", err)
	}

	return nil
}

// like walkChain but performs a deep walk, using parallel walking with walkObjectLax,
// whereby all extant messages are retained and state roots are retained if they satisfy
// the given predicate.
// missing references are ignored, as we expect to have plenty for snapshot syncs.
func (s *SplitStore) walkChainDeep(ts *types.TipSet, retainStateP func(int64) bool,
	f func(cid.Cid) error) error {
	visited := cid.NewSet()
	toWalk := ts.Cids()
	walkCnt := 0

	workers := runtime.NumCPU() / 2
	if workers < 2 {
		workers = 2
	}

	var wg sync.WaitGroup
	workch := make(chan cid.Cid, 16*workers)
	errch := make(chan error, workers)

	var once sync.Once
	defer once.Do(func() { close(workch) })

	push := func(c cid.Cid) error {
		if !visited.Visit(c) {
			return nil
		}

		select {
		case workch <- c:
			return nil
		case err := <-errch:
			return err
		}
	}

	worker := func() {
		defer wg.Done()
		for c := range workch {
			err := s.walkObjectLax(c, f)
			if err != nil {
				errch <- xerrors.Errorf("error walking object (cid: %s): %w", c, err)
				return
			}
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}

	baseEpoch := ts.Height()
	minEpoch := baseEpoch // for progress report
	log.Infof("walking at epoch %d", minEpoch)

	walkBlock := func(c cid.Cid) error {
		if !visited.Visit(c) {
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

		if hdr.Height < minEpoch {
			minEpoch = hdr.Height
			if minEpoch%10_000 == 0 {
				log.Infof("walking at epoch %d (walked: %d)", minEpoch, walkCnt)
			}
		}

		depth := int64(baseEpoch - hdr.Height)
		retainState := retainStateP(depth)

		if hdr.Height > 0 {
			if err := push(hdr.Messages); err != nil {
				return err
			}
			if retainState {
				if err := push(hdr.ParentMessageReceipts); err != nil {
					return err
				}
			}
		}

		if retainState || hdr.Height == 0 {
			if err := push(hdr.ParentStateRoot); err != nil {
				return err
			}
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

		select {
		case err := <-errch:
			return err
		default:
		}

		walking := toWalk
		toWalk = nil
		for _, c := range walking {
			if err := walkBlock(c); err != nil {
				return xerrors.Errorf("error walking block (cid: %s): %w", c, err)
			}
		}
	}

	once.Do(func() { close(workch) })
	wg.Wait()
	select {
	case err := <-errch:
		return err
	default:
	}

	log.Infow("chain walk done", "walked", walkCnt)

	return nil
}

// like walkObject but treats missing references laxly; faster version of walkObjectIncomplete
// without an occurs check.
func (s *SplitStore) walkObjectLax(c cid.Cid, f func(cid.Cid) error) error {
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
	err := s.view(c, func(data []byte) error {
		return cbg.ScanForLinks(bytes.NewReader(data), func(c cid.Cid) {
			links = append(links, c)
		})
	})

	if err != nil {
		if ipld.IsNotFound(err) { // not a problem for deep walks
			return nil
		}

		return xerrors.Errorf("error scanning linked block (cid: %s): %w", c, err)
	}

	for _, c := range links {
		err := s.walkObjectLax(c, f)
		if err != nil {
			return xerrors.Errorf("error walking link (cid: %s): %w", c, err)
		}
	}

	return nil
}
