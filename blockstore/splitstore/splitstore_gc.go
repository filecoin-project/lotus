package splitstore

import (
	"fmt"
	"time"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

func (s *SplitStore) gcHotstore() {
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

		if err := gc.CollectGarbage(opts...); err != nil {
			return err
		}

		log.Infow("garbage collecting hotstore done", "took", time.Since(startGC))
		return nil
	}

	return fmt.Errorf("blockstore doesn't support garbage collection: %T", b)
}
