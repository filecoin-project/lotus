package splitstore

import (
	"fmt"
	"time"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

func (s *SplitStore) gcHotstore() {
	var opts map[interface{}]interface{}
	if s.cfg.HotStoreMovingGCFrequency > 0 && s.compactionIndex%int64(s.cfg.HotStoreMovingGCFrequency) == 0 {
		opts = map[interface{}]interface{}{
			bstore.BlockstoreMovingGC: true,
		}
	}

	if err := s.gcBlockstore(s.hot, opts); err != nil {
		log.Warnf("error garbage collecting hostore: %s", err)
	}
}

func (s *SplitStore) gcBlockstore(b bstore.Blockstore, opts map[interface{}]interface{}) error {
	if gc, ok := b.(bstore.BlockstoreGC); ok {
		log.Info("garbage collecting blockstore")
		startGC := time.Now()

		if err := gc.CollectGarbage(opts); err != nil {
			return err
		}

		log.Infow("garbage collecting hotstore done", "took", time.Since(startGC))
		return nil
	}

	return fmt.Errorf("blockstore doesn't support garbage collection: %T", b)
}
