package dagstore

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	levelds "github.com/ipfs/go-ds-leveldb"
	measure "github.com/ipfs/go-ds-measure"
	logging "github.com/ipfs/go-log/v2"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/config"

	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/dagstore/index"
	"github.com/filecoin-project/dagstore/mount"
	"github.com/filecoin-project/dagstore/shard"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/stores"
)

const (
	maxRecoverAttempts = 1
	shardRegMarker     = ".shard-registration-complete"
)

var log = logging.Logger("dagstore")

type Wrapper struct {
	ctx          context.Context
	cancel       context.CancelFunc
	backgroundWg sync.WaitGroup

	cfg        config.DAGStoreConfig
	dagst      dagstore.Interface
	minerAPI   MinerAPI
	failureCh  chan dagstore.ShardResult
	traceCh    chan dagstore.Trace
	gcInterval time.Duration
}

var _ stores.DAGStoreWrapper = (*Wrapper)(nil)

func NewDAGStore(cfg config.DAGStoreConfig, minerApi MinerAPI) (*dagstore.DAGStore, *Wrapper, error) {
	// construct the DAG Store.
	registry := mount.NewRegistry()
	if err := registry.Register(lotusScheme, mountTemplate(minerApi)); err != nil {
		return nil, nil, xerrors.Errorf("failed to create registry: %w", err)
	}

	// The dagstore will write Shard failures to the `failureCh` here.
	failureCh := make(chan dagstore.ShardResult, 1)

	// The dagstore will write Trace events to the `traceCh` here.
	traceCh := make(chan dagstore.Trace, 32)

	var (
		transientsDir = filepath.Join(cfg.RootDir, "transients")
		datastoreDir  = filepath.Join(cfg.RootDir, "datastore")
		indexDir      = filepath.Join(cfg.RootDir, "index")
	)

	dstore, err := newDatastore(datastoreDir)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to create dagstore datastore in %s: %w", datastoreDir, err)
	}

	irepo, err := index.NewFSRepo(indexDir)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to initialise dagstore index repo")
	}

	dcfg := dagstore.Config{
		TransientsDir: transientsDir,
		IndexRepo:     irepo,
		Datastore:     dstore,
		MountRegistry: registry,
		FailureCh:     failureCh,
		TraceCh:       traceCh,
		// not limiting fetches globally, as the Lotus mount does
		// conditional throttling.
		MaxConcurrentIndex:        cfg.MaxConcurrentIndex,
		MaxConcurrentReadyFetches: cfg.MaxConcurrentReadyFetches,
		RecoverOnStart:            dagstore.RecoverOnAcquire,
	}

	dagst, err := dagstore.NewDAGStore(dcfg)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to create DAG store: %w", err)
	}

	w := &Wrapper{
		cfg:        cfg,
		dagst:      dagst,
		minerAPI:   minerApi,
		failureCh:  failureCh,
		traceCh:    traceCh,
		gcInterval: time.Duration(cfg.GCInterval),
	}

	return dagst, w, nil
}

// newDatastore creates a datastore under the given base directory
// for dagstore metadata.
func newDatastore(dir string) (ds.Batching, error) {
	// Create the datastore directory if it doesn't exist yet.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, xerrors.Errorf("failed to create directory %s for DAG store datastore: %w", dir, err)
	}

	// Create a new LevelDB datastore
	dstore, err := levelds.NewDatastore(dir, &levelds.Options{
		Compression: ldbopts.NoCompression,
		NoSync:      false,
		Strict:      ldbopts.StrictAll,
		ReadOnly:    false,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to open datastore for DAG store: %w", err)
	}
	// Keep statistics about the datastore
	mds := measure.New("measure.", dstore)
	return mds, nil
}

func (w *Wrapper) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	// Run a go-routine to do DagStore GC.
	w.backgroundWg.Add(1)
	go w.gcLoop()

	// run a go-routine to read the trace for debugging.
	w.backgroundWg.Add(1)
	go w.traceLoop()

	// Run a go-routine for shard recovery
	if dss, ok := w.dagst.(*dagstore.DAGStore); ok {
		w.backgroundWg.Add(1)
		go dagstore.RecoverImmediately(w.ctx, dss, w.failureCh, maxRecoverAttempts, w.backgroundWg.Done)
	}

	return w.dagst.Start(ctx)
}

func (w *Wrapper) traceLoop() {
	defer w.backgroundWg.Done()

	for w.ctx.Err() == nil {
		select {
		// Log trace events from the DAG store
		case tr := <-w.traceCh:
			log.Debugw("trace",
				"shard-key", tr.Key.String(),
				"op-type", tr.Op.String(),
				"after", tr.After.String())

		case <-w.ctx.Done():
			return
		}
	}
}

func (w *Wrapper) gcLoop() {
	defer w.backgroundWg.Done()

	ticker := time.NewTicker(w.gcInterval)
	defer ticker.Stop()

	for w.ctx.Err() == nil {
		select {
		// GC the DAG store on every tick
		case <-ticker.C:
			_, _ = w.dagst.GC(w.ctx)

		// Exit when the DAG store wrapper is shutdown
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *Wrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (stores.ClosableBlockstore, error) {
	log.Debugf("acquiring shard for piece CID %s", pieceCid)

	key := shard.KeyFromCID(pieceCid)
	resch := make(chan dagstore.ShardResult, 1)
	err := w.dagst.AcquireShard(ctx, key, resch, dagstore.AcquireOpts{})
	log.Debugf("sent message to acquire shard for piece CID %s", pieceCid)

	if err != nil {
		if !errors.Is(err, dagstore.ErrShardUnknown) {
			return nil, xerrors.Errorf("failed to schedule acquire shard for piece CID %s: %w", pieceCid, err)
		}

		// if the DAGStore does not know about the Shard -> register it and then try to acquire it again.
		log.Warnw("failed to load shard as shard is not registered, will re-register", "pieceCID", pieceCid)
		// The path of a transient file that we can ask the DAG Store to use
		// to perform the Indexing rather than fetching it via the Mount if
		// we already have a transient file. However, we don't have it here
		// and therefore we pass an empty file path.
		carPath := ""
		if err := stores.RegisterShardSync(ctx, w, pieceCid, carPath, false); err != nil {
			return nil, xerrors.Errorf("failed to re-register shard during loading piece CID %s: %w", pieceCid, err)
		}
		log.Warnw("successfully re-registered shard", "pieceCID", pieceCid)

		resch = make(chan dagstore.ShardResult, 1)
		if err := w.dagst.AcquireShard(ctx, key, resch, dagstore.AcquireOpts{}); err != nil {
			return nil, xerrors.Errorf("failed to acquire Shard for piece CID %s after re-registering: %w", pieceCid, err)
		}
	}

	// TODO: The context is not yet being actively monitored by the DAG store,
	// so we need to select against ctx.Done() until the following issue is
	// implemented:
	// https://github.com/filecoin-project/dagstore/issues/39
	var res dagstore.ShardResult
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res = <-resch:
		if res.Error != nil {
			return nil, xerrors.Errorf("failed to acquire shard for piece CID %s: %w", pieceCid, res.Error)
		}
	}

	bs, err := res.Accessor.Blockstore()
	if err != nil {
		return nil, err
	}

	log.Debugf("successfully loaded blockstore for piece CID %s", pieceCid)
	return &Blockstore{ReadBlockstore: bs, Closer: res.Accessor}, nil
}

func (w *Wrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool, resch chan dagstore.ShardResult) error {
	// Create a lotus mount with the piece CID
	key := shard.KeyFromCID(pieceCid)
	mt, err := NewLotusMount(pieceCid, w.minerAPI)
	if err != nil {
		return xerrors.Errorf("failed to create lotus mount for piece CID %s: %w", pieceCid, err)
	}

	// Register the shard
	opts := dagstore.RegisterOpts{
		ExistingTransient:  carPath,
		LazyInitialization: !eagerInit,
	}
	err = w.dagst.RegisterShard(ctx, key, mt, resch, opts)
	if err != nil {
		return xerrors.Errorf("failed to schedule register shard for piece CID %s: %w", pieceCid, err)
	}
	log.Debugf("successfully submitted Register Shard request for piece CID %s with eagerInit=%t", pieceCid, eagerInit)

	return nil
}

func (w *Wrapper) MigrateDeals(ctx context.Context, deals []storagemarket.MinerDeal) (bool, error) {
	log := log.Named("migrator")

	// Check if all deals have already been registered as shards
	isComplete, err := w.registrationComplete()
	if err != nil {
		return false, xerrors.Errorf("failed to get dagstore migration status: %w", err)
	}
	if isComplete {
		// All deals have been registered as shards, bail out
		log.Info("no shard migration necessary; already marked complete")
		return false, nil
	}

	log.Infow("registering shards for all active deals in sealing subsystem", "count", len(deals))

	inSealingSubsystem := make(map[fsm.StateKey]struct{}, len(providerstates.StatesKnownBySealingSubsystem))
	for _, s := range providerstates.StatesKnownBySealingSubsystem {
		inSealingSubsystem[s] = struct{}{}
	}

	// channel where results will be received, and channel where the total
	// number of registered shards will be sent.
	resch := make(chan dagstore.ShardResult, 32)
	totalCh := make(chan int)
	doneCh := make(chan struct{})

	// Start making progress consuming results. We won't know how many to
	// actually consume until we register all shards.
	//
	// If there are any problems registering shards, just log an error
	go func() {
		defer close(doneCh)

		var total = math.MaxInt64
		var res dagstore.ShardResult
		for rcvd := 0; rcvd < total; {
			select {
			case total = <-totalCh:
				// we now know the total number of registered shards
				// nullify so that we no longer consume from it after closed.
				close(totalCh)
				totalCh = nil
			case res = <-resch:
				rcvd++
				if res.Error == nil {
					log.Infow("async shard registration completed successfully", "shard_key", res.Key)
				} else {
					log.Warnw("async shard registration failed", "shard_key", res.Key, "error", res.Error)
				}
			}
		}
	}()

	// Filter for deals that are handed off.
	//
	// If the deal has not yet been handed off to the sealing subsystem, we
	// don't need to call RegisterShard in this migration; RegisterShard will
	// be called in the new code once the deal reaches the state where it's
	// handed off to the sealing subsystem.
	var registered int
	for _, deal := range deals {
		pieceCid := deal.Proposal.PieceCID

		// enrich log statements in this iteration with deal ID and piece CID.
		log := log.With("deal_id", deal.DealID, "piece_cid", pieceCid)

		// Filter for deals that have been handed off to the sealing subsystem
		if _, ok := inSealingSubsystem[deal.State]; !ok {
			log.Infow("deal not ready; skipping")
			continue
		}

		log.Infow("registering deal in dagstore with lazy init")

		// Register the deal as a shard with the DAG store with lazy initialization.
		// The index will be populated the first time the deal is retrieved, or
		// through the bulk initialization script.
		err = w.RegisterShard(ctx, pieceCid, "", false, resch)
		if err != nil {
			log.Warnw("failed to register shard", "error", err)
			continue
		}
		registered++
	}

	log.Infow("finished registering all shards", "total", registered)
	totalCh <- registered
	<-doneCh

	log.Infow("confirmed registration of all shards")

	// Completed registering all shards, so mark the migration as complete
	err = w.markRegistrationComplete()
	if err != nil {
		log.Errorf("failed to mark shards as registered: %s", err)
	} else {
		log.Info("successfully marked migration as complete")
	}

	log.Infow("dagstore migration complete")

	return true, nil
}

// Check for the existence of a "marker" file indicating that the migration
// has completed
func (w *Wrapper) registrationComplete() (bool, error) {
	path := filepath.Join(w.cfg.RootDir, shardRegMarker)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Create a "marker" file indicating that the migration has completed
func (w *Wrapper) markRegistrationComplete() error {
	path := filepath.Join(w.cfg.RootDir, shardRegMarker)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	return file.Close()
}

func (w *Wrapper) Close() error {
	// Cancel the context
	w.cancel()

	// Close the DAG store
	log.Info("will close the dagstore")
	if err := w.dagst.Close(); err != nil {
		return xerrors.Errorf("failed to close dagstore: %w", err)
	}
	log.Info("dagstore closed")

	// Wait for the background go routine to exit
	log.Info("waiting for dagstore background wrapper goroutines to exit")
	w.backgroundWg.Wait()
	log.Info("exited dagstore background wrapper goroutines")

	return nil
}
