package splitstore

import (
	"sync"
	"sync/atomic"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
)

var (
	// WarmupBoundary is the number of epochs to load state during warmup.
	WarmupBoundary = policy.ChainFinality
)

// warmup acquires the compaction lock and spawns a goroutine to warm up the hotstore;
// this is necessary when we sync from a snapshot or when we enable the splitstore
// on top of an existing blockstore (which becomes the coldstore).
func (s *SplitStore) warmup(curTs *types.TipSet) error {
	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		return xerrors.Errorf("error locking compaction")
	}
	s.compactType = warmup
	go func() {
		defer atomic.StoreInt32(&s.compacting, 0)

		log.Info("warming up hotstore")
		start := time.Now()

		err := s.doWarmup(curTs)
		if err != nil {
			log.Errorf("error warming up hotstore: %s", err)
			return
		}

		log.Infow("warm up done", "took", time.Since(start))
	}()

	return nil
}

// the actual warmup procedure; it walks the chain loading all state roots at the boundary
// and headers all the way up to genesis.
// objects are written in batches so as to minimize overhead.
func (s *SplitStore) doWarmup(curTs *types.TipSet) error {
	var boundaryEpoch abi.ChainEpoch
	epoch := curTs.Height()
	if WarmupBoundary < epoch {
		boundaryEpoch = epoch - WarmupBoundary
	}
	var mx sync.Mutex
	batchHot := make([]blocks.Block, 0, batchSize)
	count := new(int64)
	xcount := new(int64)
	missing := new(int64)

	visitor, err := s.markSetEnv.New("warmup", 0)
	if err != nil {
		return xerrors.Errorf("error creating visitor: %w", err)
	}
	defer visitor.Close() //nolint

	err = s.walkChain(curTs, boundaryEpoch, epoch+1, // we don't load messages/receipts in warmup
		visitor,
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			atomic.AddInt64(count, 1)

			has, err := s.hot.Has(s.ctx, c)
			if err != nil {
				return err
			}

			if has {
				return nil
			}

			blk, err := s.cold.Get(s.ctx, c)
			if err != nil {
				if ipld.IsNotFound(err) {
					atomic.AddInt64(missing, 1)
					return errStopWalk
				}
				return err
			}

			atomic.AddInt64(xcount, 1)

			mx.Lock()
			batchHot = append(batchHot, blk)
			if len(batchHot) == batchSize {
				err = s.hot.PutMany(s.ctx, batchHot)
				if err != nil {
					mx.Unlock()
					return err
				}
				batchHot = batchHot[:0]
			}
			mx.Unlock()

			return nil
		}, func(cid.Cid) error { return nil })

	if err != nil {
		return err
	}

	if len(batchHot) > 0 {
		err = s.hot.PutMany(s.ctx, batchHot)
		if err != nil {
			return err
		}
	}

	log.Infow("warmup stats", "visited", *count, "warm", *xcount, "missing", *missing)

	s.markSetSize = *count + *count>>2 // overestimate a bit
	err = s.ds.Put(s.ctx, markSetSizeKey, int64ToBytes(s.markSetSize))
	if err != nil {
		log.Warnf("error saving mark set size: %s", err)
	}

	// save the warmup epoch
	err = s.ds.Put(s.ctx, warmupEpochKey, epochToBytes(epoch))
	if err != nil {
		return xerrors.Errorf("error saving warm up epoch: %w", err)
	}

	s.warmupEpoch.Store(int64(epoch))

	// also save the compactionIndex, as this is used as an indicator of warmup for upgraded nodes
	err = s.ds.Put(s.ctx, compactionIndexKey, int64ToBytes(s.compactionIndex))
	if err != nil {
		return xerrors.Errorf("error saving compaction index: %w", err)
	}

	return nil
}
