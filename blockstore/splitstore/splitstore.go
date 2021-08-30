package splitstore

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-state-types/abi"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"

	"go.opencensus.io/stats"
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

	log = logging.Logger("splitstore")

	// set this to true if you are debugging the splitstore to enable debug logging
	enableDebugLog = false
	// set this to true if you want to track origin stack traces in the write log
	enableDebugLogWriteTraces = false
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

	// HotstoreMessageRetention indicates the hotstore retention policy for messages.
	// It has the following semantics:
	// - a value of 0 will only retain messages within the compaction boundary (4 finalities)
	// - a positive integer indicates the number of finalities, outside the compaction boundary,
	//   for which messages will be retained in the hotstore.
	HotStoreMessageRetention uint64

	// HotstoreFullGCFrequency indicates how frequently (in terms of compactions) to garbage collect
	// the hotstore using full (moving) GC if supported by the hotstore.
	// A value of 0 disables full GC entirely.
	// A positive value is the number of compactions before a full GC is performed;
	// a value of 1 will perform full GC in every compaction.
	HotStoreFullGCFrequency uint64
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
	compacting int32 // compaction/prune/warmup in progress
	closing    int32 // the splitstore is closing

	cfg  *Config
	path string

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

	compactionIndex int64

	ctx    context.Context
	cancel func()

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

	// registered protectors
	protectors []func(func(cid.Cid) error) error
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

	if !markSetEnv.SupportsVisitor() {
		return nil, xerrors.Errorf("markset type does not support atomic visitors")
	}

	// and now we can make a SplitStore
	ss := &SplitStore{
		cfg:        cfg,
		path:       path,
		ds:         ds,
		cold:       cold,
		hot:        hots,
		markSetEnv: markSetEnv,

		coldPurgeSize: defaultColdPurgeSize,
	}

	ss.txnViewsCond.L = &ss.txnViewsMx
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
	s.protectView(cid)
	defer s.viewDone()

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

	// should we warmup
	warmup := false

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
		warmup = true

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

	// load compactionIndex from metadata ds to provide a hint as to when to perform moving gc
	bs, err = s.ds.Get(compactionIndexKey)
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

	log.Infow("starting splitstore", "baseEpoch", s.baseEpoch, "warmupEpoch", s.warmupEpoch)

	if warmup {
		err = s.warmup(curTs)
		if err != nil {
			return xerrors.Errorf("error starting warmup: %w", err)
		}
	}

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
		log.Warn("close with ongoing compaction in progress; waiting for it to finish...")
		for atomic.LoadInt32(&s.compacting) == 1 {
			time.Sleep(time.Second)
		}
	}

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
	return s.ds.Put(baseEpochKey, epochToBytes(epoch))
}
