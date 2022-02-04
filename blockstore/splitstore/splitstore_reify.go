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
	var toreify, totrack, toforget []cid.Cid

	defer func() {
		s.reifyMx.Lock()
		defer s.reifyMx.Unlock()

		for _, c := range toreify {
			delete(s.reifyInProgress, c)
		}
		for _, c := range totrack {
			delete(s.reifyInProgress, c)
		}
		for _, c := range toforget {
			delete(s.reifyInProgress, c)
		}
	}()

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	err := s.walkObject(c, newTmpVisitor(),
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

			has, err := s.hot.Has(s.ctx, c)
			if err != nil {
				return xerrors.Errorf("error checking hotstore: %w", err)
			}

			if has {
				if s.txnMarkSet != nil {
					hasMark, err := s.txnMarkSet.Has(c)
					if err != nil {
						log.Warnf("error checking markset: %s", err)
					} else if hasMark {
						toforget = append(toforget, c)
						return errStopWalk
					}
				} else {
					totrack = append(totrack, c)
					return errStopWalk
				}
			}

			toreify = append(toreify, c)
			return nil
		})

	if err != nil {
		log.Warnf("error walking cold object for reification (cid: %s): %s", c, err)
		return
	}

	log.Debugf("reifying %d objects rooted at %s", len(toreify), c)

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

	if s.txnMarkSet != nil {
		if len(toreify) > 0 {
			s.markLiveRefs(toreify)
		}
		if len(totrack) > 0 {
			s.markLiveRefs(totrack)
		}
	} else {
		if len(toreify) > 0 {
			s.trackTxnRefMany(toreify)
		}
		if len(totrack) > 0 {
			s.trackTxnRefMany(totrack)
		}
	}
}
