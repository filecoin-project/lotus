package splitstore

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"github.com/ledgerwatch/lmdb-go/lmdb"

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

const (
	CompactionThreshold = 5 * build.Finality
	CompactionCold      = build.Finality
)

var baseEpochKey = dstore.NewKey("baseEpoch")

var log = logging.Logger("splitstore")

func init() {
	// TODO temporary for debugging purposes; to be removed for merge.
	logging.SetLogLevel("splitstore", "DEBUG")
}

type Config struct {
	// use LMDB for tracking store and liveset instead of BoltDB
	UseLMDB bool
	// perform full reachability analysis (expensive) for compaction
	// You should enable this option if you plan to use the splitstore without a backing coldstore
	EnableFullCompaction bool
	// EXPERIMENTAL enable pruning of unreachable objects.
	// This has not been sufficiently tested yet; only enable if you know what you are doing.
	// Only applies if you enable full compaction.
	EnableGC bool
	// full archival nodes should enable this if EnableFullCompaction is enabled
	// do NOT enable this if you synced from a snapshot.
	// Only applies if you enabled full compaction
	Archival bool
}

type SplitStore struct {
	compacting int32

	fullCompaction  bool
	enableGC        bool
	skipOldMsgs     bool
	skipMsgReceipts bool

	baseEpoch abi.ChainEpoch

	mx    sync.Mutex
	curTs *types.TipSet

	cs    *store.ChainStore
	ds    dstore.Datastore
	hot   bstore.Blockstore
	cold  bstore.Blockstore
	snoop TrackingStore

	env LiveSetEnv
}

var _ bstore.Blockstore = (*SplitStore)(nil)

// NewSplitStore creates a new SplitStore instance, given a path for the hotstore dbs and a cold
// blockstore. The SplitStore must be attached to the ChainStore with Start in order to trigger
// compaction.
func NewSplitStore(path string, ds dstore.Datastore, cold, hot bstore.Blockstore, cfg *Config) (*SplitStore, error) {
	// the tracking store
	snoop, err := NewTrackingStore(path, cfg.UseLMDB)
	if err != nil {
		return nil, err
	}

	// the liveset env
	env, err := NewLiveSetEnv(path, cfg.UseLMDB)
	if err != nil {
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

		fullCompaction:  cfg.EnableFullCompaction,
		enableGC:        cfg.EnableGC,
		skipOldMsgs:     !(cfg.EnableFullCompaction && cfg.Archival),
		skipMsgReceipts: !(cfg.EnableFullCompaction && cfg.Archival),
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
	if s.fullCompaction {
		s.compactFull()
	} else {
		s.compactSimple()
	}
}

func (s *SplitStore) compactSimple() {
	s.mx.Lock()
	curTs := s.curTs
	s.mx.Unlock()

	coldEpoch := s.baseEpoch + CompactionCold

	log.Infow("running simple compaction", "currentEpoch", curTs.Height(), "baseEpoch", s.baseEpoch, "coldEpoch", coldEpoch)

	coldSet, err := s.env.NewLiveSet("cold")
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer coldSet.Close() //nolint:errcheck

	// 1. mark reachable cold objects by looking at the objects reachable only from the cold epoch
	log.Info("marking reachable cold objects")
	startMark := time.Now()

	coldTs, err := s.cs.GetTipsetByHeight(context.Background(), coldEpoch, curTs, true)
	if err != nil {
		// TODO do something better here
		panic(err)
	}

	err = s.cs.WalkSnapshot(context.Background(), coldTs, 1, s.skipOldMsgs, s.skipMsgReceipts,
		func(cid cid.Cid) error {
			return coldSet.Mark(cid)
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	log.Infow("marking done", "took", time.Since(startMark))

	// 2. move cold unreachable objects to the coldstore
	log.Info("collecting cold objects")
	startCollect := time.Now()

	cold := make(map[cid.Cid]struct{})

	// some stats for logging
	var stHot, stCold int

	err = s.snoop.ForEach(func(cid cid.Cid, wrEpoch abi.ChainEpoch) error {
		// is the object stil hot?
		if wrEpoch > coldEpoch {
			// yes, stay in the hotstore
			stHot++
			return nil
		}

		// check whether it is reachable in the cold boundary
		mark, err := coldSet.Has(cid)
		if err != nil {
			return xerrors.Errorf("error checkiing cold set for %s: %w", cid, err)
		}

		if mark {
			stHot++
			return nil
		}

		// it's cold, mark it for move
		cold[cid] = struct{}{}
		stCold++
		return nil
	})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	log.Infow("collection done", "took", time.Since(startCollect))
	log.Infow("compaction stats", "hot", stHot, "cold", stCold)

	log.Info("moving cold objects to the coldstore")
	startMove := time.Now()
	for cid := range cold {
		blk, err := s.hot.Get(cid)
		if err != nil {
			if err == dstore.ErrNotFound {
				// this can happen if the node is killed after we have deleted the block from the hotstore
				// but before we have deleted it from the snoop; just delete the snoop.
				err = s.snoop.Delete(cid)
				if err != nil {
					log.Errorf("error deleting cid %s from tracking store: %s", cid, err)
					// TODO do something better here -- just continue?
					panic(err)
				}
			} else {
				log.Errorf("error retrieving tracked block %s from hotstore: %s", cid, err)
				// TODO do something better here -- just continue?
				panic(err)
			}

			continue
		}

		// put the object in the coldstore
		err = s.cold.Put(blk)
		if err != nil {
			log.Errorf("error puting block %s to coldstore: %s", cid, err)
			// TODO do something better here -- just continue?
			panic(err)
		}

		// delete the object from the hotstore
		err = s.hot.DeleteBlock(cid)
		if err != nil {
			log.Errorf("error deleting block %s from hotstore: %s", cid, err)
			// TODO do something better here -- just continue?
			panic(err)
		}
	}
	log.Infow("moving done", "took", time.Since(startMove))

	// remove the snoop tracking
	purgeStart := time.Now()
	log.Info("purging cold objects from tracking store")

	err = s.snoop.DeleteBatch(cold)
	if err != nil {
		log.Errorf("error purging cold objects from tracking store: %s", err)
		// TODO do something better here -- just continue?
		panic(err)
	}
	log.Infow("purging done", "took", time.Since(purgeStart))

	err = s.snoop.Sync()
	if err != nil {
		// TODO do something better here
		panic(err)
	}

	err = s.setBaseEpoch(coldEpoch)
	if err != nil {
		// TODO do something better here
		panic(err)
	}
}

func (s *SplitStore) compactFull() {
	s.mx.Lock()
	curTs := s.curTs
	s.mx.Unlock()

	epoch := curTs.Height()
	coldEpoch := s.baseEpoch + CompactionCold

	log.Infow("running full compaction", "currentEpoch", curTs.Height(), "baseEpoch", s.baseEpoch, "coldEpoch", coldEpoch)

	// create two live sets, one for marking the cold finality region
	// and one for marking the hot region
	hotSet, err := s.env.NewLiveSet("hot")
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer hotSet.Close() //nolint:errcheck

	coldSet, err := s.env.NewLiveSet("cold")
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer coldSet.Close() //nolint:errcheck

	// Phase 1: marking
	log.Info("marking live objects")
	startMark := time.Now()

	// Phase 1a: mark all reachable CIDs in the hot range
	err = s.cs.WalkSnapshot(context.Background(), curTs, epoch-coldEpoch, s.skipOldMsgs, s.skipMsgReceipts,
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

	err = s.cs.WalkSnapshot(context.Background(), coldTs, CompactionCold, s.skipOldMsgs, s.skipMsgReceipts,
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
	// - If a cold object is unreachable, it is deleted if GC is enabled, otherwise moved to the coldstore.
	startSweep := time.Now()
	log.Info("sweeping cold objects")

	// some stats for logging
	var stHot, stCold, stDead int

	cold := make(map[cid.Cid]struct{})
	dead := make(map[cid.Cid]struct{})

	err = s.snoop.ForEach(func(cid cid.Cid, wrEpoch abi.ChainEpoch) error {
		// is the object stil hot?
		if wrEpoch > coldEpoch {
			// yes, stay in the hotstore
			stHot++
			return nil
		}

		// the object is cold -- check whether it is reachable in the hot range
		mark, err := hotSet.Has(cid)
		if err != nil {
			return xerrors.Errorf("error checking live mark for %s: %w", cid, err)
		}

		if mark {
			// the object is reachable in the hot range, stay in the hotstore
			stHot++
			return nil
		}

		// check whether it is reachable in the cold range
		mark, err = coldSet.Has(cid)
		if err != nil {
			return xerrors.Errorf("error checkiing cold set for %s: %w", cid, err)
		}

		if s.enableGC {
			if mark {
				// the object is reachable in the cold range, move it to the cold store
				cold[cid] = struct{}{}
				stCold++
			} else {
				// the object is dead and will be deleted
				dead[cid] = struct{}{}
				stDead++
			}
		} else {
			// if GC is disabled, we move both cold and dead objects to the coldstore
			cold[cid] = struct{}{}
			if mark {
				stCold++
			} else {
				stDead++
			}
		}

		return nil
	})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	log.Infow("compaction stats", "hot", stHot, "cold", stCold, "dead", stDead)

	log.Info("moving cold objects to the coldstore")
	for cid := range cold {
		blk, err := s.hot.Get(cid)
		if err != nil {
			if err == dstore.ErrNotFound {
				// this can happen if the node is killed after we have deleted the block from the hotstore
				// but before we have deleted it from the snoop; just delete the snoop.
				err = s.snoop.Delete(cid)
				if err != nil {
					log.Errorf("error deleting cid %s from tracking store: %s", cid, err)
					// TODO do something better here -- just continue?
					panic(err)
				}
			} else {
				log.Errorf("error retrieving tracked block %s from hotstore: %s", cid, err)
				// TODO do something better here -- just continue?
				panic(err)
			}

			continue
		}

		// put the object in the coldstore
		err = s.cold.Put(blk)
		if err != nil {
			log.Errorf("error puting block %s to coldstore: %s", cid, err)
			// TODO do something better here -- just continue?
			panic(err)
		}

		// delete the object from the hotstore
		err = s.hot.DeleteBlock(cid)
		if err != nil {
			log.Errorf("error deleting block %s from hotstore: %s", cid, err)
			// TODO do something better here -- just continue?
			panic(err)
		}
	}

	// remove the snoop tracking
	purgeStart := time.Now()
	log.Info("purging cold objects from tracking store")

	err = s.snoop.DeleteBatch(cold)
	if err != nil {
		log.Errorf("error purging cold objects from tracking store: %s", err)
		// TODO do something better here -- just continue?
		panic(err)
	}

	log.Infow("purging done", "took", time.Since(purgeStart))

	if len(dead) > 0 {
		log.Info("deleting dead objects")

		for cid := range dead {
			// delete the object from the hotstore
			err = s.hot.DeleteBlock(cid)
			if err != nil {
				log.Errorf("error deleting block %s from hotstore: %s", cid, err)
				// TODO do something better here -- just continue?
				panic(err)
			}
		}

		// remove the snoop tracking
		purgeStart := time.Now()
		log.Info("purging dead objects from tracking store")

		err = s.snoop.DeleteBatch(dead)
		if err != nil {
			log.Errorf("error purging dead objects from tracking store: %s", err)
			// TODO do something better here -- just continue?
			panic(err)
		}

		log.Infow("purging done", "took", time.Since(purgeStart))
	}

	log.Infow("sweeping done", "took", time.Since(startSweep))

	err = s.snoop.Sync()
	if err != nil {
		// TODO do something better here
		panic(err)
	}

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
