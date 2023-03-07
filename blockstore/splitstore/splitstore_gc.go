package splitstore

import (
	"fmt"
	"time"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

const (
	// When < 150 GB of space would remain during moving GC, trigger moving GC
	targetThreshold = 150_000_000_000
	// Don't attempt moving GC with 50 GB or less would remain during moving GC
	targetBuffer = 50_000_000_000
	// Fraction of garbage in badger vlog for online GC traversal to collect garbage
	aggressiveOnlineGCThreshold = 0.0001
)

func (s *SplitStore) gcHotAfterCompaction() {
	// Measure hotstore size, determine if we should do full GC, determine if we can do full GC.
	// We should do full GC if
	//  FullGCFrequency is specified and compaction index matches frequency
	//  OR HotstoreMaxSpaceTarget is specified and total moving space is within 150 GB of target
	// We can do full if
	//  HotstoreMaxSpaceTarget is not specified
	//  OR total moving space would not exceed 50 GB below target
	//
	// 	a) If we should not do full GC => online GC
	//  b) If we should do full GC and can => moving GC
	//  c) If we should do full GC and can't => aggressive online GC
	var hotSize int64
	var err error
	sizer, ok := s.hot.(bstore.BlockstoreSize)
	if ok {
		hotSize, err = sizer.Size()
		if err != nil {
			log.Warnf("error getting hotstore size: %s, estimating empty hot store for targeting", err)
			hotSize = 0
		}
	} else {
		hotSize = 0
	}

	copySizeApprox := s.szKeys + s.szMarkedLiveRefs + s.szProtectedTxns + s.szWalk
	shouldTarget := s.cfg.HotstoreMaxSpaceTarget > 0 && hotSize+copySizeApprox > int64(s.cfg.HotstoreMaxSpaceTarget)-targetThreshold
	shouldFreq := s.cfg.HotStoreFullGCFrequency > 0 && s.compactionIndex%int64(s.cfg.HotStoreFullGCFrequency) == 0
	shouldDoFull := shouldTarget || shouldFreq
	canDoFull := s.cfg.HotstoreMaxSpaceTarget == 0 || hotSize+copySizeApprox < int64(s.cfg.HotstoreMaxSpaceTarget)-targetBuffer
	log.Infof("measured hot store size: %d, approximate new size: %d, should do full %t, can do full %t", hotSize, copySizeApprox, shouldDoFull, canDoFull)

	var opts []bstore.BlockstoreGCOption
	if shouldDoFull && canDoFull {
		opts = append(opts, bstore.WithFullGC(true))
	} else if shouldDoFull && !canDoFull {
		log.Warnf("Attention! Estimated moving GC size %d is not within safety buffer %d of target max %d, performing aggressive online GC to attempt to bring hotstore size down safely", copySizeApprox, targetBuffer, s.cfg.HotstoreMaxSpaceTarget)
		log.Warn("If problem continues you can 1) temporarily allocate more disk space to hotstore and 2) reflect in HotstoreMaxSpaceTarget OR trigger manual move with `lotus chain prune hot-moving`")
		log.Warn("If problem continues and you do not have any more disk space you can run continue to manually trigger online GC at aggressive thresholds (< 0.01) with `lotus chain prune hot`")

		opts = append(opts, bstore.WithThreshold(aggressiveOnlineGCThreshold))
	}

	if err := s.gcBlockstore(s.hot, opts); err != nil {
		log.Warnf("error garbage collecting hostore: %s", err)
	}
}

func (s *SplitStore) gcBlockstore(b bstore.Blockstore, opts []bstore.BlockstoreGCOption) error {
	if gc, ok := b.(bstore.BlockstoreGC); ok {
		log.Info("garbage collecting blockstore")
		startGC := time.Now()

		if err := gc.CollectGarbage(s.ctx, opts...); err != nil {
			return err
		}

		log.Infow("garbage collecting blockstore done", "took", time.Since(startGC))
		return nil
	}

	return fmt.Errorf("blockstore doesn't support garbage collection: %T", b)
}

func (s *SplitStore) gcBlockstoreOnce(b bstore.Blockstore, opts []bstore.BlockstoreGCOption) error {
	if gc, ok := b.(bstore.BlockstoreGCOnce); ok {
		log.Debug("gc blockstore once")
		startGC := time.Now()

		if err := gc.GCOnce(s.ctx, opts...); err != nil {
			return err
		}

		log.Debugw("gc blockstore once done", "took", time.Since(startGC))
		return nil
	}

	return fmt.Errorf("blockstore doesn't support gc once: %T", b)
}
