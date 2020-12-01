package splitstore

import (
	"context"
	"encoding/binary"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bmatsuo/lmdb-go/lmdb"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	lmdbbs "github.com/filecoin-project/go-bs-lmdb"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
)

var CompactionThreshold = 5 * build.Finality

var baseEpochKey = dstore.NewKey("baseEpoch")

var log = logging.Logger("splitstore")

type SplitStore struct {
	compacting int32

	baseEpoch abi.ChainEpoch

	mx    sync.Mutex
	curTs *types.TipSet

	cs    *store.ChainStore
	ds    dstore.Datastore
	hot   bstore.Blockstore
	cold  bstore.Blockstore
	snoop TrackingStore

	env *lmdb.Env
}

var _ bstore.Blockstore = (*SplitStore)(nil)

// NewSplitStore creates a new SplitStore instance, given a path for the hotstore dbs and a cold
// blockstore. The SplitStore must be attached to the ChainStore with Start in order to trigger
// compaction.
func NewSplitStore(path string, ds dstore.Datastore, cold bstore.Blockstore) (*SplitStore, error) {
	// the hot store
	hot, err := lmdbbs.Open(filepath.Join(path, "hot.db"))
	if err != nil {
		return nil, err
	}

	// the tracking store
	snoop, err := NewTrackingStore(filepath.Join(path, "snoop.db"))
	if err != nil {
		hot.Close() //nolint:errcheck
		return nil, err
	}

	// the liveset env
	env, err := NewLiveSetEnv(filepath.Join(path, "sweep.db"))
	if err != nil {
		hot.Close()   //nolint:errcheck
		snoop.Close() //nolint:errcheck
		return nil, err
	}

	// and now we can make a SplitStore
	ss := &SplitStore{
		ds:    ds,
		hot:   hot,
		cold:  cold,
		snoop: snoop,
		env:   env,
	}

	return ss, nil
}

// Blockstore interface
func (s *SplitStore) DeleteBlock(cid cid.Cid) error {
	// afaict we don't seem to be using this method, so it's not implemented
	return errors.New("DeleteBlock not implemented on SplitStore; don't do this Luke!") //nolint
}

func (s *SplitStore) Has(cid cid.Cid) (bool, error) {
	has, err := s.hot.Has(cid)

	if err != nil || has {
		return has, err
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
	s.mx.Lock()
	if s.curTs == nil {
		s.mx.Unlock()
		return s.cold.Put(blk)
	}

	epoch := s.curTs.Height()
	s.mx.Unlock()

	err := s.snoop.Put(blk.Cid(), epoch)
	if err != nil && !lmdb.IsErrno(err, lmdb.KeyExist) {
		log.Errorf("error tracking CID in hotstore: %s; falling back to coldstore", err)
		return s.cold.Put(blk)
	}

	return s.hot.Put(blk)
}

func (s *SplitStore) PutMany(blks []blocks.Block) error {
	s.mx.Lock()
	if s.curTs == nil {
		s.mx.Unlock()
		return s.cold.PutMany(blks)
	}

	epoch := s.curTs.Height()
	s.mx.Unlock()

	batch := make([]cid.Cid, 0, len(blks))
	for _, blk := range blks {
		batch = append(batch, blk.Cid())
	}

	err := s.snoop.PutBatch(batch, epoch)
	if err != nil {
		if lmdb.IsErrno(err, lmdb.KeyExist) {
			// a write is duplicate, but we don't know which; write each block separately
			for _, blk := range blks {
				err = s.Put(blk)
				if err != nil {
					return err
				}
			}
			return nil
		}

		log.Errorf("error tracking CIDs in hotstore: %s; falling back to coldstore", err)
		return s.cold.PutMany(blks)
	}

	return s.hot.PutMany(blks)
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
		s.baseEpoch = bytesToEpoch(bs)

	case dstore.ErrNotFound:
		if s.curTs == nil {
			// this can happen in some tests
			break
		}

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

func (s *SplitStore) Close() error {
	if atomic.LoadInt32(&s.compacting) == 1 {
		log.Warn("ongoing compaction; waiting for it to finish...")
		for atomic.LoadInt32(&s.compacting) == 1 {
			time.Sleep(time.Second)
		}
	}

	return s.env.Close()
}

func (s *SplitStore) HeadChange(revert, apply []*types.TipSet) error {
	s.mx.Lock()
	s.curTs = apply[len(apply)-1]
	epoch := s.curTs.Height()
	s.mx.Unlock()

	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		// we are currently compacting, do nothing and wait for the next head change
		return nil
	}

	if epoch-s.baseEpoch > CompactionThreshold {
		go func() {
			defer atomic.StoreInt32(&s.compacting, 0)

			log.Info("compacting splitstore")
			start := time.Now()

			s.compact()

			log.Infow("compaction done", "took", time.Since(start))
		}()
	} else {
		// no compaction necessary
		atomic.StoreInt32(&s.compacting, 0)
	}

	return nil
}

// Compaction/GC Algorithm
func (s *SplitStore) compact() {
	// create two on disk live sets, one for marking the cold finality region
	// and one for marking the hot region
	hotSet, err := NewLiveSet(s.env, "hot")
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer hotSet.Close() //nolint:errcheck

	coldSet, err := NewLiveSet(s.env, "cold")
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer coldSet.Close() //nolint:errcheck

	// Phase 1: marking
	log.Info("marking live objects")
	startMark := time.Now()

	// Phase 1a: mark all reachable CIDs in the hot range
	s.mx.Lock()
	curTs := s.curTs
	s.mx.Unlock()

	epoch := curTs.Height()
	coldEpoch := s.baseEpoch + build.Finality
	err = s.cs.WalkSnapshot(context.Background(), curTs, epoch-coldEpoch, false, false,
		func(cid cid.Cid) error {
			return hotSet.Mark(cid)
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	// Phase 1b: mark all reachable CIDs in the cold range
	coldTs, err := s.cs.GetTipsetByHeight(context.Background(), coldEpoch, curTs, true)
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
	ch, err := s.snoop.Keys(context.Background())
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
		if wrEpoch > coldEpoch {
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
	return s.ds.Put(baseEpochKey, epochToBytes(epoch))
}

func epochToBytes(epoch abi.ChainEpoch) []byte {
	buf := make([]byte, 16)
	n := binary.PutUvarint(buf, uint64(epoch))
	return buf[:n]
}

func bytesToEpoch(buf []byte) abi.ChainEpoch {
	epoch, n := binary.Uvarint(buf)
	if n < 0 {
		panic("bogus base epoch bytes")
	}
	return abi.ChainEpoch(epoch)
}
