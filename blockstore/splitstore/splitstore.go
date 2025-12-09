package splitstore

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/stats"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"
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

	// compactionIndexKey stores the compaction index (serial number)
	compactionIndexKey = dstore.NewKey("/splitstore/compactionIndex")

	// stores the prune index (serial number)
	pruneIndexKey = dstore.NewKey("/splitstore/pruneIndex")

	// stores the base epoch of last prune in the metadata store
	pruneEpochKey = dstore.NewKey("/splitstore/pruneEpoch")

	log = logging.Logger("splitstore")

	errClosing = errors.New("splitstore is closing")

	// set this to true if you are debugging the splitstore to enable debug logging
	enableDebugLog = false
	// set this to true if you want to track origin stack traces in the write log
	enableDebugLogWriteTraces = false

	// upgradeBoundary is the boundary before and after an upgrade where we suppress compaction
	upgradeBoundary = policy.ChainFinality
)

type CompactType int

const (
	none CompactType = iota
	warmup
	hot
	cold
	check
)

func init() {
	if os.Getenv("LOTUS_SPLITSTORE_DEBUG_LOG") == "1" {
		enableDebugLog = true
	}

	if os.Getenv("LOTUS_SPLITSTORE_DEBUG_LOG_WRITE_TRACES") == "1" {
		enableDebugLogWriteTraces = true
	}
}

type Config struct {
	// MarkSetType is the type of mark set to use.
	//
	// The default value is "map", which uses an in-memory map-backed markset.
	// If you are constrained in memory (i.e. compaction runs out of memory), you
	// can use "badger", which will use a disk-backed markset using badger.
	// Note that compaction will take quite a bit longer when using the "badger" option,
	// but that shouldn't really matter (as long as it is under 7.5hrs).
	MarkSetType string

	// DiscardColdBlocks indicates whether to skip moving cold blocks to the coldstore.
	// If the splitstore is running with a noop coldstore then this option is set to true
	// which skips moving (as it is a noop, but still takes time to read all the cold objects)
	// and directly purges cold blocks.
	DiscardColdBlocks bool

	// UniversalColdBlocks indicates whether all blocks being garbage collected and purged
	// from the hotstore should be written to the cold store
	UniversalColdBlocks bool

	// HotStoreMessageRetention indicates the hotstore retention policy for messages.
	// It has the following semantics:
	// - a value of 0 will only retain messages within the compaction boundary (4 finalities)
	// - a positive integer indicates the number of finalities, outside the compaction boundary,
	//   for which messages will be retained in the hotstore.
	HotStoreMessageRetention uint64

	// HotStoreFullGCFrequency indicates how frequently (in terms of compactions) to garbage collect
	// the hotstore using full (moving) GC if supported by the hotstore.
	// A value of 0 disables full GC entirely.
	// A positive value is the number of compactions before a full GC is performed;
	// a value of 1 will perform full GC in every compaction.
	HotStoreFullGCFrequency uint64

	// HotstoreMaxSpaceTarget suggests the max allowed space the hotstore can take.
	// This is not a hard limit, it is possible for the hotstore to exceed the target
	// for example if state grows massively between compactions. The splitstore
	// will make a best effort to avoid overflowing the target and in practice should
	// never overflow.  This field is used when doing GC at the end of a compaction to
	// adaptively choose moving GC
	HotstoreMaxSpaceTarget uint64

	// Moving GC will be triggered when total moving size exceeds
	// HotstoreMaxSpaceTarget - HotstoreMaxSpaceThreshold
	HotstoreMaxSpaceThreshold uint64

	// Safety buffer to prevent moving GC from overflowing disk.
	// Moving GC will not occur when total moving size exceeds
	// HotstoreMaxSpaceTarget - HotstoreMaxSpaceSafetyBuffer
	HotstoreMaxSpaceSafetyBuffer uint64
}

// ChainAccessor allows the Splitstore to access the chain. It will most likely
// be a ChainStore at runtime.
type ChainAccessor interface {
	GetTipsetByHeight(context.Context, abi.ChainEpoch, *types.TipSet, bool) (*types.TipSet, error)
	GetHeaviestTipSet() *types.TipSet
	SubscribeHeadChanges(change func(revert []*types.TipSet, apply []*types.TipSet) error)
}

// upgradeRange is a precomputed epoch range during which we shouldn't compact so as to not
// interfere with an upgrade
type upgradeRange struct {
	start, end abi.ChainEpoch
}

// hotstore is the interface that must be satisfied by the hot blockstore; it is an extension
// of the Blockstore interface with the traits we need for compaction.
type hotstore interface {
	bstore.Blockstore
	bstore.BlockstoreIterator
}

type SplitStore struct {
	compacting  int32       // flag for when compaction is in progress
	compactType CompactType // compaction type, protected by compacting atomic, only meaningful when compacting == 1
	closing     int32       // the splitstore is closing

	cfg  *Config
	path string

	mx          sync.Mutex
	warmupEpoch atomic.Int64
	baseEpoch   abi.ChainEpoch // protected by compaction lock
	pruneEpoch  abi.ChainEpoch // protected by compaction lock

	headChangeMx sync.Mutex

	chain ChainAccessor
	ds    dstore.Datastore
	cold  bstore.Blockstore
	hot   hotstore

	upgrades []upgradeRange

	markSetEnv  MarkSetEnv
	markSetSize int64

	compactionIndex int64
	pruneIndex      int64

	ctx    context.Context
	cancel func()

	outOfSync         int32 // for fast checking
	chainSyncMx       sync.Mutex
	chainSyncCond     sync.Cond
	chainSyncFinished bool // protected by chainSyncMx

	debug *debugLog

	// transactional protection for concurrent read/writes during compaction
	txnLk           sync.RWMutex
	txnViewsMx      sync.Mutex
	txnViewsCond    sync.Cond
	txnViews        int
	txnViewsWaiting bool
	txnActive       bool
	txnRefsMx       sync.Mutex
	txnRefs         map[cid.Cid]struct{}
	txnMissing      map[cid.Cid]struct{}
	txnMarkSet      MarkSet
	txnSyncMx       sync.Mutex
	txnSyncCond     sync.Cond
	txnSync         bool

	// background cold object reification
	reifyWorkers    sync.WaitGroup
	reifyMx         sync.Mutex
	reifyCond       sync.Cond
	reifyPend       map[cid.Cid]struct{}
	reifyInProgress map[cid.Cid]struct{}

	// registered protectors
	protectors []func(func(cid.Cid) error) error

	// dag sizes measured during latest compaction
	// logged and used for GC strategy

	// protected by compaction lock
	szWalk          int64
	szProtectedTxns int64
	szKeys          int64 // approximate, not counting keys protected when entering critical section

	// protected by txnLk
	szMarkedLiveRefs int64
}

var _ bstore.Blockstore = (*SplitStore)(nil)

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
		path:       path,
		ds:         ds,
		cold:       cold,
		hot:        hots,
		markSetEnv: markSetEnv,
	}

	ss.txnViewsCond.L = &ss.txnViewsMx
	ss.txnSyncCond.L = &ss.txnSyncMx
	ss.chainSyncCond.L = &ss.chainSyncMx

	baseCtx := context.Background()
	ctx := metrics.AddNetworkTag(baseCtx)

	ss.ctx, ss.cancel = context.WithCancel(ctx)

	ss.reifyCond.L = &ss.reifyMx
	ss.reifyPend = make(map[cid.Cid]struct{})
	ss.reifyInProgress = make(map[cid.Cid]struct{})

	if enableDebugLog {
		ss.debug, err = openDebugLog(path)
		if err != nil {
			return nil, err
		}
	}

	if ss.checkpointExists() {
		log.Info("found compaction checkpoint; resuming compaction")
		if err := ss.completeCompaction(); err != nil {
			_ = markSetEnv.Close()
			return nil, xerrors.Errorf("error resuming compaction: %w", err)
		}
	}
	if ss.pruneCheckpointExists() {
		log.Info("found prune checkpoint; resuming prune")
		if err := ss.completePrune(); err != nil {
			_ = markSetEnv.Close()
			return nil, xerrors.Errorf("error resuming prune: %w", err)
		}
	}

	return ss, nil
}

// Blockstore interface --------------------------------------------------------

func (s *SplitStore) DeleteBlock(_ context.Context, _ cid.Cid) error {
	// afaict we don't seem to be using this method, so it's not implemented
	return errors.New("DeleteBlock not implemented on SplitStore; don't do this Luke!") //nolint
}

func (s *SplitStore) DeleteMany(_ context.Context, _ []cid.Cid) error {
	// afaict we don't seem to be using this method, so it's not implemented
	return errors.New("DeleteMany not implemented on SplitStore; don't do this Luke!") //nolint
}

func (s *SplitStore) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	if isIdentiyCid(cid) {
		return true, nil
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	// critical section
	if s.txnMarkSet != nil {
		has, err := s.txnMarkSet.Has(cid)
		if err != nil {
			return false, err
		}

		if has {
			return s.has(cid)
		}
		switch s.compactType {
		case hot:
			return s.cold.Has(ctx, cid)
		case cold:
			return s.hot.Has(ctx, cid)
		default:
			return false, xerrors.Errorf("invalid compaction type %d, only hot and cold allowed for critical section", s.compactType)
		}
	}

	has, err := s.hot.Has(ctx, cid)

	if err != nil {
		return has, err
	}

	if has {
		s.trackTxnRef(cid)
		return true, nil
	}

	has, err = s.cold.Has(ctx, cid)
	if has {
		s.trackTxnRef(cid)
		if bstore.IsHotView(ctx) {
			s.reifyColdObject(cid)
		}
	}

	return has, err

}

func (s *SplitStore) Get(ctx context.Context, cid cid.Cid) (blocks.Block, error) {
	if isIdentiyCid(cid) {
		data, err := decodeIdentityCid(cid)
		if err != nil {
			return nil, err
		}

		return blocks.NewBlockWithCid(data, cid)
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	// critical section
	if s.txnMarkSet != nil {
		has, err := s.txnMarkSet.Has(cid)
		if err != nil {
			return nil, err
		}

		if has {
			return s.get(cid)
		}
		switch s.compactType {
		case hot:
			return s.cold.Get(ctx, cid)
		case cold:
			return s.hot.Get(ctx, cid)
		default:
			return nil, xerrors.Errorf("invalid compaction type %d, only hot and cold allowed for critical section", s.compactType)
		}
	}

	blk, err := s.hot.Get(ctx, cid)

	switch {
	case err == nil:
		s.trackTxnRef(cid)
		return blk, nil

	case ipld.IsNotFound(err):
		if s.isWarm() {
			s.debug.LogReadMiss(cid)
		}

		blk, err = s.cold.Get(ctx, cid)
		if err == nil {
			s.trackTxnRef(cid)
			if bstore.IsHotView(ctx) {
				s.reifyColdObject(cid)
			}

			stats.Record(s.ctx, metrics.SplitstoreMiss.M(1))
		}
		return blk, err

	default:
		return nil, err
	}
}

func (s *SplitStore) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	if isIdentiyCid(cid) {
		data, err := decodeIdentityCid(cid)
		if err != nil {
			return 0, err
		}

		return len(data), nil
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	// critical section
	if s.txnMarkSet != nil {
		has, err := s.txnMarkSet.Has(cid)
		if err != nil {
			return 0, err
		}

		if has {
			return s.getSize(cid)
		}
		switch s.compactType {
		case hot:
			return s.cold.GetSize(ctx, cid)
		case cold:
			return s.hot.GetSize(ctx, cid)
		default:
			return 0, xerrors.Errorf("invalid compaction type %d, only hot and cold allowed for critical section", s.compactType)
		}
	}

	size, err := s.hot.GetSize(ctx, cid)

	switch {
	case err == nil:
		s.trackTxnRef(cid)
		return size, nil

	case ipld.IsNotFound(err):
		if s.isWarm() {
			s.debug.LogReadMiss(cid)
		}

		size, err = s.cold.GetSize(ctx, cid)
		if err == nil {
			s.trackTxnRef(cid)
			if bstore.IsHotView(ctx) {
				s.reifyColdObject(cid)
			}

			stats.Record(s.ctx, metrics.SplitstoreMiss.M(1))
		}
		return size, err

	default:
		return 0, err
	}
}

func (s *SplitStore) Flush(ctx context.Context) error {
	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	if err := s.cold.Flush(ctx); err != nil {
		return err
	}
	if err := s.hot.Flush(ctx); err != nil {
		return err
	}
	if err := s.ds.Sync(ctx, dstore.Key{}); err != nil {
		return err
	}

	return nil
}

func (s *SplitStore) Put(ctx context.Context, blk blocks.Block) error {
	if isIdentiyCid(blk.Cid()) {
		return nil
	}

	s.txnLk.RLock()
	defer s.txnLk.RUnlock()

	err := s.hot.Put(ctx, blk)
	if err != nil {
		return err
	}

	s.debug.LogWrite(blk)

	// critical section
	if s.txnMarkSet != nil && s.compactType == hot { // puts only touch hot store
		s.markLiveRefs([]cid.Cid{blk.Cid()})
		return nil
	}
	s.trackTxnRef(blk.Cid())

	return nil
}

func (s *SplitStore) PutMany(ctx context.Context, blks []blocks.Block) error {
	// filter identities
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

	err := s.hot.PutMany(ctx, blks)
	if err != nil {
		return err
	}

	s.debug.LogWriteMany(blks)

	// critical section
	if s.txnMarkSet != nil && s.compactType == hot { // puts only touch hot store
		s.markLiveRefs(batch)
		return nil
	}
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

func (s *SplitStore) View(ctx context.Context, cid cid.Cid, cb func([]byte) error) error {
	if isIdentiyCid(cid) {
		data, err := decodeIdentityCid(cid)
		if err != nil {
			return err
		}

		return cb(data)
	}

	// critical section
	s.txnLk.RLock() // the lock is released in protectView if we are not in critical section
	if s.txnMarkSet != nil {
		has, err := s.txnMarkSet.Has(cid)
		s.txnLk.RUnlock()

		if err != nil {
			return err
		}

		if has {
			return s.view(cid, cb)
		}
		switch s.compactType {
		case hot:
			return s.cold.View(ctx, cid, cb)
		case cold:
			return s.hot.View(ctx, cid, cb)
		default:
			return xerrors.Errorf("invalid compaction type %d, only hot and cold allowed for critical section", s.compactType)
		}
	}

	// views are (optimistically) protected two-fold:
	// - if there is an active transaction, then the reference is protected.
	// - if there is no active transaction, active views are tracked in a
	//   wait group and compaction is inhibited from starting until they
	//   have all completed.  this is necessary to ensure that a (very) long-running
	//   view can't have its data pointer deleted, which would be catastrophic.
	//   Note that we can't just RLock for the duration of the view, as this could
	//    lead to deadlock with recursive views.
	s.protectView(cid)
	defer s.viewDone()

	err := s.hot.View(ctx, cid, cb)
	if ipld.IsNotFound(err) {
		if s.isWarm() {
			s.debug.LogReadMiss(cid)
		}

		err = s.cold.View(ctx, cid, cb)
		if err == nil {
			if bstore.IsHotView(ctx) {
				s.reifyColdObject(cid)
			}

			stats.Record(s.ctx, metrics.SplitstoreMiss.M(1))
		}
		return err
	}
	return err
}

func (s *SplitStore) isWarm() bool {
	return s.warmupEpoch.Load() > 0
}

// Start state tracking
func (s *SplitStore) Start(chain ChainAccessor, us stmgr.UpgradeSchedule) error {
	s.chain = chain
	curTs := chain.GetHeaviestTipSet()

	// precompute the upgrade boundaries
	s.upgrades = make([]upgradeRange, 0, len(us))
	for _, upgrade := range us {
		boundary := upgrade.Height
		for _, pre := range upgrade.PreMigrations {
			preMigrationBoundary := upgrade.Height - pre.StartWithin
			if preMigrationBoundary < boundary {
				boundary = preMigrationBoundary
			}
		}

		upgradeStart := boundary - upgradeBoundary
		upgradeEnd := upgrade.Height + upgradeBoundary

		s.upgrades = append(s.upgrades, upgradeRange{start: upgradeStart, end: upgradeEnd})
	}

	// should we warmup
	warmup := false

	// load base epoch from metadata ds
	// if none, then use current epoch because it's a fresh start
	bs, err := s.ds.Get(s.ctx, baseEpochKey)
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

	// load prune epoch from metadata ds
	bs, err = s.ds.Get(s.ctx, pruneEpochKey)
	switch err {
	case nil:
		s.pruneEpoch = bytesToEpoch(bs)
	case dstore.ErrNotFound:
		if curTs == nil {
			//this can happen in some tests
			break
		}
		if err := s.setPruneEpoch(curTs.Height()); err != nil {
			return xerrors.Errorf("error saving prune epoch: %w", err)
		}
	default:
		return xerrors.Errorf("error loading prune epoch: %w", err)
	}

	// load warmup epoch from metadata ds
	bs, err = s.ds.Get(s.ctx, warmupEpochKey)
	switch err {
	case nil:
		s.warmupEpoch.Store(bytesToInt64(bs))

	case dstore.ErrNotFound:
		warmup = true

	default:
		return xerrors.Errorf("error loading warmup epoch: %w", err)
	}

	// load markSetSize from metadata ds to provide a size hint for marksets
	bs, err = s.ds.Get(s.ctx, markSetSizeKey)
	switch err {
	case nil:
		s.markSetSize = bytesToInt64(bs)

	case dstore.ErrNotFound:
	default:
		return xerrors.Errorf("error loading mark set size: %w", err)
	}

	// load compactionIndex from metadata ds to provide a hint as to when to perform moving gc
	bs, err = s.ds.Get(s.ctx, compactionIndexKey)
	switch err {
	case nil:
		s.compactionIndex = bytesToInt64(bs)

	case dstore.ErrNotFound:
		// this is potentially an upgrade from splitstore v0; schedule a warmup as v0 has
		// some issues with hot references leaking into the coldstore.
		warmup = true
	default:
		return xerrors.Errorf("error loading compaction index: %w", err)
	}

	log.Infow("starting splitstore", "baseEpoch", s.baseEpoch, "warmupEpoch", s.warmupEpoch.Load())

	if warmup {
		err = s.warmup(curTs)
		if err != nil {
			return xerrors.Errorf("error starting warmup: %w", err)
		}
	}

	// spawn the reifier
	go s.reifyOrchestrator()

	// watch the chain
	chain.SubscribeHeadChanges(s.HeadChange)

	return nil
}

func (s *SplitStore) AddProtector(protector func(func(cid.Cid) error) error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	s.protectors = append(s.protectors, protector)
}

func (s *SplitStore) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closing, 0, 1) {
		// already closing
		return nil
	}

	if atomic.LoadInt32(&s.compacting) == 1 {
		s.txnSyncMx.Lock()
		s.txnSync = true
		s.txnSyncCond.Broadcast()
		s.txnSyncMx.Unlock()

		s.chainSyncMx.Lock()
		s.chainSyncFinished = true
		s.chainSyncCond.Broadcast()
		s.chainSyncMx.Unlock()

		log.Warn("close with ongoing compaction in progress; waiting for it to finish...")
		for atomic.LoadInt32(&s.compacting) == 1 {
			time.Sleep(time.Second)
		}
	}

	s.reifyCond.Broadcast()
	s.reifyWorkers.Wait()
	s.cancel()
	return multierr.Combine(s.markSetEnv.Close(), s.debug.Close())
}

func (s *SplitStore) checkClosing() error {
	if atomic.LoadInt32(&s.closing) == 1 {
		return xerrors.Errorf("splitstore is closing")
	}

	return nil
}

func (s *SplitStore) setBaseEpoch(epoch abi.ChainEpoch) error {
	s.baseEpoch = epoch
	return s.ds.Put(s.ctx, baseEpochKey, epochToBytes(epoch))
}

func (s *SplitStore) setPruneEpoch(epoch abi.ChainEpoch) error {
	s.pruneEpoch = epoch
	return s.ds.Put(s.ctx, pruneEpochKey, epochToBytes(epoch))
}
