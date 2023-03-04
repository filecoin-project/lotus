package splitstore

import (
	"fmt"
	"time"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

func (s *SplitStore) gcHotAfterCompaction() {
	// TODO size aware GC
	// 1. Add a config value to specify targetted max number of bytes M
	// 2. Use measurement of marked hotstore size H (we now have this), actual hostore size T (need to compute this), total move size H + T, approximate purged size P
	// 3. Trigger moving GC whenever H + T is within 50 GB of M
	// 4. if H + T > M use aggressive online threshold
	// 5. Use threshold that covers 3 std devs of vlogs when doing aggresive online.  Mean == (H + P) / T, assume normal distribution
	// 6. Use threshold that covers 1 or 2 std devs of vlogs when doing regular online GC
	var opts []bstore.BlockstoreGCOption
	if s.cfg.HotStoreFullGCFrequency > 0 && s.compactionIndex%int64(s.cfg.HotStoreFullGCFrequency) == 0 {
		opts = append(opts, bstore.WithFullGC(true))
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
