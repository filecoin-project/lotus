package splitstore

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-state-types/abi"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
)

// these are variable so that 1) lotus-soup builds 2) we can change them in tests
var (
	CompactionThreshold = 5 * build.Finality
	CompactionCold      = build.Finality
)

var (
	baseEpochKey   = dstore.NewKey("/splitstore/baseEpoch")
	warmupEpochKey = dstore.NewKey("/splitstore/warmupEpoch")
	log            = logging.Logger("splitstore")
)

const batchSize = 16384

func init() {
	// TODO temporary for debugging purposes; to be removed for merge.
	_ = logging.SetLogLevel("splitstore", "DEBUG")
}

type Config struct {
	// TrackingStore type; bolt (default) or lmdb
	TrackingStoreType string
	// LiveSet type; bloom (default), bolt, or lmdb
	LiveSetType string
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

// ChainAccessor allows the Splitstore to access the chain. It will most likely
// be a ChainStore at runtime.
type ChainAccessor interface {
	GetTipsetByHeight(context.Context, abi.ChainEpoch, *types.TipSet, bool) (*types.TipSet, error)
	GetHeaviestTipSet() *types.TipSet
	SubscribeHeadChanges(change func(revert []*types.TipSet, apply []*types.TipSet) error)
	WalkSnapshot(context.Context, *types.TipSet, abi.ChainEpoch, bool, bool, func(cid.Cid) error) error
}

type SplitStore struct {
	compacting int32

	fullCompaction  bool
	enableGC        bool
	skipOldMsgs     bool
	skipMsgReceipts bool

	baseEpoch   abi.ChainEpoch
	warmupEpoch abi.ChainEpoch

	mx    sync.Mutex
	curTs *types.TipSet

	chain ChainAccessor
	ds    dstore.Datastore
	hot   bstore.Blockstore
	cold  bstore.Blockstore
	snoop TrackingStore

	env LiveSetEnv

	liveSetSize int64
}

var _ bstore.Blockstore = (*SplitStore)(nil)

// Open opens an existing splistore, or creates a new splitstore. The splitstore
// is backed by the provided hot and cold stores. The returned SplitStore MUST be
// attached to the ChainStore with Start in order to trigger compaction.
func Open(path string, ds dstore.Datastore, hot, cold bstore.Blockstore, cfg *Config) (*SplitStore, error) {
	// the tracking store
	snoop, err := OpenTrackingStore(path, cfg.TrackingStoreType)
	if err != nil {
		return nil, err
	}

	// the liveset env
	env, err := OpenLiveSetEnv(path, cfg.LiveSetType)
	if err != nil {
		_ = snoop.Close()
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
func (s *SplitStore) DeleteBlock(_ cid.Cid) error {
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
	if err != nil {
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
func (s *SplitStore) Start(chain ChainAccessor) error {
	s.chain = chain
	s.curTs = chain.GetHeaviestTipSet()

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
			return xerrors.Errorf("error saving base epoch: %w", err)
		}

	default:
		return xerrors.Errorf("error loading base epoch: %w", err)
	}

	// load warmup epoch from metadata ds
	// if none, then the splitstore will warm up the hotstore at first head change notif
	// by walking the current tipset
	bs, err = s.ds.Get(warmupEpochKey)
	switch err {
	case nil:
		s.warmupEpoch = bytesToEpoch(bs)

	case dstore.ErrNotFound:

	default:
		return xerrors.Errorf("error loading warmup epoch: %w", err)
	}

	// watch the chain
	chain.SubscribeHeadChanges(s.HeadChange)

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

func (s *SplitStore) HeadChange(_, apply []*types.TipSet) error {
	s.mx.Lock()
	curTs := apply[len(apply)-1]
	epoch := curTs.Height()
	s.curTs = curTs
	s.mx.Unlock()

	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		// we are currently compacting, do nothing and wait for the next head change
		return nil
	}

	if s.warmupEpoch == 0 {
		// splitstore needs to warm up
		go func() {
			defer atomic.StoreInt32(&s.compacting, 0)

			log.Info("warming up hotstore")
			start := time.Now()

			s.warmup(curTs)

			log.Infow("warm up done", "took", time.Since(start))
		}()

		return nil
	}

	if epoch-s.baseEpoch > CompactionThreshold {
		// it's time to compact
		go func() {
			defer atomic.StoreInt32(&s.compacting, 0)

			log.Info("compacting splitstore")
			start := time.Now()

			s.compact(curTs)

			log.Infow("compaction done", "took", time.Since(start))
		}()
	} else {
		// no compaction necessary
		atomic.StoreInt32(&s.compacting, 0)
	}

	return nil
}

func (s *SplitStore) warmup(curTs *types.TipSet) {
	epoch := curTs.Height()

	batchHot := make([]blocks.Block, 0, batchSize)
	batchSnoop := make([]cid.Cid, 0, batchSize)

	count := int64(0)
	err := s.chain.WalkSnapshot(context.Background(), curTs, 1, s.skipOldMsgs, s.skipMsgReceipts,
		func(cid cid.Cid) error {
			count++

			has, err := s.hot.Has(cid)
			if err != nil {
				return err
			}

			if has {
				return nil
			}

			blk, err := s.cold.Get(cid)
			if err != nil {
				return err
			}

			batchHot = append(batchHot, blk)
			batchSnoop = append(batchSnoop, cid)

			if len(batchHot) == batchSize {
				err = s.snoop.PutBatch(batchSnoop, epoch)
				if err != nil {
					return err
				}
				batchSnoop = batchSnoop[:0]

				err = s.hot.PutMany(batchHot)
				if err != nil {
					return err
				}
				batchHot = batchHot[:0]
			}

			return nil
		})

	if err != nil {
		log.Errorf("error warming up splitstore: %s", err)
		return
	}

	if len(batchHot) > 0 {
		err = s.snoop.PutBatch(batchSnoop, epoch)
		if err != nil {
			log.Errorf("error warming up splitstore: %s", err)
			return
		}

		err = s.hot.PutMany(batchHot)
		if err != nil {
			log.Errorf("error warming up splitstore: %s", err)
			return
		}
	}

	if count > s.liveSetSize {
		s.liveSetSize = count
	}

	// save the warmup epoch
	s.warmupEpoch = epoch
	err = s.ds.Put(warmupEpochKey, epochToBytes(epoch))
	if err != nil {
		log.Errorf("error saving warmup epoch: %s", err)
	}
}

// Compaction/GC Algorithm
func (s *SplitStore) compact(curTs *types.TipSet) {
	if s.liveSetSize == 0 {
		start := time.Now()
		log.Info("estimating live set size")
		s.estimateLiveSetSize(curTs)
		log.Infow("estimating live set size done", "took", time.Since(start), "size", s.liveSetSize)
	} else {
		log.Infow("current live set size estimate", "size", s.liveSetSize)
	}

	if s.fullCompaction {
		s.compactFull(curTs)
	} else {
		s.compactSimple(curTs)
	}
}

func (s *SplitStore) estimateLiveSetSize(curTs *types.TipSet) {
	s.liveSetSize = 0
	err := s.chain.WalkSnapshot(context.Background(), curTs, 1, s.skipOldMsgs, s.skipMsgReceipts,
		func(cid cid.Cid) error {
			s.liveSetSize++
			return nil
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}
}

func (s *SplitStore) compactSimple(curTs *types.TipSet) {
	coldEpoch := s.baseEpoch + CompactionCold

	log.Infow("running simple compaction", "currentEpoch", curTs.Height(), "baseEpoch", s.baseEpoch, "coldEpoch", coldEpoch)

	coldSet, err := s.env.NewLiveSet("cold", s.liveSetSize)
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer coldSet.Close() //nolint:errcheck

	// 1. mark reachable cold objects by looking at the objects reachable only from the cold epoch
	log.Info("marking reachable cold objects")
	startMark := time.Now()

	coldTs, err := s.chain.GetTipsetByHeight(context.Background(), coldEpoch, curTs, true)
	if err != nil {
		// TODO do something better here
		panic(err)
	}

	count := int64(0)
	err = s.chain.WalkSnapshot(context.Background(), coldTs, 1, s.skipOldMsgs, s.skipMsgReceipts,
		func(cid cid.Cid) error {
			count++
			return coldSet.Mark(cid)
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	if count > s.liveSetSize {
		s.liveSetSize = count
	}

	log.Infow("marking done", "took", time.Since(startMark))

	// 2. move cold unreachable objects to the coldstore
	log.Info("collecting cold objects")
	startCollect := time.Now()

	cold := make(map[cid.Cid]struct{})

	// some stats for logging
	var stHot, stCold int

	// 2.1 iterate through the snoop and collect unreachable cold objects
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

	// 2.2 copy the cold objects to the coldstore
	log.Info("moving cold objects to the coldstore")
	startMove := time.Now()

	batch := make([]blocks.Block, 0, batchSize)

	for cid := range cold {
		blk, err := s.hot.Get(cid)
		if err != nil {
			if err == dstore.ErrNotFound {
				// this can happen if the node is killed after we have deleted the block from the hotstore
				// but before we have deleted it from the snoop; just delete the snoop.
				err = s.snoop.Delete(cid)
				if err != nil {
					log.Errorf("error deleting cid %s from snoop: %s", cid, err)
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

		batch = append(batch, blk)
		if len(batch) == batchSize {
			err = s.cold.PutMany(batch)
			if err != nil {
				log.Errorf("error putting cold batch to coldstore: %s", err)
				// TODO do something better here -- just continue?
				panic(err)
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		err = s.cold.PutMany(batch)
		if err != nil {
			log.Errorf("error putting cold batch to coldstore: %s", err)
			// TODO do something better here -- just continue?
			panic(err)
		}
	}
	log.Infow("moving done", "took", time.Since(startMove))

	// 2.3 delete cold objects from the hotstore
	// TODO we really want batching for this!
	log.Info("purging cold objects from the hotstore")
	purgeStart := time.Now()
	for cid := range cold {
		// delete the object from the hotstore
		err = s.hot.DeleteBlock(cid)
		if err != nil {
			log.Errorf("error deleting block %s from hotstore: %s", cid, err)
			// TODO do something better here -- just continue?
			panic(err)
		}
	}
	log.Infow("purging cold from hotstore done", "took", time.Since(purgeStart))

	// 2.4 remove the snoop tracking for cold objects
	purgeStart = time.Now()
	log.Info("purging cold objects from snoop")

	err = s.snoop.DeleteBatch(cold)
	if err != nil {
		log.Errorf("error purging cold objects from snoop: %s", err)
		// TODO do something better here -- just continue?
		panic(err)
	}
	log.Infow("purging cold from snoop done", "took", time.Since(purgeStart))

	// we are done; do some housekeeping
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

func (s *SplitStore) compactFull(curTs *types.TipSet) {
	epoch := curTs.Height()
	coldEpoch := s.baseEpoch + CompactionCold

	log.Infow("running full compaction", "currentEpoch", curTs.Height(), "baseEpoch", s.baseEpoch, "coldEpoch", coldEpoch)

	// create two live sets, one for marking the cold finality region
	// and one for marking the hot region
	hotSet, err := s.env.NewLiveSet("hot", s.liveSetSize)
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer hotSet.Close() //nolint:errcheck

	coldSet, err := s.env.NewLiveSet("cold", s.liveSetSize)
	if err != nil {
		// TODO do something better here
		panic(err)
	}
	defer coldSet.Close() //nolint:errcheck

	// Phase 1: marking
	log.Info("marking live objects")
	startMark := time.Now()

	// Phase 1a: mark all reachable CIDs in the hot range
	count := int64(0)
	err = s.chain.WalkSnapshot(context.Background(), curTs, epoch-coldEpoch, s.skipOldMsgs, s.skipMsgReceipts,
		func(cid cid.Cid) error {
			count++
			return hotSet.Mark(cid)
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	if count > s.liveSetSize {
		s.liveSetSize = count
	}

	// Phase 1b: mark all reachable CIDs in the cold range
	coldTs, err := s.chain.GetTipsetByHeight(context.Background(), coldEpoch, curTs, true)
	if err != nil {
		// TODO do something better here
		panic(err)
	}

	count = 0
	err = s.chain.WalkSnapshot(context.Background(), coldTs, CompactionCold, s.skipOldMsgs, s.skipMsgReceipts,
		func(cid cid.Cid) error {
			count++
			return coldSet.Mark(cid)
		})

	if err != nil {
		// TODO do something better here
		panic(err)
	}

	if count > s.liveSetSize {
		s.liveSetSize = count
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

	// 2.1 iterate through the snoop and collect cold and dead objects
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

	// 2.2 copy the cold objects to the coldstore
	log.Info("moving cold objects to the coldstore")
	startMove := time.Now()

	batch := make([]blocks.Block, 0, batchSize)
	for cid := range cold {
		blk, err := s.hot.Get(cid)
		if err != nil {
			if err == dstore.ErrNotFound {
				// this can happen if the node is killed after we have deleted the block from the hotstore
				// but before we have deleted it from the snoop; just delete the snoop.
				err = s.snoop.Delete(cid)
				if err != nil {
					log.Errorf("error deleting cid %s from snoop: %s", cid, err)
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

		batch = append(batch, blk)
		if len(batch) == batchSize {
			err = s.cold.PutMany(batch)
			if err != nil {
				log.Errorf("error putting cold batch to coldstore: %s", err)
				// TODO do something better here -- just continue?
				panic(err)
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		err = s.cold.PutMany(batch)
		if err != nil {
			log.Errorf("error putting cold batch to coldstore: %s", err)
			// TODO do something better here -- just continue?
			panic(err)
		}
	}
	log.Infow("moving done", "took", time.Since(startMove))

	// 2.3 delete cold objects from the hotstore
	// TODO we really want batching for this!
	log.Info("purging cold objects from the hotstore")
	purgeStart := time.Now()
	for cid := range cold {
		// delete the object from the hotstore
		err = s.hot.DeleteBlock(cid)
		if err != nil {
			log.Errorf("error deleting block %s from hotstore: %s", cid, err)
			// TODO do something better here -- just continue?
			panic(err)
		}
	}
	log.Infow("purging cold from hotstore done", "took", time.Since(purgeStart))

	// 2.4 remove the snoop tracking for cold objects
	purgeStart = time.Now()
	log.Info("purging cold objects from snoop")

	err = s.snoop.DeleteBatch(cold)
	if err != nil {
		log.Errorf("error purging cold objects from snoop: %s", err)
		// TODO do something better here -- just continue?
		panic(err)
	}
	log.Infow("purging cold from snoop done", "took", time.Since(purgeStart))

	// 3. if we have dead objects, delete them from the hotstore and remove the tracking
	if len(dead) > 0 {
		log.Info("deleting dead objects")

		purgeStart = time.Now()
		log.Info("purging dead objects from the hotstore")
		// TODO we really want batching for this!
		for cid := range dead {
			// delete the object from the hotstore
			err = s.hot.DeleteBlock(cid)
			if err != nil {
				log.Errorf("error deleting block %s from hotstore: %s", cid, err)
				// TODO do something better here -- just continue?
				panic(err)
			}
		}
		log.Infow("purging dead from hotstore done", "took", time.Since(purgeStart))

		// remove the snoop tracking
		purgeStart := time.Now()
		log.Info("purging dead objects from snoop")

		err = s.snoop.DeleteBatch(dead)
		if err != nil {
			log.Errorf("error purging dead objects from snoop: %s", err)
			// TODO do something better here -- just continue?
			panic(err)
		}

		log.Infow("purging dead from snoop done", "took", time.Since(purgeStart))
	}

	log.Infow("sweeping done", "took", time.Since(startSweep))

	// we are done; do some housekeeping
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
