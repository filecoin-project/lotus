package splitstore

import (
	"time"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

func (s *SplitStore) gcHotstore() {
	// we only perform moving gc every 10 compactions as it can take a while
	if s.compactionIndex%10 != 0 {
		goto online_gc
	}

	// check if the hotstore is movable; if so, move it.
	if mover, ok := s.hot.(bstore.BlockstoreMover); ok {
		log.Info("moving hotstore")
		startMove := time.Now()
		err := mover.MoveTo("", nil)
		if err != nil {
			log.Warnf("error moving hotstore: %s", err)
			// try online gc
			goto online_gc
		}

		log.Infow("moving hotstore done", "took", time.Since(startMove))
		return
	}

online_gc:
	// check if the hotstore supports online GC; if so, GC it.
	if gc, ok := s.hot.(bstore.BlockstoreGC); ok {
		log.Info("garbage collecting hotstore")
		startGC := time.Now()
		err := gc.CollectGarbage()
		if err != nil {
			log.Warnf("error garbage collecting hotstore: %s", err)
			return
		}

		log.Infof("garbage collecting hotstore done", "took", time.Since(startGC))
		return
	}

	return
}
