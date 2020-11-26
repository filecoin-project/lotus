package splitstore

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
)

const CompactionThreshold = 5 * build.Finality

var baseEpochKey = dstore.NewKey("baseEpoch")

var log = logging.Logger("splitstore")

type SplitStore struct {
	baseEpoch abi.ChainEpoch
	curTs     *types.TipSet

	cs *store.ChainStore
	ds dstore.Datastore

	hot  bstore.Blockstore
	cold bstore.Blockstore

	snoop TrackingStore

	stateMx    sync.Mutex
	compacting bool
}

var _ bstore.Blockstore = (*SplitStore)(nil)

// Blockstore interface
func (s *SplitStore) DeleteBlock(cid cid.Cid) error {
	// afaict we don't seem to be using this method, so it's not implemented
	return errors.New("DeleteBlock not implemented on SplitStore; don't do this Luke!") //nolint
}

func (s *SplitStore) Has(cid cid.Cid) (bool, error) {
	has, err := s.hot.Has(cid)

	if err != nil {
		return false, err
	}

	if has {
		return true, nil
	}

	return s.cold.Has(cid)
}

func (s *SplitStore) Get(cid cid.Cid) (blocks.Block, error) {
	blk, err := s.hot.Get(cid)

	switch err {
	case nil:
		return blk, nil

	case bstore.ErrNotFound:
		return s.cold.Get(cid)

	default:
		return nil, err
	}
}

func (s *SplitStore) GetSize(cid cid.Cid) (int, error) {
	size, err := s.hot.GetSize(cid)

	switch err {
	case nil:
		return size, nil

	case bstore.ErrNotFound:
		return s.cold.GetSize(cid)

	default:
		return 0, err
	}
}

func (s *SplitStore) Put(blk blocks.Block) error {
	epoch := s.curTs.Height()
	err := s.snoop.Put(blk.Cid(), epoch)
	if err != nil {
		return err
	}

	return s.hot.Put(blk)
}

func (s *SplitStore) PutMany(blks []blocks.Block) error {
	err := s.hot.PutMany(blks)
	if err != nil {
		return err
	}

	epoch := s.curTs.Height()

	batch := make([]cid.Cid, 0, len(blks))
	for _, blk := range blks {
		batch = append(batch, blk.Cid())
	}

	return s.snoop.PutBatch(batch, epoch)
}

func (s *SplitStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	ctx, cancel := context.WithCancel(ctx)

	chHot, err := s.hot.AllKeysChan(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	chCold, err := s.cold.AllKeysChan(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	ch := make(chan cid.Cid)
	go func() {
		defer cancel()
		defer close(ch)

		for _, in := range []<-chan cid.Cid{chHot, chCold} {
			for cid := range in {
				select {
				case ch <- cid:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

func (s *SplitStore) HashOnRead(enabled bool) {
	s.hot.HashOnRead(enabled)
	s.cold.HashOnRead(enabled)
}

func (s *SplitStore) View(cid cid.Cid, cb func([]byte) error) error {
	err := s.hot.View(cid, cb)
	switch err {
	case bstore.ErrNotFound:
		return s.cold.View(cid, cb)

	default:
		return err
	}
}

// State tracking
func (s *SplitStore) Start(cs *store.ChainStore) error {
	s.cs = cs
	s.curTs = cs.GetHeaviestTipSet()

	// load base epoch from metadata ds
	// if none, then use current epoch because it's a fresh start
	bs, err := s.ds.Get(baseEpochKey)
	switch err {
	case nil:
		epoch, n := binary.Uvarint(bs)
		if n < 0 {
			panic("bogus base epoch")
		}
		s.baseEpoch = abi.ChainEpoch(epoch)

	case dstore.ErrNotFound:
		err = s.setBaseEpoch(s.curTs.Height())
		if err != nil {
			return err
		}

	default:
		return err
	}

	// watch the chain
	cs.SubscribeHeadChanges(s.HeadChange)

	return nil
}

func (s *SplitStore) HeadChange(revert, apply []*types.TipSet) error {
	s.curTs = apply[len(apply)-1]
	epoch := s.curTs.Height()
	if epoch-s.baseEpoch > CompactionThreshold && !s.isCompacting() {
		s.setCompacting(true)
		go func() {
			defer s.setCompacting(false)

			log.Info("compacting splitstore")
			start := time.Now()

			s.compact()

			log.Infow("compaction done", "took", time.Since(start))
		}()
	}

	return nil
}

func (s *SplitStore) isCompacting() bool {
	s.stateMx.Lock()
	defer s.stateMx.Unlock()
	return s.compacting
}

func (s *SplitStore) setCompacting(state bool) {
	s.stateMx.Lock()
	defer s.stateMx.Unlock()
	s.compacting = state
}

// Compaction/GC Algorithm
func (s *SplitStore) compact() {
	// create two on disk live sets, one for marking the cold finality region
	// and one for marking the hot region
	hotSet, err := s.newLiveSet()
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer hotSet.Close() //nolint:errcheck

	coldSet, err := s.newLiveSet()
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer coldSet.Close() //nolint:errcheck

	// Phase 1: marking
	log.Info("marking live objects")
	startMark := time.Now()

	// Phase 1a: mark all reachable CIDs in the hot range
	curTs := s.curTs
	epoch := curTs.Height()
	coldEpoch := s.baseEpoch + build.Finality
	err = s.cs.WalkSnapshot(context.Background(), curTs, epoch-coldEpoch+1, false, false,
		func(cid cid.Cid) error {
			return hotSet.Mark(cid)
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	// Phase 1b: mark all reachable CIDs in the cold range
	coldTs, err := s.cs.GetTipsetByHeight(context.Background(), coldEpoch-1, curTs, true)
	if err != nil {
		// TODO do something better here
		panic(err)
	}

	err = s.cs.WalkSnapshot(context.Background(), coldTs, build.Finality, false, false,
		func(cid cid.Cid) error {
			return coldSet.Mark(cid)
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	log.Infow("marking done", "took", time.Since(startMark))

	// Phase 2: sweep cold objects:
	// - If a cold object is reachable in the hot range, it stays in the hotstore.
	// - If a cold object is reachable in the cold range, it is moved to the coldstore.
	// - If a cold object is unreachable, it is deleted.
	ch, err := s.snoop.Keys()
	if err != nil {
		// TODO do something better here
		panic(err)
	}

	startSweep := time.Now()
	log.Info("sweeping cold objects")

	// some stats for logging
	var stHot, stCold, stDead int

	for cid := range ch {
		wrEpoch, err := s.snoop.Get(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}

		// is the object stil hot?
		if wrEpoch >= coldEpoch {
			// yes, stay in the hotstore
			stHot++
			continue
		}

		// the object is cold -- check whether it is reachable in the hot range
		mark, err := hotSet.Has(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}

		if mark {
			// the object is reachable in the hot range, stay in the hotstore
			stHot++
			continue
		}

		// check whether it is reachable in the cold range
		mark, err = coldSet.Has(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}

		if mark {
			// the object is reachable in the cold range, move it to the cold store
			blk, err := s.hot.Get(cid)
			if err != nil {
				// TODO do something better here
				panic(err)
			}

			err = s.cold.Put(blk)
			if err != nil {
				// TODO do something better here
				panic(err)
			}

			stCold++
		} else {
			stDead++
		}

		// delete the object from the hotstore
		err = s.hot.DeleteBlock(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}

		// remove the snoop tracking
		err = s.snoop.Delete(cid)
		if err != nil {
			// TODO do something better here
			panic(err)
		}
	}

	log.Infow("sweeping done", "took", time.Since(startSweep))
	log.Infow("compaction stats", "hot", stHot, "cold", stCold, "dead", stDead)

	err = s.setBaseEpoch(coldEpoch)
	if err != nil {
		// TODO do something better here
		panic(err)
	}
}

func (s *SplitStore) setBaseEpoch(epoch abi.ChainEpoch) error {
	s.baseEpoch = epoch
	// write to datastore
	bs := make([]byte, 16)
	n := binary.PutUvarint(bs, uint64(epoch))
	bs = bs[:n]
	return s.ds.Put(baseEpochKey, bs)
}

func (s *SplitStore) newLiveSet() (LiveSet, error) {
	// TODO implementation
	return nil, errors.New("newLiveSet: IMPLEMENT ME!!!") //nolint
}
