package splitstore

import (
	"fmt"
	"time"

	bstore "github.com/filecoin-project/lotus/blockstore"
	cid "github.com/ipfs/go-cid"
)

func (s *SplitStore) gcHotstore() {
	// we only perform moving gc every 20 compactions (about once a week) as it can take a while
	if s.compactionIndex%20 == 0 {
		if err := s.gcBlockstoreMoving(s.hot, "", nil); err != nil {
			log.Warnf("error moving hotstore: %s", err)
			// fallthrough to online gc
		} else {
			return
		}
	}

	if err := s.gcBlockstoreOnline(s.hot); err != nil {
		log.Warnf("error garbage collecting hostore: %s", err)
	}
}

func (s *SplitStore) gcBlockstoreMoving(b bstore.Blockstore, path string, filter func(cid.Cid) bool) error {
	if mover, ok := b.(bstore.BlockstoreMover); ok {
		log.Info("moving blockstore")
		startMove := time.Now()

		if err := mover.MoveTo(path, filter); err != nil {
			return err
		}

		log.Infow("moving hotstore done", "took", time.Since(startMove))
		return nil
	}

	return fmt.Errorf("blockstore doesn't support moving: %T", b)
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
