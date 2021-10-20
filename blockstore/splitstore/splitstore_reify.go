package splitstore

import (
	"runtime"
	"sync/atomic"

	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
)

func (s *SplitStore) reifyColdObject(c cid.Cid) {
	if isUnitaryObject(c) {
		return
	}

	s.reifyMx.Lock()
	defer s.reifyMx.Unlock()

	_, ok := s.reifyInProgress[c]
	if ok {
		return
	}

	s.reifyPend[c] = struct{}{}
	s.reifyCond.Broadcast()
}

func (s *SplitStore) reifyOrchestrator() {
	workers := runtime.NumCPU() / 4
	if workers < 2 {
		workers = 2
	}

	workch := make(chan cid.Cid, workers)
	defer close(workch)

	for i := 0; i < workers; i++ {
		go s.reifyWorker(workch)
	}

	for {
		s.reifyMx.Lock()
		for len(s.reifyPend) == 0 && atomic.LoadInt32(&s.closing) == 0 {
			s.reifyCond.Wait()
		}

		if atomic.LoadInt32(&s.closing) != 0 {
			s.reifyMx.Unlock()
			return
		}

		reifyPend := s.reifyPend
		s.reifyPend = make(map[cid.Cid]struct{})
		s.reifyMx.Unlock()

		for c := range reifyPend {
			select {
			case workch <- c:
			case <-s.ctx.Done():
				return
			}
		}
	}
}

func (s *SplitStore) reifyWorker(workch chan cid.Cid) {
	for c := range workch {
		s.doReify(c)
	}
}

func (s *SplitStore) doReify(c cid.Cid) {
	s.txnLk.RLock()
	s.trackTxnRef(c)
	s.txnLk.RUnlock()

	var toreify []cid.Cid
	err := s.walkObject(c, tmpVisitor(),
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			s.reifyMx.Lock()
			_, inProgress := s.reifyInProgress[c]
			if !inProgress {
				s.reifyInProgress[c] = struct{}{}
			}
			s.reifyMx.Unlock()

			if inProgress {
				return errStopWalk
			}

			has, err := s.hot.Has(c)
			if err != nil {
				return xerrors.Errorf("error checking hotstore: %w", err)
			}

			if has {
				return errStopWalk
			}

			toreify = append(toreify, c)
			return nil
		})

	defer func() {
		if len(toreify) == 0 {
			return
		}

		s.reifyMx.Lock()
		for _, c := range toreify {
			delete(s.reifyInProgress, c)
		}
		s.reifyMx.Unlock()
	}()

	if err != nil {
		log.Warnf("error walking cold object for reification (cid: %s): %s", c, err)
		return
	}

	if len(toreify) == 0 {
		return
	}

	batch := make([]blocks.Block, 0, len(toreify))
	for _, c := range toreify {
		blk, err := s.cold.Get(c)
		if err != nil {
			log.Warnf("error retrieving cold object for reification (cid: %s): %s", c, err)
			continue
		}

		batch = append(batch, blk)
	}

	if len(batch) == 0 {
		return
	}

	err = s.hot.PutMany(batch)
	if err != nil {
		log.Warnf("error reifying cold object (cid: %s): %s", c, err)
	}
}
