package splitstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	mh "github.com/multiformats/go-multihash"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"

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
	CompactionThreshold = 5 * build.Finality

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
	// MarkSetType is the type of mark set to use.
	//
	// Only current sane value is "map", but we may add an option for a disk-backed
	// markset for memory-constrained situations.
	MarkSetType string

	// DiscardColdBlocks indicates whether to skip moving cold blocks to the coldstore.
	// If the splitstore is running with a noop coldstore then this option is set to true
	// which skips moving (as it is a noop, but still takes time to read all the cold objects)
	// and directly purges cold blocks.
	DiscardColdBlocks bool
}

// ChainAccessor allows the Splitstore to access the chain. It will most likely
// be a ChainStore at runtime.
type ChainAccessor interface {
	GetTipsetByHeight(context.Context, abi.ChainEpoch, *types.TipSet, bool) (*types.TipSet, error)
	GetHeaviestTipSet() *types.TipSet
	SubscribeHeadChanges(change func(revert []*types.TipSet, apply []*types.TipSet) error)
}

// hotstore is the interface that must be satisfied by the hot blockstore; it is an extension
// of the Blockstore interface with the traits we need for compaction.
type hotstore interface {
	bstore.Blockstore
	bstore.BlockstoreIterator
}

type SplitStore struct {
	compacting int32 // compaction (or warmp up) in progress
	closing    int32 // the splitstore is closing

	cfg *Config

	mx          sync.Mutex
	warmupEpoch abi.ChainEpoch // protected by mx
	baseEpoch   abi.ChainEpoch // protected by compaction lock

	headChangeMx sync.Mutex

	coldPurgeSize int

	chain ChainAccessor
	ds    dstore.Datastore
	cold  bstore.Blockstore
	hot   hotstore

	markSetEnv  MarkSetEnv
	markSetSize int64

	ctx    context.Context
	cancel func()

	debug *debugLog

	// transactional protection for concurrent read/writes during compaction
	txnLk      sync.RWMutex
	txnActive  bool
	txnViews   sync.WaitGroup
	txnProtect MarkSet
	txnRefsMx  sync.Mutex
	txnRefs    map[cid.Cid]struct{}
	txnMissing map[cid.Cid]struct{}
}

var _ bstore.Blockstore = (*SplitStore)(nil)

func init() {
	if os.Getenv("LOTUS_SPLITSTORE_DEBUG_LOG") == "1" {
		enableDebugLog = true
	}

	if os.Getenv("LOTUS_SPLITSTORE_DEBUG_LOG_WRITE_TRACES") == "1" {
		enableDebugLogWriteTraces = true
	}
}

// Open opens an existing splistore, or creates a new splitstore. The splitstore
// is backed by the provided hot and cold stores. The returned SplitStore MUST be
// attached to the ChainStore with Start in order to trigger compaction.
func Open(path string, ds dstore.Datastore, hot, cold bstore.Blockstore, cfg *Config) (*SplitStore, error) {
	// hot blockstore must support the hotstore interface
	hots, ok := hot.(hotstore)
	if !ok {
		// be specific about what is missing
		if _, ok := hot.(bstore.BlockstoreIterator); !ok {
			return nil, xerrors.Errorf("hot blockstore does not support efficient iteration: %T", hot)
		}

		return nil, xerrors.Errorf("hot blockstore does not support the necessary traits: %T", hot)
	}

	// the markset env
	markSetEnv, err := OpenMarkSetEnv(path, cfg.MarkSetType)
	if err != nil {
		return nil, err
	}

	// and now we can make a SplitStore
	ss := &SplitStore{
		cfg:        cfg,
		ds:         ds,
		cold:       cold,
		hot:        hots,
		markSetEnv: markSetEnv,

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

func (s *SplitStore) Has(cid cid.Cid) (bool, error) {
	if isIdentiyCid(cid) {
		return true, nil
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	has, err := s.hot.Has(cid)

	if err != nil {
		return has, err
	}

	if has {
		s.trackTxnRef(cid)
		return true, nil
	}

	return s.cold.Has(cid)
}

func (s *SplitStore) Get(cid cid.Cid) (blocks.Block, error) {
	if isIdentiyCid(cid) {
		data, err := decodeIdentityCid(cid)
		if err != nil {
			return nil, err
		}

		return blocks.NewBlockWithCid(data, cid)
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	blk, err := s.hot.Get(cid)

	switch err {
	case nil:
		s.trackTxnRef(cid)
		return blk, nil

	case bstore.ErrNotFound:
		if s.isWarm() {
			s.debug.LogReadMiss(cid)
		}

		blk, err = s.cold.Get(cid)
		if err == nil {
			stats.Record(s.ctx, metrics.SplitstoreMiss.M(1))

		}
		return blk, err

	default:
		return nil, err
	}
}

func (s *SplitStore) GetSize(cid cid.Cid) (int, error) {
	if isIdentiyCid(cid) {
		data, err := decodeIdentityCid(cid)
		if err != nil {
			return 0, err
		}

		return len(data), nil
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	size, err := s.hot.GetSize(cid)

	switch err {
	case nil:
		s.trackTxnRef(cid)
		return size, nil

	case bstore.ErrNotFound:
		if s.isWarm() {
			s.debug.LogReadMiss(cid)
		}

		size, err = s.cold.GetSize(cid)
		if err == nil {
			stats.Record(s.ctx, metrics.SplitstoreMiss.M(1))
		}
		return size, err

	default:
		return 0, err
	}
}

func (s *SplitStore) Put(blk blocks.Block) error {
	if isIdentiyCid(blk.Cid()) {
		return nil
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	err := s.hot.Put(blk)
	if err != nil {
		return err
	}

	s.debug.LogWrite(blk)

	s.trackTxnRef(blk.Cid())
	return nil
}

func (s *SplitStore) PutMany(blks []blocks.Block) error {
	// filter identites
	idcids := 0
	for _, blk := range blks {
		if isIdentiyCid(blk.Cid()) {
			idcids++
		}
	}

	if idcids > 0 {
		if idcids == len(blks) {
			// it's all identities
			return nil
		}

		filtered := make([]blocks.Block, 0, len(blks)-idcids)
		for _, blk := range blks {
			if isIdentiyCid(blk.Cid()) {
				continue
			}
			filtered = append(filtered, blk)
		}

		blks = filtered
	}

	batch := make([]cid.Cid, 0, len(blks))
	for _, blk := range blks {
		batch = append(batch, blk.Cid())
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	err := s.hot.PutMany(blks)
	if err != nil {
		return err
	}

	s.debug.LogWriteMany(blks)

	s.trackTxnRefMany(batch)
	return nil
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

	seen := cid.NewSet()
	ch := make(chan cid.Cid, 8) // buffer is arbitrary, just enough to avoid context switches
	go func() {
		defer cancel()
		defer close(ch)

		for _, in := range []<-chan cid.Cid{chHot, chCold} {
			for c := range in {
				// ensure we only emit each key once
				if !seen.Visit(c) {
					continue
				}

				select {
				case ch <- c:
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
	if isIdentiyCid(cid) {
		data, err := decodeIdentityCid(cid)
		if err != nil {
			return err
		}

		return cb(data)
	}

	// views are (optimistically) protected two-fold:
	// - if there is an active transaction, then the reference is protected.
	// - if there is no active transaction, active views are tracked in a
	//   wait group and compaction is inhibited from starting until they
	//   have all completed.  this is necessary to ensure that a (very) long-running
	//   view can't have its data pointer deleted, which would be catastrophic.
	//   Note that we can't just RLock for the duration of the view, as this could
	//    lead to deadlock with recursive views.
	wg := s.protectView(cid)
	if wg != nil {
		defer wg.Done()
	}

	err := s.hot.View(cid, cb)
	switch err {
	case bstore.ErrNotFound:
		if s.isWarm() {
			s.debug.LogReadMiss(cid)
		}

		err = s.cold.View(cid, cb)
		if err == nil {
			stats.Record(s.ctx, metrics.SplitstoreMiss.M(1))
		}
		return err

	default:
		return err
	}
}

func (s *SplitStore) isWarm() bool {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.warmupEpoch > 0
}

// State tracking
func (s *SplitStore) Start(chain ChainAccessor) error {
	s.chain = chain
	curTs := chain.GetHeaviestTipSet()

	// load base epoch from metadata ds
	// if none, then use current epoch because it's a fresh start
	bs, err := s.ds.Get(baseEpochKey)
	switch err {
	case nil:
		s.baseEpoch = bytesToEpoch(bs)

	case dstore.ErrNotFound:
		if curTs == nil {
			// this can happen in some tests
			break
		}

		err = s.setBaseEpoch(curTs.Height())
		if err != nil {
			return xerrors.Errorf("error saving base epoch: %w", err)
		}

	default:
		return xerrors.Errorf("error loading base epoch: %w", err)
	}

	// load warmup epoch from metadata ds
	bs, err = s.ds.Get(warmupEpochKey)
	switch err {
	case nil:
		s.warmupEpoch = bytesToEpoch(bs)

	case dstore.ErrNotFound:
		// the hotstore hasn't warmed up, start a concurrent warm up
		err = s.warmup(curTs)
		if err != nil {
			return xerrors.Errorf("error warming up: %w", err)
		}

	default:
		return xerrors.Errorf("error loading warmup epoch: %w", err)
	}

	// load markSetSize from metadata ds to provide a size hint for marksets
	bs, err = s.ds.Get(markSetSizeKey)
	switch err {
	case nil:
		s.markSetSize = bytesToInt64(bs)

	case dstore.ErrNotFound:
	default:
		return xerrors.Errorf("error loading mark set size: %w", err)
	}

	log.Infow("starting splitstore", "baseEpoch", s.baseEpoch, "warmupEpoch", s.warmupEpoch)

	// watch the chain
	chain.SubscribeHeadChanges(s.HeadChange)

	return nil
}

func (s *SplitStore) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closing, 0, 1) {
		// already closing
		return nil
	}

	if atomic.LoadInt32(&s.compacting) == 1 {
		log.Warn("close with ongoing compaction in progress; waiting for it to finish...")
		for atomic.LoadInt32(&s.compacting) == 1 {
			time.Sleep(time.Second)
		}
	}

	s.cancel()
	return multierr.Combine(s.markSetEnv.Close(), s.debug.Close())
}

func (s *SplitStore) HeadChange(_, apply []*types.TipSet) error {
	s.headChangeMx.Lock()
	defer s.headChangeMx.Unlock()

	// Revert only.
	if len(apply) == 0 {
		return nil
	}

	curTs := apply[len(apply)-1]
	epoch := curTs.Height()

	// NOTE: there is an implicit invariant assumption that HeadChange is invoked
	//       synchronously and no other HeadChange can be invoked while one is in
	//       progress.
	//       this is guaranteed by the chainstore, and it is pervasive in all lotus
	//       -- if that ever changes then all hell will break loose in general and
	//       we will have a rance to protectTipSets here.
	//      Reagrdless, we put a mutex in HeadChange just to be safe

	if !atomic.CompareAndSwapInt32(&s.compacting, 0, 1) {
		// we are currently compacting -- protect the new tipset(s)
		s.protectTipSets(apply)
		return nil
	}

	// check if we are actually closing first
	if atomic.LoadInt32(&s.closing) == 1 {
		atomic.StoreInt32(&s.compacting, 0)
		return nil
	}

	timestamp := time.Unix(int64(curTs.MinTimestamp()), 0)
	if time.Since(timestamp) > SyncGapTime {
		// don't attempt compaction before we have caught up syncing
		atomic.StoreInt32(&s.compacting, 0)
		return nil
	}

	if epoch-s.baseEpoch > CompactionThreshold {
		// it's time to compact -- prepare the transaction and go!
		wg := s.beginTxnProtect()
		go func() {
			defer atomic.StoreInt32(&s.compacting, 0)
			defer s.endTxnProtect()

			log.Info("compacting splitstore")
			start := time.Now()

			s.compact(curTs, wg)

			log.Infow("compaction done", "took", time.Since(start))
		}()
	} else {
		// no compaction necessary
		atomic.StoreInt32(&s.compacting, 0)
	}

	return nil
}

// transactionally protect incoming tipsets
func (s *SplitStore) protectTipSets(apply []*types.TipSet) {
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	if !s.txnActive {
		return
	}

	var cids []cid.Cid
	for _, ts := range apply {
		cids = append(cids, ts.Cids()...)
	}

	s.trackTxnRefMany(cids)
}

// transactionally protect a view
func (s *SplitStore) protectView(c cid.Cid) *sync.WaitGroup {
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	s.txnViews.Add(1)
	if s.txnActive {
		s.trackTxnRef(c)
	}

	return &s.txnViews
}

// transactionally protect a reference to an object
func (s *SplitStore) trackTxnRef(c cid.Cid) {
	if !s.txnActive {
		// not compacting
		return
	}

	if isUnitaryObject(c) {
		return
	}

	if s.txnProtect != nil {
		mark, err := s.txnProtect.Has(c)
		if err != nil {
			log.Warnf("error checking markset: %s", err)
			// track it anyways
		} else if mark {
			return
		}
	}

	s.txnRefsMx.Lock()
	s.txnRefs[c] = struct{}{}
	s.txnRefsMx.Unlock()
}

// transactionally protect a batch of references
func (s *SplitStore) trackTxnRefMany(cids []cid.Cid) {
	if !s.txnActive {
		// not compacting
		return
	}

	s.txnRefsMx.Lock()
	defer s.txnRefsMx.Unlock()

	quiet := false
	for _, c := range cids {
		if isUnitaryObject(c) {
			continue
		}

		if s.txnProtect != nil {
			mark, err := s.txnProtect.Has(c)
			if err != nil {
				if !quiet {
					quiet = true
					log.Warnf("error checking markset: %s", err)
				}
				// This is inconsistent with trackTxnRef (where we track it
				// anyways).
				continue
			}

			if mark {
				continue
			}
		}

		s.txnRefs[c] = struct{}{}
	}
}

// protect all pending transactional references
func (s *SplitStore) protectTxnRefs(markSet MarkSet) error {
	for {
		var txnRefs map[cid.Cid]struct{}

		s.txnRefsMx.Lock()
		if len(s.txnRefs) > 0 {
			txnRefs = s.txnRefs
			s.txnRefs = make(map[cid.Cid]struct{})
		}
		s.txnRefsMx.Unlock()

		if len(txnRefs) == 0 {
			return nil
		}

		log.Infow("protecting transactional references", "refs", len(txnRefs))
		count := 0
		workch := make(chan cid.Cid, len(txnRefs))
		startProtect := time.Now()

		for c := range txnRefs {
			mark, err := markSet.Has(c)
			if err != nil {
				return xerrors.Errorf("error checking markset: %w", err)
			}

			if mark {
				continue
			}

			workch <- c
			count++
		}
		close(workch)

		if count == 0 {
			return nil
		}

		workers := runtime.NumCPU() / 2
		if workers < 2 {
			workers = 2
		}
		if workers > count {
			workers = count
		}

		worker := func() error {
			for c := range workch {
				err := s.doTxnProtect(c, markSet)
				if err != nil {
					return xerrors.Errorf("error protecting transactional references to %s: %w", c, err)
				}
			}
			return nil
		}

		g := new(errgroup.Group)
		for i := 0; i < workers; i++ {
			g.Go(worker)
		}

		if err := g.Wait(); err != nil {
			return err
		}

		log.Infow("protecting transactional refs done", "took", time.Since(startProtect), "protected", count)
	}
}

// transactionally protect a reference by walking the object and marking.
// concurrent markings are short circuited by checking the markset.
func (s *SplitStore) doTxnProtect(root cid.Cid, markSet MarkSet) error {
	if err := s.checkClosing(); err != nil {
		return err
	}

	// Note: cold objects are deleted heaviest first, so the consituents of an object
	// cannot be deleted before the object itself.
	return s.walkObjectIncomplete(root, cid.NewSet(),
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			mark, err := markSet.Has(c)
			if err != nil {
				return xerrors.Errorf("error checking markset: %w", err)
			}

			// it's marked, nothing to do
			if mark {
				return errStopWalk
			}

			return markSet.Mark(c)
		},
		func(c cid.Cid) error {
			if s.txnMissing != nil {
				log.Warnf("missing object reference %s in %s", c, root)
				s.txnRefsMx.Lock()
				s.txnMissing[c] = struct{}{}
				s.txnRefsMx.Unlock()
			}
			return errStopWalk
		})
}

// warmup acuiqres the compaction lock and spawns a goroutine to warm up the hotstore;
// this is necessary when we sync from a snapshot or when we enable the splitstore
// on top of an existing blockstore (which becomes the coldstore).
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

// the actual warmup procedure; it walks the chain loading all state roots at the boundary
// and headers all the way up to genesis.
// objects are written in batches so as to minimize overhead.
func (s *SplitStore) doWarmup(curTs *types.TipSet) error {
	epoch := curTs.Height()
	batchHot := make([]blocks.Block, 0, batchSize)
	count := int64(0)
	xcount := int64(0)
	missing := int64(0)
	err := s.walkChain(curTs, epoch, false,
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			count++

			has, err := s.hot.Has(c)
			if err != nil {
				return err
			}

			if has {
				return nil
			}

			blk, err := s.cold.Get(c)
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

	s.markSetSize = count + count>>2 // overestimate a bit
	err = s.ds.Put(markSetSizeKey, int64ToBytes(s.markSetSize))
	if err != nil {
		log.Warnf("error saving mark set size: %s", err)
	}

	// save the warmup epoch
	err = s.ds.Put(warmupEpochKey, epochToBytes(epoch))
	if err != nil {
		return xerrors.Errorf("error saving warm up epoch: %w", err)
	}
	s.mx.Lock()
	s.warmupEpoch = epoch
	s.mx.Unlock()

	return nil
}

// --- Compaction ---
// Compaction works transactionally with the following algorithm:
// - We prepare a transaction, whereby all i/o referenced objects through the API are tracked.
// - We walk the chain and mark reachable objects, keeping 4 finalities of state roots and messages and all headers all the way to genesis.
// - Once the chain walk is complete, we begin full transaction protection with concurrent marking; we walk and mark all references created during the chain walk. On the same time, all I/O through the API concurrently marks objects as live references.
// - We collect cold objects by iterating through the hotstore and checking the mark set; if an object is not marked, then it is candidate for purge.
// - When running with a coldstore, we next copy all cold objects to the coldstore.
// - At this point we are ready to begin purging:
//   - We sort cold objects heaviest first, so as to never delete the consituents of a DAG before the DAG itself (which would leave dangling references)
//   - We delete in small batches taking a lock; each batch is checked again for marks, from the concurrent transactional mark, so as to never delete anything live
// - We then end the transaction and compact/gc the hotstore.
func (s *SplitStore) compact(curTs *types.TipSet, wg *sync.WaitGroup) {
	log.Info("waiting for active views to complete")
	start := time.Now()
	// I'm not holding the txnLk here. Is it not possible for:
	//
	// 1. The waitgroup to temporarily hit 0.
	// 2. protectView to be called, raising it back to 1.
	//
	// _Reusing_ a waitgroup is legal. Hitting 0 then going back to 1 while "waiting" is not.
	wg.Wait()
	log.Infow("waiting for active views done", "took", time.Since(start))

	start = time.Now()
	err := s.doCompact(curTs)
	took := time.Since(start).Milliseconds()
	stats.Record(s.ctx, metrics.SplitstoreCompactionTimeSeconds.M(float64(took)/1e3))

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

	if err := s.checkClosing(); err != nil {
		return err
	}

	// we are ready for concurrent marking
	s.beginTxnMarking(markSet)

	// 1. mark reachable objects by walking the chain from the current epoch; we keep state roots
	//   and messages until the boundary epoch.
	log.Info("marking reachable objects")
	startMark := time.Now()

	var count int64
	err = s.walkChain(curTs, boundaryEpoch, true,
		func(c cid.Cid) error {
			if isUnitaryObject(c) {
				return errStopWalk
			}

			count++
			return markSet.Mark(c)
		})

	if err != nil {
		return xerrors.Errorf("error marking: %w", err)
	}

	s.markSetSize = count + count>>2 // overestimate a bit

	log.Infow("marking done", "took", time.Since(startMark), "marked", count)

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 1.1 protect transactional refs
	err = s.protectTxnRefs(markSet)
	if err != nil {
		return xerrors.Errorf("error protecting transactional refs: %w", err)
	}

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 2. iterate through the hotstore to collect cold objects
	log.Info("collecting cold objects")
	startCollect := time.Now()

	// some stats for logging
	var hotCnt, coldCnt int

	cold := make([]cid.Cid, 0, s.coldPurgeSize)
	err = s.hot.ForEachKey(func(c cid.Cid) error {
		// was it marked?
		mark, err := markSet.Has(c)
		if err != nil {
			return xerrors.Errorf("error checking mark set for %s: %w", c, err)
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
		return xerrors.Errorf("error collecting cold objects: %w", err)
	}

	log.Infow("cold collection done", "took", time.Since(startCollect))

	if coldCnt > 0 {
		s.coldPurgeSize = coldCnt + coldCnt>>2 // overestimate a bit
	}

	log.Infow("compaction stats", "hot", hotCnt, "cold", coldCnt)
	stats.Record(s.ctx, metrics.SplitstoreCompactionHot.M(int64(hotCnt)))
	stats.Record(s.ctx, metrics.SplitstoreCompactionCold.M(int64(coldCnt)))

	if err := s.checkClosing(); err != nil {
		return err
	}

	// now that we have collected cold objects, check for missing references from transactional i/o
	// and disable further collection of such references (they will not be acted upon as we can't
	// possibly delete objects we didn't have when we were collecting cold objects)
	s.waitForMissingRefs(markSet)

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 3. copy the cold objects to the coldstore -- if we have one
	if !s.cfg.DiscardColdBlocks {
		log.Info("moving cold objects to the coldstore")
		startMove := time.Now()
		err = s.moveColdBlocks(cold)
		if err != nil {
			return xerrors.Errorf("error moving cold objects: %w", err)
		}
		log.Infow("moving done", "took", time.Since(startMove))

		if err := s.checkClosing(); err != nil {
			return err
		}
	}

	// 4. sort cold objects so that the dags with most references are deleted first
	//    this ensures that we can't refer to a dag with its consituents already deleted, ie
	//    we lave no dangling references.
	log.Info("sorting cold objects")
	startSort := time.Now()
	err = s.sortObjects(cold)
	if err != nil {
		return xerrors.Errorf("error sorting objects: %w", err)
	}
	log.Infow("sorting done", "took", time.Since(startSort))

	// 4.1 protect transactional refs once more
	//     strictly speaking, this is not necessary as purge will do it before deleting each
	//     batch.  however, there is likely a largish number of references accumulated during
	//     ths sort and this protects before entering pruge context.
	err = s.protectTxnRefs(markSet)
	if err != nil {
		return xerrors.Errorf("error protecting transactional refs: %w", err)
	}

	if err := s.checkClosing(); err != nil {
		return err
	}

	// 5. purge cold objects from the hotstore, taking protected references into account
	log.Info("purging cold objects from the hotstore")
	startPurge := time.Now()
	err = s.purge(cold, markSet)
	if err != nil {
		return xerrors.Errorf("error purging cold blocks: %w", err)
	}
	log.Infow("purging cold objects from hotstore done", "took", time.Since(startPurge))

	// we are done; do some housekeeping
	s.endTxnProtect()
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

func (s *SplitStore) beginTxnProtect() *sync.WaitGroup {
	log.Info("preparing compaction transaction")

	s.txnLk.Lock()
	defer s.txnLk.Unlock()

	s.txnActive = true
	s.txnRefs = make(map[cid.Cid]struct{})
	s.txnMissing = make(map[cid.Cid]struct{})

	return &s.txnViews
}

func (s *SplitStore) beginTxnMarking(markSet MarkSet) {
	markSet.SetConcurrent()

	s.txnLk.Lock()
	s.txnProtect = markSet
	s.txnLk.Unlock()
}

func (s *SplitStore) endTxnProtect() {
	s.txnLk.Lock()
	defer s.txnLk.Unlock()

	if !s.txnActive {
		return
	}

	// release markset memory
	if s.txnProtect != nil {
		_ = s.txnProtect.Close()
	}

	s.txnActive = false
	s.txnProtect = nil
	s.txnRefs = nil
	s.txnMissing = nil
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
		if hdr.Height >= boundary || hdr.Height == 0 {
			scanCnt++
			if inclMsgs && hdr.Height > 0 {
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
		// walking can take a while, so check this with every opportunity
		if err := s.checkClosing(); err != nil {
			return err
		}

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

	// check this before recursing
	if err := s.checkClosing(); err != nil {
		return err
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

// like walkObject, but the object may be potentially incomplete (references missing)
func (s *SplitStore) walkObjectIncomplete(c cid.Cid, walked *cid.Set, f, missing func(cid.Cid) error) error {
	if !walked.Visit(c) {
		return nil
	}

	// occurs check -- only for DAGs
	if c.Prefix().Codec == cid.DagCBOR {
		has, err := s.has(c)
		if err != nil {
			return xerrors.Errorf("error occur checking %s: %w", c, err)
		}

		if !has {
			err = missing(c)
			if err == errStopWalk {
				return nil
			}

			return err
		}
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

	// check this before recursing
	if err := s.checkClosing(); err != nil {
		return err
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
		err := s.walkObjectIncomplete(c, walked, f, missing)
		if err != nil {
			return xerrors.Errorf("error walking link (cid: %s): %w", c, err)
		}
	}

	return nil
}

// internal version used by walk
func (s *SplitStore) view(c cid.Cid, cb func([]byte) error) error {
	if isIdentiyCid(c) {
		data, err := decodeIdentityCid(c)
		if err != nil {
			return err
		}

		return cb(data)
	}

	err := s.hot.View(c, cb)
	switch err {
	case bstore.ErrNotFound:
		return s.cold.View(c, cb)

	default:
		return err
	}
}

func (s *SplitStore) has(c cid.Cid) (bool, error) {
	if isIdentiyCid(c) {
		return true, nil
	}

	has, err := s.hot.Has(c)

	if has || err != nil {
		return has, err
	}

	return s.cold.Has(c)
}

func (s *SplitStore) checkClosing() error {
	if atomic.LoadInt32(&s.closing) == 1 {
		log.Info("splitstore is closing; aborting compaction")
		return xerrors.Errorf("compaction aborted")
	}

	return nil
}

func (s *SplitStore) moveColdBlocks(cold []cid.Cid) error {
	batch := make([]blocks.Block, 0, batchSize)

	for _, c := range cold {
		if err := s.checkClosing(); err != nil {
			return err
		}

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
			return xerrors.Errorf("error putting batch to coldstore: %w", err)
		}
	}

	return nil
}

// sorts a slice of objects heaviest first -- it's a little expensive but worth the
// guarantee that we don't leave dangling references behind, e.g. if we die in the middle
// of a purge.
func (s *SplitStore) sortObjects(cids []cid.Cid) error {
	// we cache the keys to avoid making a gazillion of strings
	keys := make(map[cid.Cid]string)
	key := func(c cid.Cid) string {
		s, ok := keys[c]
		if !ok {
			s = string(c.Hash())
			keys[c] = s
		}
		return s
	}

	// compute sorting weights as the cumulative number of DAG links
	weights := make(map[string]int)
	for _, c := range cids {
		// this can take quite a while, so check for shutdown with every opportunity
		if err := s.checkClosing(); err != nil {
			return err
		}

		w := s.getObjectWeight(c, weights, key)
		weights[key(c)] = w
	}

	// sort!
	sort.Slice(cids, func(i, j int) bool {
		wi := weights[key(cids[i])]
		wj := weights[key(cids[j])]
		if wi == wj {
			// nit: consider just comparing the keys?
			return bytes.Compare(cids[i].Hash(), cids[j].Hash()) > 0
		}

		return wi > wj
	})

	return nil
}

func (s *SplitStore) getObjectWeight(c cid.Cid, weights map[string]int, key func(cid.Cid) string) int {
	w, ok := weights[key(c)]
	if ok {
		return w
	}

	// we treat block headers specially to avoid walking the entire chain
	var hdr types.BlockHeader
	err := s.view(c, func(data []byte) error {
		return hdr.UnmarshalCBOR(bytes.NewBuffer(data))
	})
	if err == nil {
		w1 := s.getObjectWeight(hdr.ParentStateRoot, weights, key)
		weights[key(hdr.ParentStateRoot)] = w1

		w2 := s.getObjectWeight(hdr.Messages, weights, key)
		weights[key(hdr.Messages)] = w2

		return 1 + w1 + w2
	}

	var links []cid.Cid
	err = s.view(c, func(data []byte) error {
		return cbg.ScanForLinks(bytes.NewReader(data), func(c cid.Cid) {
			links = append(links, c)
		})
	})
	if err != nil {
		return 1
	}

	w = 1
	for _, c := range links {
		// these are internal refs, so dags will be dags
		if c.Prefix().Codec != cid.DagCBOR {
			w++
			continue
		}

		wc := s.getObjectWeight(c, weights, key)
		weights[key(c)] = wc

		w += wc
	}

	return w
}

func (s *SplitStore) purgeBatch(cids []cid.Cid, deleteBatch func([]cid.Cid) error) error {
	if len(cids) == 0 {
		return nil
	}

	// we don't delete one giant batch of millions of objects, but rather do smaller batches
	// so that we don't stop the world for an extended period of time
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

func (s *SplitStore) purge(cids []cid.Cid, markSet MarkSet) error {
	deadCids := make([]cid.Cid, 0, batchSize)
	var purgeCnt, liveCnt int
	defer func() {
		log.Infow("purged cold objects", "purged", purgeCnt, "live", liveCnt)
	}()

	return s.purgeBatch(cids,
		func(cids []cid.Cid) error {
			deadCids := deadCids[:0]

			for {
				if err := s.checkClosing(); err != nil {
					return err
				}

				s.txnLk.Lock()
				if len(s.txnRefs) == 0 {
					// keep the lock!
					break
				}

				// unlock and protect
				s.txnLk.Unlock()

				err := s.protectTxnRefs(markSet)
				if err != nil {
					return xerrors.Errorf("error protecting transactional refs: %w", err)
				}
			}

			defer s.txnLk.Unlock()

			for _, c := range cids {
				live, err := markSet.Has(c)
				if err != nil {
					return xerrors.Errorf("error checking for liveness: %w", err)
				}

				if live {
					liveCnt++
					continue
				}

				deadCids = append(deadCids, c)
			}

			err := s.hot.DeleteMany(deadCids)
			if err != nil {
				return xerrors.Errorf("error purging cold objects: %w", err)
			}

			s.debug.LogDelete(deadCids)

			purgeCnt += len(deadCids)
			return nil
		})
}

// I really don't like having this code, but we seem to have some occasional DAG references with
// missing constituents. During testing in mainnet *some* of these references *sometimes* appeared
// after a little bit.
// We need to figure out where they are coming from and eliminate that vector, but until then we
// have this gem[TM].
// My best guess is that they are parent message receipts or yet to be computed state roots; magik
// thinks the cause may be block validation.
func (s *SplitStore) waitForMissingRefs(markSet MarkSet) {
	s.txnLk.Lock()
	missing := s.txnMissing
	s.txnMissing = nil
	s.txnLk.Unlock()

	if len(missing) == 0 {
		return
	}

	log.Info("waiting for missing references")
	start := time.Now()
	count := 0
	defer func() {
		log.Infow("waiting for missing references done", "took", time.Since(start), "marked", count)
	}()

	for i := 0; i < 3 && len(missing) > 0; i++ {
		if err := s.checkClosing(); err != nil {
			return
		}

		wait := time.Duration(i) * time.Minute
		log.Infof("retrying for %d missing references in %s (attempt: %d)", len(missing), wait, i+1)
		if wait > 0 {
			time.Sleep(wait)
		}

		towalk := missing
		walked := cid.NewSet()
		missing = make(map[cid.Cid]struct{})

		for c := range towalk {
			err := s.walkObjectIncomplete(c, walked,
				func(c cid.Cid) error {
					if isUnitaryObject(c) {
						return errStopWalk
					}

					mark, err := markSet.Has(c)
					if err != nil {
						return xerrors.Errorf("error checking markset for %s: %w", c, err)
					}

					if mark {
						return errStopWalk
					}

					count++
					return markSet.Mark(c)
				},
				func(c cid.Cid) error {
					missing[c] = struct{}{}
					return errStopWalk
				})

			if err != nil {
				log.Warnf("error marking: %s", err)
			}
		}
	}

	if len(missing) > 0 {
		log.Warnf("still missing %d references", len(missing))
		for c := range missing {
			log.Warnf("unresolved missing reference: %s", c)
		}
	}
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

func isUnitaryObject(c cid.Cid) bool {
	pre := c.Prefix()
	switch pre.Codec {
	case cid.FilCommitmentSealed, cid.FilCommitmentUnsealed:
		return true
	default:
		return pre.MhType == mh.IDENTITY
	}
}

func isIdentiyCid(c cid.Cid) bool {
	return c.Prefix().MhType == mh.IDENTITY
}

func decodeIdentityCid(c cid.Cid) ([]byte, error) {
	dmh, err := mh.Decode(c.Hash())
	if err != nil {
		return nil, xerrors.Errorf("error decoding identity cid %s: %w", c, err)
	}

	// sanity check
	if dmh.Code != mh.IDENTITY {
		return nil, xerrors.Errorf("error decoding identity cid %s: hash type is not identity", c)
	}

	return dmh.Digest, nil
}
