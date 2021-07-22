package splitstore

import (
	"fmt"
	"time"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

func (s *SplitStore) gcHotstore() {
	if err := s.gcBlockstoreOnline(s.hot); err != nil {
		log.Warnf("error garbage collecting hostore: %s", err)
	}
}

func (s *SplitStore) gcBlockstoreOnline(b bstore.Blockstore) error {
	if gc, ok := b.(bstore.BlockstoreGC); ok {
		log.Info("garbage collecting blockstore")
		startGC := time.Now()

		if err := gc.CollectGarbage(); err != nil {
			return err
		}

		log.Infow("garbage collecting hotstore done", "took", time.Since(startGC))
		return nil
	}

	return fmt.Errorf("blockstore doesn't support online gc: %T", b)
}
