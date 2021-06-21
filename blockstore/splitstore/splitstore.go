package splitstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"go.opencensus.io/stats"
)

var (
	// CompactionThreshold is the number of epochs that need to have elapsed
	// from the previously compacted epoch to trigger a new compaction.
	//
	//        |················· CompactionThreshold ··················|
	//        |                                                        |
	// =======‖≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡‖----------|------------------------»
	//        |                    |          |   chain -->             ↑__ current epoch
	//        | archived epochs ___↑          |
	//                             |          ↑________ CompactionBoundary
	//                             ↑__ CompactionSlack
	//
	// === :: cold (already archived)
	// ≡≡≡ :: to be archived in this compaction
	// --- :: hot
	CompactionThreshold = 7 * build.Finality

	// CompactionBoundary is the number of epochs from the current epoch at which
	// we will walk the chain for live objects.
	CompactionBoundary = 4 * build.Finality

	// CompactionSlack is the number of epochs from the compaction boundary to the beginning
	// of the cold epoch.
	CompactionSlack = 2 * build.Finality

	// SyncGapTime is the time delay from a tipset's min timestamp before we decide
	// there is a sync gap
	SyncGapTime = time.Minute
)

var (
	// baseEpochKey stores the base epoch (last compaction epoch) in the
	// metadata store.
	baseEpochKey = dstore.NewKey("/splitstore/baseEpoch")

	// warmupEpochKey stores whether a hot store warmup has been performed.
	// On first start, the splitstore will walk the state tree and will copy
	// all active blocks into the hotstore.
	warmupEpochKey = dstore.NewKey("/splitstore/warmupEpoch")

	// syncGapEpochKey stores the last epoch where a sync gap was detected.
	// If there is a sync gap after the boundary epoch, compaction will perform
	// a slower full walk from the current epoch to the boundary epoch
	syncGapEpochKey = dstore.NewKey("/splitstore/syncGapEpoch")

	// markSetSizeKey stores the current estimate for the mark set size.
	// this is first computed at warmup and updated in every compaction
	markSetSizeKey = dstore.NewKey("/splitstore/markSetSize")

	log = logging.Logger("splitstore")
)

const (
	batchSize = 16384

	defaultColdPurgeSize = 7_000_000
)

type Config struct {
	// TrackingStore is the type of tracking store to use.
	//
	// Supported values are: "bolt" (default if omitted), "mem" (for tests and readonly access).
	TrackingStoreType string

	// MarkSetType is the type of mark set to use.
	//
	// Supported values are: "bloom" (default if omitted), "bolt".
	MarkSetType string

	// HotHeaders indicates whether to keep chain block headers in hotstore or not.
	// This is necessary, and automatically set by DI in lotus node construction, if
	// you are running with a noop coldstore.
	HotHeaders bool
}

// ChainAccessor allows the Splitstore to access the chain. It will most likely
// be a ChainStore at runtime.
type ChainAccessor interface {
	GetGenesis() (*types.BlockHeader, error)
	GetTipsetByHeight(context.Context, abi.ChainEpoch, *types.TipSet, bool) (*types.TipSet, error)
	GetHeaviestTipSet() *types.TipSet
	SubscribeHeadChanges(change func(revert []*types.TipSet, apply []*types.TipSet) error)
}

type SplitStore struct {
	compacting  int32 // compaction (or warmp up) in progress
	critsection int32 // compaction critical section
	closing     int32 // the split store is closing

	cfg *Config

	baseEpoch    abi.ChainEpoch
	syncGapEpoch abi.ChainEpoch
	warmupEpoch  abi.ChainEpoch
	writeEpoch   abi.ChainEpoch

	coldPurgeSize int

	mx    sync.Mutex
	curTs *types.TipSet

	chain   ChainAccessor
	ds      dstore.Datastore
	hot     bstore.Blockstore
	cold    bstore.Blockstore
	tracker TrackingStore

	env MarkSetEnv

	markSetSize int64

	ctx    context.Context
	cancel func()
}

var _ bstore.Blockstore = (*SplitStore)(nil)

// Open opens an existing splistore, or creates a new splitstore. The splitstore
// is backed by the provided hot and cold stores. The returned SplitStore MUST be
// attached to the ChainStore with Start in order to trigger compaction.
func Open(path string, ds dstore.Datastore, hot, cold bstore.Blockstore, cfg *Config) (*SplitStore, error) {
	// the tracking store
	tracker, err := OpenTrackingStore(path, cfg.TrackingStoreType)
	if err != nil {
		return nil, err
	}

	// the markset env
	env, err := OpenMarkSetEnv(path, cfg.MarkSetType)
	if err != nil {
		_ = tracker.Close()
		return nil, err
	}

	// and now we can make a SplitStore
	ss := &SplitStore{
		cfg:     cfg,
		ds:      ds,
		hot:     hot,
		cold:    cold,
		tracker: tracker,
		env:     env,

		coldPurgeSize: defaultColdPurgeSize,
	}

	ss.ctx, ss.cancel = context.WithCancel(context.Background())

	return ss, nil
}

// Blockstore interface
func (s *SplitStore) DeleteBlock(_ cid.Cid) error {
	// afaict we don't seem to be using this method, so it's not implemented
	return errors.New("DeleteBlock not implemented on SplitStore; don't do this Luke!") //nolint
}

func (s *SplitStore) DeleteMany(_ []cid.Cid) error {
	// afaict we don't seem to be using this method, so it's not implemented
	return errors.New("DeleteMany not implemented on SplitStore; don't do this Luke!") //nolint
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
		blk, err = s.cold.Get(cid)
		if err == nil {
			stats.Record(context.Background(), metrics.SplitstoreMiss.M(1))
		}
		return blk, err

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
		size, err = s.cold.GetSize(cid)
		if err == nil {
			stats.Record(context.Background(), metrics.SplitstoreMiss.M(1))
		}
		return size, err

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

	epoch := s.writeEpoch
	s.mx.Unlock()

	err := s.tracker.Put(blk.Cid(), epoch)
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

	epoch := s.writeEpoch
	s.mx.Unlock()

	batch := make([]cid.Cid, 0, len(blks))
	for _, blk := range blks {
		batch = append(batch, blk.Cid())
	}

	err := s.tracker.PutBatch(batch, epoch)
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
		err = s.cold.View(cid, cb)
		if err == nil {
			stats.Record(context.Background(), metrics.SplitstoreMiss.M(1))
		}
		return err

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
		// the hotstore hasn't warmed up, load the genesis into the hotstore
		err = s.warmup(s.curTs)
		if err != nil {
			return xerrors.Errorf("error warming up: %w", err)
		}

	default:
		return xerrors.Errorf("error loading warmup epoch: %w", err)
	}

	// load sync gap epoch from metadata ds
	bs, err = s.ds.Get(syncGapEpochKey)
	switch err {
	case nil:
		s.syncGapEpoch = bytesToEpoch(bs)

	case dstore.ErrNotFound:
	default:
		return xerrors.Errorf("error loading sync gap epoch: %w", err)
	}

	// load markSetSize from metadata ds
	// if none, the splitstore will compute it during warmup and update in every compaction
	bs, err = s.ds.Get(markSetSizeKey)
	switch err {
	case nil:
		s.markSetSize = bytesToInt64(bs)

	case dstore.ErrNotFound:
	default:
		return xerrors.Errorf("error loading mark set size: %w", err)
	}

	s.updateWriteEpoch()

	log.Infow("starting splitstore", "baseEpoch", s.baseEpoch, "warmupEpoch", s.warmupEpoch, "writeEpoch", s.writeEpoch)

	go s.background()

	// watch the chain
	chain.SubscribeHeadChanges(s.HeadChange)

	return nil
}

func (s *SplitStore) Close() error {
	atomic.StoreInt32(&s.closing, 1)

	if atomic.LoadInt32(&s.critsection) == 1 {
		log.Warn("ongoing compaction in critical section; waiting for it to finish...")
		for atomic.LoadInt32(&s.critsection) == 1 {
			time.Sleep(time.Second)
		}
	}

	s.cancel()
	return multierr.Combine(s.tracker.Close(), s.env.Close())
}

func (s *SplitStore) HeadChange(_, apply []*types.TipSet) error {
	// Revert only.
	if len(apply) == 0 {
		return nil
	}

	s.mx.Lock()
	curTs := apply[len(apply)-1]
	epoch := curTs.Height()
	s.curTs = curTs
	s.mx.Unlock()

	s.updateWriteEpoch()

	timestamp := time.Unix(int64(curTs.MinTimestamp()), 0)
	if time.Since(timestamp) > SyncGapTime {
		err := s.setSyncGapEpoch(epoch)
		if err != nil {
			log.Warnf("error saving sync gap epoch: %s", err)
		}

		// don't attempt compaction before we have caught up syncing
		return nil
	}

	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		// we are currently compacting, do nothing and wait for the next head change
		return nil
	}

	if epoch-s.baseEpoch > CompactionThreshold {
		// it's time to compact
		go func() {
			defer atomic.StoreInt32(&s.compacting, 0)

			log.Info("compacting splitstore")
			start := time.Now()

			s.compact(curTs, s.syncGapEpoch)

			log.Infow("compaction done", "took", time.Since(start))
		}()
	} else {
		// no compaction necessary
		atomic.StoreInt32(&s.compacting, 0)
	}

	return nil
}

func (s *SplitStore) updateWriteEpoch() {
	s.mx.Lock()
	defer s.mx.Unlock()

	curTs := s.curTs
	timestamp := time.Unix(int64(curTs.MinTimestamp()), 0)

	dt := time.Since(timestamp)
	if dt < 0 {
		writeEpoch := curTs.Height() + 1
		if writeEpoch > s.writeEpoch {
			s.writeEpoch = writeEpoch
		}

		return
	}

	writeEpoch := curTs.Height() + abi.ChainEpoch(dt.Seconds())/builtin.EpochDurationSeconds + 1
	if writeEpoch > s.writeEpoch {
		s.writeEpoch = writeEpoch
	}
}

func (s *SplitStore) background() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-ticker.C:
			s.updateWriteEpoch()
		}
	}
}

func (s *SplitStore) warmup(curTs *types.TipSet) error {
	err := s.loadGenesisState()
	if err != nil {
		return xerrors.Errorf("error loading genesis state: %w", err)
	}

	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		return xerrors.Errorf("error locking compaction")
	}

	go func() {
		defer atomic.StoreInt32(&s.compacting, 0)

		log.Info("warming up hotstore")
		start := time.Now()

		err = s.doWarmup(curTs)
		if err != nil {
			log.Errorf("error warming up hotstore: %s", err)
			return
		}

		log.Infow("warm up done", "took", time.Since(start))
	}()

	return nil
}

func (s *SplitStore) loadGenesisState() error {
	// makes sure the genesis and its state root are hot
	gb, err := s.chain.GetGenesis()
	if err != nil {
		return xerrors.Errorf("error getting genesis: %w", err)
	}

	genesis := gb.Cid()
	genesisStateRoot := gb.ParentStateRoot

	has, err := s.hot.Has(genesis)
	if err != nil {
		return xerrors.Errorf("error checking hotstore for genesis: %w", err)
	}

	if !has {
		blk, err := gb.ToStorageBlock()
		if err != nil {
			return xerrors.Errorf("error converting genesis block to storage block: %w", err)
		}

		err = s.hot.Put(blk)
		if err != nil {
			return xerrors.Errorf("error putting genesis block to hotstore: %w", err)
		}
	}

	err = s.walkLinks(genesisStateRoot, cid.NewSet(), func(c cid.Cid) error {
		has, err = s.hot.Has(c)
		if err != nil {
			return xerrors.Errorf("error checking hotstore for genesis state root: %w", err)
		}

		if !has {
			blk, err := s.cold.Get(c)
			if err != nil {
				if err == bstore.ErrNotFound {
					return nil
				}

				return xerrors.Errorf("error retrieving genesis state linked object from coldstore: %w", err)
			}

			err = s.hot.Put(blk)
			if err != nil {
				return xerrors.Errorf("error putting genesis state linked object to hotstore: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error walking genesis state root links: %w", err)
	}

	return nil
}

func (s *SplitStore) doWarmup(curTs *types.TipSet) error {
	epoch := curTs.Height()

	batchHot := make([]blocks.Block, 0, batchSize)
	batchSnoop := make([]cid.Cid, 0, batchSize)

	count := int64(0)
	xcount := int64(0)
	missing := int64(0)
	err := s.walk(curTs, epoch, false,
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
				if err == bstore.ErrNotFound {
					missing++
					return nil
				}
				return err
			}

			xcount++

			batchHot = append(batchHot, blk)
			batchSnoop = append(batchSnoop, cid)

			if len(batchHot) == batchSize {
				err = s.tracker.PutBatch(batchSnoop, epoch)
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
		return err
	}

	if len(batchHot) > 0 {
		err = s.tracker.PutBatch(batchSnoop, epoch)
		if err != nil {
			return err
		}

		err = s.hot.PutMany(batchHot)
		if err != nil {
			return err
		}
	}

	log.Infow("warmup stats", "visited", count, "warm", xcount, "missing", missing)

	if count > s.markSetSize {
		s.markSetSize = count + count>>2 // overestimate a bit
	}

	err = s.ds.Put(markSetSizeKey, int64ToBytes(s.markSetSize))
	if err != nil {
		log.Warnf("error saving mark set size: %s", err)
	}

	// save the warmup epoch
	err = s.ds.Put(warmupEpochKey, epochToBytes(epoch))
	if err != nil {
		return xerrors.Errorf("error saving warm up epoch: %w", err)
	}
	s.warmupEpoch = epoch

	return nil
}

// Compaction/GC Algorithm
func (s *SplitStore) compact(curTs *types.TipSet, syncGapEpoch abi.ChainEpoch) {
	var err error
	if s.markSetSize == 0 {
		start := time.Now()
		log.Info("estimating mark set size")
		err = s.estimateMarkSetSize(curTs)
		if err != nil {
			log.Errorf("error estimating mark set size: %s; aborting compaction", err)
			return
		}
		log.Infow("estimating mark set size done", "took", time.Since(start), "size", s.markSetSize)
	} else {
		log.Infow("current mark set size estimate", "size", s.markSetSize)
	}

	start := time.Now()
	err = s.doCompact(curTs, syncGapEpoch)
	took := time.Since(start).Milliseconds()
	stats.Record(context.Background(), metrics.SplitstoreCompactionTimeSeconds.M(float64(took)/1e3))

	if err != nil {
		log.Errorf("COMPACTION ERROR: %s", err)
	}
}

func (s *SplitStore) estimateMarkSetSize(curTs *types.TipSet) error {
	epoch := curTs.Height()

	var count int64
	err := s.walk(curTs, epoch, false,
		func(cid cid.Cid) error {
			count++
			return nil
		})

	if err != nil {
		return err
	}

	s.markSetSize = count + count>>2 // overestimate a bit
	return nil
}

func (s *SplitStore) doCompact(curTs *types.TipSet, syncGapEpoch abi.ChainEpoch) error {
	currentEpoch := curTs.Height()
	boundaryEpoch := currentEpoch - CompactionBoundary
	coldEpoch := boundaryEpoch - CompactionSlack

	log.Infow("running compaction", "currentEpoch", currentEpoch, "baseEpoch", s.baseEpoch, "coldEpoch", coldEpoch, "boundaryEpoch", boundaryEpoch)

	markSet, err := s.env.Create("live", s.markSetSize)
	if err != nil {
		return xerrors.Errorf("error creating mark set: %w", err)
	}
	defer markSet.Close() //nolint:errcheck

	// 1. mark reachable objects by walking the chain from the current epoch to the boundary epoch
	log.Infow("marking reachable blocks", "currentEpoch", currentEpoch, "boundaryEpoch", boundaryEpoch)
	startMark := time.Now()

	var inclMsgs bool
	var markTs *types.TipSet
	if syncGapEpoch > boundaryEpoch {
		// There is a sync gap that may have caused writes that are logically after the boundary
		// epoch to be written before the respective head change notification and hence be tracked
		// at the wrong epoch.
		// This can happen if the node is offline or falls out of sync for an extended period of time.
		// In this case we perform a full walk to avoid pathologies with pushing actually hot
		// objects into the coldstore.
		markTs = curTs
		inclMsgs = true
		log.Infof("sync gap detected at epoch %d; marking from current epoch to boundary epoch", syncGapEpoch)
	} else {
		// There is no pathological sync gap, so we can use the much faster single tipset walk at
		// the boundary epoch.
		boundaryTs, err := s.chain.GetTipsetByHeight(context.Background(), boundaryEpoch, curTs, true)
		if err != nil {
			return xerrors.Errorf("error getting tipset at boundary epoch: %w", err)
		}
		markTs = boundaryTs
	}

	var count int64
	err = s.walk(markTs, boundaryEpoch, inclMsgs,
		func(cid cid.Cid) error {
			count++
			return markSet.Mark(cid)
		})

	if err != nil {
		return xerrors.Errorf("error marking cold blocks: %w", err)
	}

	if count > s.markSetSize {
		s.markSetSize = count + count>>2 // overestimate a bit
	}

	log.Infow("marking done", "took", time.Since(startMark), "marked", count)

	// 2. move cold unreachable objects to the coldstore
	log.Info("collecting cold objects")
	startCollect := time.Now()

	cold := make([]cid.Cid, 0, s.coldPurgeSize)

	// some stats for logging
	var hotCnt, coldCnt int

	// 2.1 iterate through the tracking store and collect unreachable cold objects
	err = s.tracker.ForEach(func(cid cid.Cid, writeEpoch abi.ChainEpoch) error {
		// is the object still hot?
		if writeEpoch > coldEpoch {
			// yes, stay in the hotstore
			hotCnt++
			return nil
		}

		// check whether it is reachable in the cold boundary
		mark, err := markSet.Has(cid)
		if err != nil {
			return xerrors.Errorf("error checkiing mark set for %s: %w", cid, err)
		}

		if mark {
			hotCnt++
			return nil
		}

		// it's cold, mark it for move
		cold = append(cold, cid)
		coldCnt++
		return nil
	})

	if err != nil {
		return xerrors.Errorf("error collecting cold objects: %w", err)
	}

	if coldCnt > 0 {
		s.coldPurgeSize = coldCnt + coldCnt>>2 // overestimate a bit
	}

	log.Infow("collection done", "took", time.Since(startCollect))
	log.Infow("compaction stats", "hot", hotCnt, "cold", coldCnt)
	stats.Record(context.Background(), metrics.SplitstoreCompactionHot.M(int64(hotCnt)))
	stats.Record(context.Background(), metrics.SplitstoreCompactionCold.M(int64(coldCnt)))

	// Enter critical section
	atomic.StoreInt32(&s.critsection, 1)
	defer atomic.StoreInt32(&s.critsection, 0)

	// check to see if we are closing first; if that's the case just return
	if atomic.LoadInt32(&s.closing) == 1 {
		log.Info("splitstore is closing; aborting compaction")
		return xerrors.Errorf("compaction aborted")
	}

	// 2.2 copy the cold objects to the coldstore
	log.Info("moving cold blocks to the coldstore")
	startMove := time.Now()
	err = s.moveColdBlocks(cold)
	if err != nil {
		return xerrors.Errorf("error moving cold blocks: %w", err)
	}
	log.Infow("moving done", "took", time.Since(startMove))

	// 2.3 delete cold objects from the hotstore
	log.Info("purging cold objects from the hotstore")
	startPurge := time.Now()
	err = s.purgeBlocks(cold)
	if err != nil {
		return xerrors.Errorf("error purging cold blocks: %w", err)
	}
	log.Infow("purging cold from hotstore done", "took", time.Since(startPurge))

	// 2.4 remove the tracker tracking for cold objects
	startPurge = time.Now()
	log.Info("purging cold objects from tracker")
	err = s.purgeTracking(cold)
	if err != nil {
		return xerrors.Errorf("error purging tracking for cold blocks: %w", err)
	}
	log.Infow("purging cold from tracker done", "took", time.Since(startPurge))

	// we are done; do some housekeeping
	err = s.tracker.Sync()
	if err != nil {
		return xerrors.Errorf("error syncing tracker: %w", err)
	}

	s.gcHotstore()

	err = s.setBaseEpoch(coldEpoch)
	if err != nil {
		return xerrors.Errorf("error saving base epoch: %w", err)
	}

	err = s.ds.Put(markSetSizeKey, int64ToBytes(s.markSetSize))
	if err != nil {
		return xerrors.Errorf("error saving mark set size: %w", err)
	}

	return nil
}

func (s *SplitStore) walk(ts *types.TipSet, boundary abi.ChainEpoch, inclMsgs bool, f func(cid.Cid) error) error {
	walked := cid.NewSet()
	toWalk := ts.Cids()

	walkBlock := func(c cid.Cid) error {
		if !walked.Visit(c) {
			return nil
		}

		blk, err := s.Get(c)
		if err != nil {
			return xerrors.Errorf("error retrieving block (cid: %s): %w", c, err)
		}

		var hdr types.BlockHeader
		if err := hdr.UnmarshalCBOR(bytes.NewBuffer(blk.RawData())); err != nil {
			return xerrors.Errorf("error unmarshaling block header (cid: %s): %w", c, err)
		}

		// don't walk under the boundary, unless we are keeping the headers hot
		if hdr.Height < boundary && !s.cfg.HotHeaders {
			return nil
		}

		if err := f(c); err != nil {
			return err
		}

		if hdr.Height >= boundary {
			if inclMsgs {
				if err := s.walkLinks(hdr.Messages, walked, f); err != nil {
					return xerrors.Errorf("error walking messages (cid: %s): %w", hdr.Messages, err)
				}

				if err := s.walkLinks(hdr.ParentMessageReceipts, walked, f); err != nil {
					return xerrors.Errorf("error walking message receipts (cid: %s): %w", hdr.ParentMessageReceipts, err)
				}
			}

			if err := s.walkLinks(hdr.ParentStateRoot, walked, f); err != nil {
				return xerrors.Errorf("error walking state root (cid: %s): %w", hdr.ParentStateRoot, err)
			}
		}

		if hdr.Height > 0 {
			toWalk = append(toWalk, hdr.Parents...)
		}
		return nil
	}

	for len(toWalk) > 0 {
		walking := toWalk
		toWalk = nil
		for _, c := range walking {
			if err := walkBlock(c); err != nil {
				return xerrors.Errorf("error walking block (cid: %s): %w", c, err)
			}
		}
	}

	return nil
}

func (s *SplitStore) walkLinks(c cid.Cid, walked *cid.Set, f func(cid.Cid) error) error {
	if !walked.Visit(c) {
		return nil
	}

	if err := f(c); err != nil {
		return err
	}

	if c.Prefix().Codec != cid.DagCBOR {
		return nil
	}

	blk, err := s.Get(c)
	if err != nil {
		return xerrors.Errorf("error retrieving linked block (cid: %s): %w", c, err)
	}

	var rerr error
	err = cbg.ScanForLinks(bytes.NewReader(blk.RawData()), func(c cid.Cid) {
		if rerr != nil {
			return
		}

		rerr = s.walkLinks(c, walked, f)
	})

	if err != nil {
		return xerrors.Errorf("error scanning links (cid: %s): %w", c, err)
	}

	return rerr
}

func (s *SplitStore) moveColdBlocks(cold []cid.Cid) error {
	batch := make([]blocks.Block, 0, batchSize)

	for _, cid := range cold {
		blk, err := s.hot.Get(cid)
		if err != nil {
			if err == dstore.ErrNotFound {
				// this can happen if the node is killed after we have deleted the block from the hotstore
				// but before we have deleted it from the tracker; just delete the tracker.
				err = s.tracker.Delete(cid)
				if err != nil {
					return xerrors.Errorf("error deleting unreachable cid %s from tracker: %w", cid, err)
				}
			} else {
				return xerrors.Errorf("error retrieving tracked block %s from hotstore: %w", cid, err)
			}

			continue
		}

		batch = append(batch, blk)
		if len(batch) == batchSize {
			err = s.cold.PutMany(batch)
			if err != nil {
				return xerrors.Errorf("error putting batch to coldstore: %w", err)
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		err := s.cold.PutMany(batch)
		if err != nil {
			return xerrors.Errorf("error putting cold to coldstore: %w", err)
		}
	}

	return nil
}

func (s *SplitStore) purgeBatch(cids []cid.Cid, deleteBatch func([]cid.Cid) error) error {
	if len(cids) == 0 {
		return nil
	}

	// don't delete one giant batch of 7M objects, but rather do smaller batches
	done := false
	for i := 0; !done; i++ {
		start := i * batchSize
		end := start + batchSize
		if end >= len(cids) {
			end = len(cids)
			done = true
		}

		err := deleteBatch(cids[start:end])
		if err != nil {
			return xerrors.Errorf("error deleting batch: %w", err)
		}
	}

	return nil
}

func (s *SplitStore) purgeBlocks(cids []cid.Cid) error {
	return s.purgeBatch(cids, s.hot.DeleteMany)
}

func (s *SplitStore) purgeTracking(cids []cid.Cid) error {
	return s.purgeBatch(cids, s.tracker.DeleteBatch)
}

func (s *SplitStore) gcHotstore() {
	if compact, ok := s.hot.(interface{ Compact() error }); ok {
		log.Infof("compacting hotstore")
		startCompact := time.Now()
		err := compact.Compact()
		if err != nil {
			log.Warnf("error compacting hotstore: %s", err)
			return
		}
		log.Infow("hotstore compaction done", "took", time.Since(startCompact))
	}

	if gc, ok := s.hot.(interface{ CollectGarbage() error }); ok {
		log.Infof("garbage collecting hotstore")
		startGC := time.Now()
		err := gc.CollectGarbage()
		if err != nil {
			log.Warnf("error garbage collecting hotstore: %s", err)
			return
		}
		log.Infow("hotstore garbage collection done", "took", time.Since(startGC))
	}
}

func (s *SplitStore) setBaseEpoch(epoch abi.ChainEpoch) error {
	s.baseEpoch = epoch
	return s.ds.Put(baseEpochKey, epochToBytes(epoch))
}

func (s *SplitStore) setSyncGapEpoch(epoch abi.ChainEpoch) error {
	s.syncGapEpoch = epoch
	return s.ds.Put(syncGapEpochKey, epochToBytes(epoch))
}

func epochToBytes(epoch abi.ChainEpoch) []byte {
	return uint64ToBytes(uint64(epoch))
}

func bytesToEpoch(buf []byte) abi.ChainEpoch {
	return abi.ChainEpoch(bytesToUint64(buf))
}

func int64ToBytes(i int64) []byte {
	return uint64ToBytes(uint64(i))
}

func bytesToInt64(buf []byte) int64 {
	return int64(bytesToUint64(buf))
}

func uint64ToBytes(i uint64) []byte {
	buf := make([]byte, 16)
	n := binary.PutUvarint(buf, i)
	return buf[:n]
}

func bytesToUint64(buf []byte) uint64 {
	i, _ := binary.Uvarint(buf)
	return i
}
