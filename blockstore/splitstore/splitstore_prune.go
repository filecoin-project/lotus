package splitstore

import (
	"bytes"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
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
)

// PruneChain instructs the SplitStore to prune chain state in the coldstore, according to the
// options specified.
func (s *SplitStore) PruneChain(opts map[string]interface{}) error {
	// options
	var onlineGC, movingGC bool
	var movePath string
	var retainState int64 = -1

	for k, v := range opts {
		switch k {
		case PruneOnlineGC:
			onlineGC = true
		case PruneMovingGC:
			path, ok := v.(string)
			if !ok {
				return xerrors.Errorf("bad prune path specification; expected string but got %T", v)
			}
			movePath = path
		case PruneRetainState:
			retain, ok := v.(int64)
			if !ok {
				return xerrors.Errorf("bad state retention spceification; expected integer but got %T", v)
			}
			retainState = retain
		default:
			return xerrors.Errorf("unrecognized option %+T", k)
		}
	}

	doGC := func() error { return nil }
	if onlineGC && movingGC {
		return xerrors.Errorf("at most one of online, moving GC can be specified")
	}
	if onlineGC {
		if _, ok := s.cold.(bstore.BlockstoreGC); !ok {
			return xerrors.Errorf("coldstore does not support online GC")
		}
		doGC = func() error { return s.gcBlockstoreOnline(s.cold) }
	}
	if movingGC {
		if _, ok := s.cold.(bstore.BlockstoreMover); !ok {
			return xerrors.Errorf("coldstore does not support moving GC")
		}
		doGC = func() error { return s.gcBlockstoreMoving(s.cold, movePath, nil) }
	}

	var retainStateP func(int64) bool
	switch {
	case retainState > 0:
		retainStateP = func(depth int64) bool {
			return depth <= int64(CompactionBoundary)+retainState*int64(build.Finality)
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
		return xerrors.Errorf("compaction or warmup in progress")
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
	log.Info("waiting for active views to complete")
	start := time.Now()
	s.viewWait()
	log.Infow("waiting for active views done", "took", time.Since(start))

	err := s.doPrune(curTs, retainStateP, doGC)
	if err != nil {
		log.Errorf("PRUNE ERROR: %s", err)
	}
}

func (s *SplitStore) doPrune(curTs *types.TipSet, retainStateP func(int64) bool, doGC func() error) error {
	currentEpoch := curTs.Height()
	log.Infow("running prune", "currentEpoch", currentEpoch, "baseEpoch", s.baseEpoch)

	markSet, err := s.markSetEnv.Create("live", s.markSetSize)
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

	// 1. mark reachable objects by walking the chain from the current epoch; we keep all messages
	//    and chain headers; state and reciepts are retained only if it is within retention policy scope
	log.Info("marking reachable objects")
	startMark := time.Now()

	var count int64
	err = s.walkChainDeep(curTs, retainStateP,
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			count++
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

	// 2. iterate through the hotstore to collect dead objects
	log.Info("collecting dead objects")
	startCollect := time.Now()

	// some stats for logging
	var liveCnt, deadCnt int

	dead := make([]cid.Cid, 0, s.coldPurgeSize)
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
		dead = append(dead, c)
		deadCnt++

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error dead objects: %w", err)
	}

	log.Infow("dead collection done", "took", time.Since(startCollect))
	log.Infow("prune stats", "live", liveCnt, "dead", deadCnt)

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 3. sort dead objects so that the dags with most references are deleted first
	//    this ensures that we can't refer to a dag with its consituents already deleted, ie
	//    we lave no dangling references.
	log.Info("sorting dead objects")
	startSort := time.Now()
	err = s.sortObjects(dead)
	if err != nil {
		return xerrors.Errorf("error sorting objects: %w", err)
	}
	log.Infow("sorting done", "took", time.Since(startSort))

	// 3.1 protect transactional refs once more
	//     strictly speaking, this is not necessary as purge will do it before deleting each
	//     batch.  however, there is likely a largish number of references accumulated during the sort
	//     and this protects before entering purge context
	err = s.protectTxnRefs(markSet)
	if err != nil {
		return xerrors.Errorf("error protecting transactional refs: %w", err)
	}

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 4. purge dead objects from the coldstore, taking protected references into account
	log.Info("purging dead objects from the coldstore")
	startPurge := time.Now()
	err = s.purge(s.cold, dead, markSet)
	if err != nil {
		return xerrors.Errorf("error purging dead objects: %w", err)
	}
	log.Infow("purging dead objects from coldstore done", "took", time.Since(startPurge))

	// we are done; end the transaction and garbage collect
	s.endTxnProtect()

	err = doGC()
	if err != nil {
		log.Warnf("error garbage collecting cold store: %s", err)
	}

	return nil
}

// like walkChain but peforms a deep walk, using walkObjectIncomplete, whereby all messages
// are retained and state roots are retained if they satisfy the given predicate
func (s *SplitStore) walkChainDeep(ts *types.TipSet, retainStateP func(int64) bool,
	f func(cid.Cid) error) error {
	visited := cid.NewSet()
	walked := cid.NewSet()
	toWalk := ts.Cids()
	walkCnt := 0

	baseEpoch := ts.Height()
	stopWalk := func(_ cid.Cid) error { return errStopWalk }

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

		depth := int64(baseEpoch - hdr.Height)
		retainState := retainStateP(depth)

		if hdr.Height > 0 {
			if err := s.walkObjectIncomplete(hdr.Messages, walked, f, stopWalk); err != nil {
				return xerrors.Errorf("error walking messages (cid: %s): %w", hdr.Messages, err)
			}

			if retainState {
				if err := s.walkObjectIncomplete(hdr.ParentMessageReceipts, walked, f, stopWalk); err != nil {
					return xerrors.Errorf("error walking message receipts (cid: %s): %w", hdr.ParentMessageReceipts, err)
				}
			}
		}

		if retainState || hdr.Height == 0 {
			if err := s.walkObjectIncomplete(hdr.ParentStateRoot, walked, f, stopWalk); err != nil {
				return xerrors.Errorf("error walking state root (cid: %s): %w", hdr.ParentStateRoot, err)
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

		walking := toWalk
		toWalk = nil
		for _, c := range walking {
			if err := walkBlock(c); err != nil {
				return xerrors.Errorf("error walking block (cid: %s): %w", c, err)
			}
		}
	}

	log.Infow("chain walk done", "walked", walkCnt)

	return nil
}
