package splitstore

import (
	"errors"
	"runtime"
	"sync/atomic"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

var (
	errReifyLimit = errors.New("reification limit reached")
	ReifyLimit    = 16384
)

func (s *SplitStore) reifyColdObject(c cid.Cid) {
	if !s.isWarm() {
		return
	}

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
		s.reifyWorkers.Add(1)
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
	defer s.reifyWorkers.Done()
	for c := range workch {
		s.doReify(c)
	}
}

func (s *SplitStore) doReify(c cid.Cid) {
	var toreify, toforget []cid.Cid

	defer func() {
		s.reifyMx.Lock()
		defer s.reifyMx.Unlock()

		for _, c := range toreify {
			delete(s.reifyInProgress, c)
		}
		for _, c := range toforget {
			delete(s.reifyInProgress, c)
		}
	}()

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	count := 0
	_, err := s.walkObjectIncomplete(c, newTmpVisitor(),
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			count++
			if count > ReifyLimit {
				return errReifyLimit
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

			has, err := s.hot.Has(s.ctx, c)
			if err != nil {
				return xerrors.Errorf("error checking hotstore: %w", err)
			}

			// All reified blocks are tracked at reification start
			if has {
				toforget = append(toforget, c)
				return errStopWalk
			}

			toreify = append(toreify, c)
			return nil
		},
		func(missing cid.Cid) error {
			log.Warnf("missing reference while reifying %s: %s", c, missing)
			return errStopWalk
		})

	if err != nil {
		if errors.Is(err, errReifyLimit) {
			log.Debug("reification aborted; reify limit reached")
			return
		}

		log.Warnf("error walking cold object for reification (cid: %s): %s", c, err)
		return
	}

	log.Debugf("reifying %d objects rooted at %s", len(toreify), c)

	// this should not get too big, maybe some 100s of objects.
	batch := make([]blocks.Block, 0, len(toreify))
	for _, c := range toreify {
		blk, err := s.cold.Get(s.ctx, c)
		if err != nil {
			log.Warnf("error retrieving cold object for reification (cid: %s): %s", c, err)
			continue
		}

		if err := s.checkClosing(); err != nil {
			return
		}

		batch = append(batch, blk)
	}

	if len(batch) > 0 {
		err = s.hot.PutMany(s.ctx, batch)
		if err != nil {
			log.Warnf("error reifying cold object (cid: %s): %s", c, err)
			return
		}
	}

}
