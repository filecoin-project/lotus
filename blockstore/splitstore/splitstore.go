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
	//        |                                             |
	// =======‖≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡‖------------------------»
	//        |                    |  chain -->             ↑__ current epoch
	//        | archived epochs ___↑
	//                             ↑________ CompactionBoundary
	//
	// === :: cold (already archived)
	// ≡≡≡ :: to be archived in this compaction
	// --- :: hot
	CompactionThreshold = 6 * build.Finality

	// CompactionBoundary is the number of epochs from the current epoch at which
	// we will walk the chain for live objects.
	CompactionBoundary = 4 * build.Finality

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

	// markSetSizeKey stores the current estimate for the mark set size.
	// this is first computed at warmup and updated in every compaction
	markSetSizeKey = dstore.NewKey("/splitstore/markSetSize")

	log = logging.Logger("splitstore")

	// used to signal end of walk
	errStopWalk = errors.New("stop walk")
	// used to signal a missing object when protecting recursive references
	errMissingObject = errors.New("missing object")

	// set this to true if you are debugging the splitstore to enable debug logging
	enableDebugLog = false
	// set this to true if you want to track origin stack traces in the write log
	enableDebugLogWriteTraces = false
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

	// SkipMoveColdBlocks indicates whether to skip moving cold blocks to the coldstore.
	// If the splitstore is running with a noop coldstore then this option is set to true
	// which skips moving (as it is a noop, but still takes time to read all the cold objects)
	// and directly purges cold blocks.
	DiscardColdBlocks bool
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

	baseEpoch   abi.ChainEpoch
	warmupEpoch abi.ChainEpoch
	writeEpoch  abi.ChainEpoch

	coldPurgeSize int

	mx    sync.Mutex
	curTs *types.TipSet

	chain ChainAccessor
	ds    dstore.Datastore
	hot   bstore.Blockstore
	cold  bstore.Blockstore

	markSetEnv  MarkSetEnv
	markSetSize int64

	ctx    context.Context
	cancel func()

	debug *debugLog

	// protection for concurrent read/writes during compaction
	txnLk      sync.RWMutex
	txnActive  bool
	txnEnv     MarkSetEnv
	txnProtect MarkSet
	txnMarkSet MarkSet
	txnRefsMx  sync.Mutex
	txnRefs    map[cid.Cid]struct{}
}

var _ bstore.Blockstore = (*SplitStore)(nil)

// Open opens an existing splistore, or creates a new splitstore. The splitstore
// is backed by the provided hot and cold stores. The returned SplitStore MUST be
// attached to the ChainStore with Start in order to trigger compaction.
func Open(path string, ds dstore.Datastore, hot, cold bstore.Blockstore, cfg *Config) (*SplitStore, error) {
	// hot blockstore must support BlockstoreIterator
	if _, ok := hot.(bstore.BlockstoreIterator); !ok {
		return nil, xerrors.Errorf("hot blockstore does not support efficient iteration: %T", hot)
	}

	// the markset env
	markSetEnv, err := OpenMarkSetEnv(path, "mapts")
	if err != nil {
		return nil, err
	}

	// the txn markset env
	txnEnv, err := OpenMarkSetEnv(path, "mapts")
	if err != nil {
		_ = markSetEnv.Close()
		return nil, err
	}

	// and now we can make a SplitStore
	ss := &SplitStore{
		cfg:        cfg,
		ds:         ds,
		hot:        hot,
		cold:       cold,
		markSetEnv: markSetEnv,
		txnEnv:     txnEnv,

		coldPurgeSize: defaultColdPurgeSize,
	}

	ss.ctx, ss.cancel = context.WithCancel(context.Background())

	if enableDebugLog {
		ss.debug, err = openDebugLog(path)
		if err != nil {
			return nil, err
		}
	}

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

func (s *SplitStore) Has(c cid.Cid) (bool, error) {
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	has, err := s.hot.Has(c)

	if err != nil {
		return has, err
	}

	if has {
		// treat it as an implicit (recursive) Write, when it is within vm.Copy context.
		// -- the vm uses this check to avoid duplicate writes on Copy.
		// When we have options in the API (or something better), the vm can explicitly signal
		// that this is an implicit Write.
		err = s.trackTxnRef(c, true)
		if xerrors.Is(err, errMissingObject) {
			// we failed to recursively protect the object because some inner object has been purged;
			// signal to the VM to copy.
			return false, nil
		}

		return true, err
	}

	return s.cold.Has(c)
}

func (s *SplitStore) Get(cid cid.Cid) (blocks.Block, error) {
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	blk, err := s.hot.Get(cid)

	switch err {
	case nil:
		err = s.trackTxnRef(cid, false)
		return blk, err

	case bstore.ErrNotFound:
		if s.debug != nil {
			s.mx.Lock()
			warm := s.warmupEpoch > 0
			curTs := s.curTs
			s.mx.Unlock()
			if warm {
				s.debug.LogReadMiss(curTs, cid)
			}
		}

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
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	size, err := s.hot.GetSize(cid)

	switch err {
	case nil:
		err = s.trackTxnRef(cid, false)
		return size, err

	case bstore.ErrNotFound:
		if s.debug != nil {
			s.mx.Lock()
			warm := s.warmupEpoch > 0
			curTs := s.curTs
			s.mx.Unlock()
			if warm {
				s.debug.LogReadMiss(curTs, cid)
			}
		}

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
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	err := s.hot.Put(blk)
	if err == nil {
		if s.debug != nil {
			s.mx.Lock()
			curTs := s.curTs
			writeEpoch := s.writeEpoch
			s.mx.Unlock()
			s.debug.LogWrite(curTs, blk, writeEpoch)
		}
		err = s.trackTxnRef(blk.Cid(), false)
	}

	return err
}

func (s *SplitStore) PutMany(blks []blocks.Block) error {
	batch := make([]cid.Cid, 0, len(blks))
	for _, blk := range blks {
		batch = append(batch, blk.Cid())
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	err := s.hot.PutMany(blks)
	if err == nil {
		if s.debug != nil {
			s.mx.Lock()
			curTs := s.curTs
			writeEpoch := s.writeEpoch
			s.mx.Unlock()
			s.debug.LogWriteMany(curTs, blks, writeEpoch)
		}

		err = s.trackTxnRefMany(batch)
	}

	return err
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
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	err := s.hot.View(cid, cb)
	switch err {
	case nil:
		err = s.trackTxnRef(cid, false)
		return err

	case bstore.ErrNotFound:
		if s.debug != nil {
			s.mx.Lock()
			warm := s.warmupEpoch > 0
			curTs := s.curTs
			s.mx.Unlock()
			if warm {
				s.debug.LogReadMiss(curTs, cid)
			}
		}

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

	log.Infow("starting splitstore", "baseEpoch", s.baseEpoch, "warmupEpoch", s.warmupEpoch)

	if s.debug != nil {
		go s.background()
	}

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
	return multierr.Combine(s.markSetEnv.Close(), s.debug.Close())
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

	timestamp := time.Unix(int64(curTs.MinTimestamp()), 0)
	if time.Since(timestamp) > SyncGapTime {
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

			s.compact(curTs)

			log.Infow("compaction done", "took", time.Since(start))
		}()
	} else {
		// no compaction necessary
		atomic.StoreInt32(&s.compacting, 0)
	}

	return nil
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

func (s *SplitStore) trackTxnRef(c cid.Cid, recursive bool) error {
	if !s.txnActive {
		// not compacting
		return nil
	}

	if s.txnRefs != nil {
		// we haven't finished marking yet, so track the reference
		s.txnRefsMx.Lock()
		s.txnRefs[c] = struct{}{}
		s.txnRefsMx.Unlock()
		return nil
	}

	// we have finished marking, protect the reference
	if !recursive {
		return s.txnProtect.Mark(c)
	}

	// it's a recursive reference in vm context, protect links if they are not in the markset already
	return s.walkObject(c, cid.NewSet(), func(c cid.Cid) error {
		mark, err := s.txnMarkSet.Has(c)
		if err != nil {
			return xerrors.Errorf("error checking mark set for %s: %w", c, err)
		}

		// it's marked, nothing to do
		if mark {
			return errStopWalk
		}

		live, err := s.txnProtect.Has(c)
		if err != nil {
			return xerrors.Errorf("error checking portected set for %s: %w", c, err)
		}

		if live {
			return errStopWalk
		}

		// this occurs check is necessary because cold objects are purged in arbitrary order
		has, err := s.hot.Has(c)
		if err != nil {
			return xerrors.Errorf("error checking hotstore for %s: %w", c, err)
		}

		// it has been deleted, signal to the vm to copy
		if !has {
			log.Warnf("missing object for recursive reference to %s: %s", c, err)
			return errMissingObject
		}

		// mark it
		return s.txnProtect.Mark(c)
	})
}

func (s *SplitStore) trackTxnRefMany(cids []cid.Cid) error {
	if !s.txnActive {
		// not compacting
		return nil
	}

	if s.txnRefs != nil {
		// we haven't finished marking yet, so track the reference
		s.txnRefsMx.Lock()
		for _, c := range cids {
			s.txnRefs[c] = struct{}{}
		}
		s.txnRefsMx.Unlock()
		return nil
	}

	// we have finished marking, protect the refs
	for _, c := range cids {
		err := s.txnProtect.Mark(c)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SplitStore) warmup(curTs *types.TipSet) error {
	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		return xerrors.Errorf("error locking compaction")
	}

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

func (s *SplitStore) doWarmup(curTs *types.TipSet) error {
	epoch := curTs.Height()
	batchHot := make([]blocks.Block, 0, batchSize)
	count := int64(0)
	xcount := int64(0)
	missing := int64(0)
	err := s.walkChain(curTs, epoch, false,
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
			if len(batchHot) == batchSize {
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
func (s *SplitStore) compact(curTs *types.TipSet) {
	start := time.Now()
	err := s.doCompact(curTs)
	took := time.Since(start).Milliseconds()
	stats.Record(context.Background(), metrics.SplitstoreCompactionTimeSeconds.M(float64(took)/1e3))

	if err != nil {
		log.Errorf("COMPACTION ERROR: %s", err)
	}
}

func (s *SplitStore) doCompact(curTs *types.TipSet) error {
	currentEpoch := curTs.Height()
	boundaryEpoch := currentEpoch - CompactionBoundary

	log.Infow("running compaction", "currentEpoch", currentEpoch, "baseEpoch", s.baseEpoch, "boundaryEpoch", boundaryEpoch)

	markSet, err := s.markSetEnv.Create("live", s.markSetSize)
	if err != nil {
		return xerrors.Errorf("error creating mark set: %w", err)
	}
	defer markSet.Close() //nolint:errcheck
	defer s.debug.Flush()

	// 0. Prepare the transaction
	s.txnLk.Lock()
	s.txnRefs = make(map[cid.Cid]struct{})
	s.txnActive = true
	s.txnLk.Unlock()

	// 1. mark reachable objects by walking the chain from the current epoch to the boundary epoch
	log.Info("marking reachable blocks")
	startMark := time.Now()

	var count int64
	err = s.walkChain(curTs, boundaryEpoch, true,
		func(c cid.Cid) error {
			count++
			return markSet.Mark(c)
		})

	if err != nil {
		return xerrors.Errorf("error marking cold blocks: %w", err)
	}

	if count > s.markSetSize {
		s.markSetSize = count + count>>2 // overestimate a bit
	}

	log.Infow("marking done", "took", time.Since(startMark), "marked", count)

	// fetch refernces taken during marking and create the transaction protect filter
	s.txnLk.Lock()
	txnRefs := s.txnRefs
	s.txnRefs = nil
	s.txnProtect, err = s.txnEnv.Create("protected", 0)
	if err != nil {
		s.txnLk.Unlock()
		return xerrors.Errorf("error creating transactional mark set: %w", err)
	}
	s.txnMarkSet = markSet
	s.txnLk.Unlock()

	defer func() {
		s.txnLk.Lock()
		_ = s.txnProtect.Close()
		s.txnActive = false
		s.txnProtect = nil
		s.txnMarkSet = nil
		s.txnLk.Unlock()
	}()

	// 1.1 Update markset for references created during marking
	log.Info("updating mark set for live references")
	startMark = time.Now()
	walked := cid.NewSet()
	count = 0
	for c := range txnRefs {
		mark, err := markSet.Has(c)
		if err != nil {
			return xerrors.Errorf("error checking markset for %s: %w", c, err)
		}

		if mark {
			continue
		}

		err = s.walkObject(c, walked, func(c cid.Cid) error {
			mark, err := markSet.Has(c)
			if err != nil {
				return xerrors.Errorf("error checking markset for %s: %w", c, err)
			}

			if mark {
				return errStopWalk
			}

			count++
			return markSet.Mark(c)
		})

		if err != nil {
			return xerrors.Errorf("error walking %s for marking: %w", c, err)
		}
	}
	log.Infow("update marking set done", "took", time.Since(startMark), "marked", count)

	// 2. iterate through the hotstore to collect cold objects
	log.Info("collecting cold objects")
	startCollect := time.Now()

	// some stats for logging
	var hotCnt, coldCnt int

	cold := make([]cid.Cid, 0, s.coldPurgeSize)
	err = s.hot.(bstore.BlockstoreIterator).ForEachKey(func(c cid.Cid) error {
		// was it marked?
		mark, err := markSet.Has(c)
		if err != nil {
			return xerrors.Errorf("error checkiing mark set for %s: %w", c, err)
		}

		if mark {
			hotCnt++
			return nil
		}

		// it's cold, mark it as candidate for move
		cold = append(cold, c)
		coldCnt++

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error collecting candidate cold objects: %w", err)
	}

	log.Infow("candidate collection done", "took", time.Since(startCollect))

	if coldCnt > 0 {
		s.coldPurgeSize = coldCnt + coldCnt>>2 // overestimate a bit
	}

	log.Infow("compaction stats", "hot", hotCnt, "cold", coldCnt)
	stats.Record(context.Background(), metrics.SplitstoreCompactionHot.M(int64(hotCnt)))
	stats.Record(context.Background(), metrics.SplitstoreCompactionCold.M(int64(coldCnt)))

	// Enter critical section
	log.Info("entering critical section")
	atomic.StoreInt32(&s.critsection, 1)
	defer atomic.StoreInt32(&s.critsection, 0)

	// check to see if we are closing first; if that's the case just return
	if atomic.LoadInt32(&s.closing) == 1 {
		log.Info("splitstore is closing; aborting compaction")
		return xerrors.Errorf("compaction aborted")
	}

	// 3. copy the cold objects to the coldstore -- if we have one
	if !s.cfg.DiscardColdBlocks {
		log.Info("moving cold blocks to the coldstore")
		startMove := time.Now()
		err = s.moveColdBlocks(cold)
		if err != nil {
			return xerrors.Errorf("error moving cold blocks: %w", err)
		}
		log.Infow("moving done", "took", time.Since(startMove))
	}

	// 4. purge cold objects from the hotstore, taking protected references into account
	log.Info("purging cold objects from the hotstore")
	startPurge := time.Now()
	err = s.purge(curTs, cold)
	if err != nil {
		return xerrors.Errorf("error purging cold blocks: %w", err)
	}
	log.Infow("purging cold from hotstore done", "took", time.Since(startPurge))

	// we are done; do some housekeeping
	s.gcHotstore()

	err = s.setBaseEpoch(boundaryEpoch)
	if err != nil {
		return xerrors.Errorf("error saving base epoch: %w", err)
	}

	err = s.ds.Put(markSetSizeKey, int64ToBytes(s.markSetSize))
	if err != nil {
		return xerrors.Errorf("error saving mark set size: %w", err)
	}

	return nil
}

func (s *SplitStore) walkChain(ts *types.TipSet, boundary abi.ChainEpoch, inclMsgs bool,
	f func(cid.Cid) error) error {
	visited := cid.NewSet()
	walked := cid.NewSet()
	toWalk := ts.Cids()
	walkCnt := 0
	scanCnt := 0

	walkBlock := func(c cid.Cid) error {
		if !visited.Visit(c) {
			return nil
		}

		walkCnt++

		if err := f(c); err != nil {
			return err
		}

		var hdr types.BlockHeader
		err := s.view(c, func(data []byte) error {
			return hdr.UnmarshalCBOR(bytes.NewBuffer(data))
		})

		if err != nil {
			return xerrors.Errorf("error unmarshaling block header (cid: %s): %w", c, err)
		}

		// we only scan the block if it is at or above the boundary
		if hdr.Height >= boundary {
			scanCnt++
			if inclMsgs {
				if err := s.walkObject(hdr.Messages, walked, f); err != nil {
					return xerrors.Errorf("error walking messages (cid: %s): %w", hdr.Messages, err)
				}

				if err := s.walkObject(hdr.ParentMessageReceipts, walked, f); err != nil {
					return xerrors.Errorf("error walking message receipts (cid: %s): %w", hdr.ParentMessageReceipts, err)
				}
			}

			if err := s.walkObject(hdr.ParentStateRoot, walked, f); err != nil {
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

	log.Infow("chain walk done", "walked", walkCnt, "scanned", scanCnt)

	return nil
}

func (s *SplitStore) walkObject(c cid.Cid, walked *cid.Set, f func(cid.Cid) error) error {
	if !walked.Visit(c) {
		return nil
	}

	if err := f(c); err != nil {
		if err == errStopWalk {
			return nil
		}

		return err
	}

	if c.Prefix().Codec != cid.DagCBOR {
		return nil
	}

	var links []cid.Cid
	err := s.view(c, func(data []byte) error {
		return cbg.ScanForLinks(bytes.NewReader(data), func(c cid.Cid) {
			links = append(links, c)
		})
	})

	if err != nil {
		return xerrors.Errorf("error scanning linked block (cid: %s): %w", c, err)
	}

	for _, c := range links {
		err := s.walkObject(c, walked, f)
		if err != nil {
			return xerrors.Errorf("error walking link (cid: %s): %w", c, err)
		}
	}

	return nil
}

// internal version used by walk
func (s *SplitStore) view(cid cid.Cid, cb func([]byte) error) error {
	err := s.hot.View(cid, cb)
	switch err {
	case bstore.ErrNotFound:
		return s.cold.View(cid, cb)

	default:
		return err
	}
}

func (s *SplitStore) moveColdBlocks(cold []cid.Cid) error {
	batch := make([]blocks.Block, 0, batchSize)

	for _, c := range cold {
		blk, err := s.hot.Get(c)
		if err != nil {
			if err == bstore.ErrNotFound {
				log.Warnf("hotstore missing block %s", c)
				continue
			}

			return xerrors.Errorf("error retrieving block %s from hotstore: %w", c, err)
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

func (s *SplitStore) purge(curTs *types.TipSet, cids []cid.Cid) error {
	deadCids := make([]cid.Cid, 0, batchSize)
	var purgeCnt, liveCnt int
	defer func() {
		log.Infow("purged cold objects", "purged", purgeCnt, "live", liveCnt)
	}()

	return s.purgeBatch(cids,
		func(cids []cid.Cid) error {
			deadCids := deadCids[:0]

			s.txnLk.Lock()
			defer s.txnLk.Unlock()

			for _, c := range cids {
				live, err := s.txnProtect.Has(c)
				if err != nil {
					return xerrors.Errorf("error checking for liveness: %w", err)
				}

				if live {
					liveCnt++
					continue
				}

				deadCids = append(deadCids, c)
				s.debug.LogMove(curTs, c)
			}

			err := s.hot.DeleteMany(deadCids)
			if err != nil {
				return xerrors.Errorf("error purging cold objects: %w", err)
			}

			purgeCnt += len(deadCids)
			return nil
		})
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
