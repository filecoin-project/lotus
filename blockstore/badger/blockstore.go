package badgerbs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	badgerstruct "github.com/dgraph-io/badger/v2/pb"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logger "github.com/ipfs/go-log/v2"
	pool "github.com/libp2p/go-buffer-pool"
	"github.com/multiformats/go-base32"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
)

var (
	// KeyPool is the buffer pool we use to compute storage keys.
	KeyPool *pool.BufferPool = pool.GlobalPool
)

var (
	// ErrBlockstoreClosed is returned from blockstore operations after
	// the blockstore has been closed.
	ErrBlockstoreClosed = fmt.Errorf("badger blockstore closed")

	log = logger.Logger("badgerbs")
)

// aliases to mask badger dependencies.
const (
	// FileIO is equivalent to badger/options.FileIO.
	FileIO = options.FileIO
	// MemoryMap is equivalent to badger/options.MemoryMap.
	MemoryMap = options.MemoryMap
	// LoadToRAM is equivalent to badger/options.LoadToRAM.
	LoadToRAM          = options.LoadToRAM
	defaultGCThreshold = 0.125
)

// Options embeds the badger options themselves, and augments them with
// blockstore-specific options.
type Options struct {
	badger.Options

	// Prefix is an optional prefix to prepend to keys. Default: "".
	Prefix string
}

func DefaultOptions(path string) Options {
	return Options{
		Options: badger.DefaultOptions(path),
		Prefix:  "",
	}
}

// badgerLogger is a local wrapper for go-log to make the interface
// compatible with badger.Logger (namely, aliasing Warnf to Warningf)
type badgerLogger struct {
	*zap.SugaredLogger // skips 1 caller to get useful line info, skipping over badger.Options.

	skip2 *zap.SugaredLogger // skips 2 callers, just like above + this logger.
}

// Warningf is required by the badger logger APIs.
func (b *badgerLogger) Warningf(format string, args ...interface{}) {
	b.skip2.Warnf(format, args...)
}

// bsState is the current blockstore state
type bsState int

const (
	// stateOpen signifies an open blockstore
	stateOpen bsState = iota
	// stateClosing signifies a blockstore that is currently closing
	stateClosing
	// stateClosed signifies a blockstore that has been colosed
	stateClosed
)

type bsMoveState int

const (
	// moveStateNone signifies that there is no move in progress
	moveStateNone bsMoveState = iota
	// moveStateMoving signifies that there is a move in a progress
	moveStateMoving
	// moveStateCleanup signifies that a move has completed or aborted and we are cleaning up
	moveStateCleanup
	// moveStateLock signifies that an exclusive lock has been acquired
	moveStateLock
)

// Blockstore is a badger-backed IPLD blockstore.
type Blockstore struct {
	stateLk sync.RWMutex
	state   bsState
	viewers sync.WaitGroup

	moveMx    sync.Mutex
	moveCond  sync.Cond
	moveState bsMoveState
	rlock     int

	db     *badger.DB
	dbNext *badger.DB // when moving
	opts   Options

	prefixing bool
	prefix    []byte
	prefixLen int
}

var _ blockstore.Blockstore = (*Blockstore)(nil)
var _ blockstore.Viewer = (*Blockstore)(nil)
var _ blockstore.BlockstoreIterator = (*Blockstore)(nil)
var _ blockstore.BlockstoreGC = (*Blockstore)(nil)
var _ blockstore.BlockstoreSize = (*Blockstore)(nil)
var _ io.Closer = (*Blockstore)(nil)

// Open creates a new badger-backed blockstore, with the supplied options.
func Open(opts Options) (*Blockstore, error) {
	opts.Logger = &badgerLogger{
		SugaredLogger: log.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar(),
		skip2:         log.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
	}

	db, err := badger.Open(opts.Options)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger blockstore: %w", err)
	}

	bs := &Blockstore{db: db, opts: opts}
	if p := opts.Prefix; p != "" {
		bs.prefixing = true
		bs.prefix = []byte(p)
		bs.prefixLen = len(bs.prefix)
	}

	bs.moveCond.L = &bs.moveMx

	return bs, nil
}

// Close closes the store. If the store has already been closed, this noops and
// returns an error, even if the first closure resulted in error.
func (b *Blockstore) Close() error {
	b.stateLk.Lock()
	if b.state != stateOpen {
		b.stateLk.Unlock()
		return nil
	}
	b.state = stateClosing
	b.stateLk.Unlock()

	defer func() {
		b.stateLk.Lock()
		b.state = stateClosed
		b.stateLk.Unlock()
	}()

	// wait for all accesses to complete
	b.viewers.Wait()

	return b.db.Close()
}

func (b *Blockstore) access() error {
	b.stateLk.RLock()
	defer b.stateLk.RUnlock()

	if b.state != stateOpen {
		return ErrBlockstoreClosed
	}

	b.viewers.Add(1)
	return nil
}

func (b *Blockstore) isOpen() bool {
	b.stateLk.RLock()
	defer b.stateLk.RUnlock()

	return b.state == stateOpen
}

// lockDB/unlockDB implement a recursive lock contingent on move state
func (b *Blockstore) lockDB() {
	b.moveMx.Lock()
	defer b.moveMx.Unlock()

	if b.rlock == 0 {
		for b.moveState == moveStateLock {
			b.moveCond.Wait()
		}
	}

	b.rlock++
}

func (b *Blockstore) unlockDB() {
	b.moveMx.Lock()
	defer b.moveMx.Unlock()

	b.rlock--
	if b.rlock == 0 && b.moveState == moveStateLock {
		b.moveCond.Broadcast()
	}
}

// lockMove/unlockMove implement an exclusive lock of move state
func (b *Blockstore) lockMove() {
	b.moveMx.Lock()
	b.moveState = moveStateLock
	for b.rlock > 0 {
		b.moveCond.Wait()
	}
}

func (b *Blockstore) unlockMove(state bsMoveState) {
	b.moveState = state
	b.moveCond.Broadcast()
	b.moveMx.Unlock()
}

// movingGC moves the blockstore to a new path, adjacent to the current path, and creates
// a symlink from the current path to the new path; the old blockstore is deleted.
//
// The blockstore MUST accept new writes during the move and ensure that these
// are persisted to the new blockstore; if a failure occurs aboring the move,
// then they must be persisted to the old blockstore.
// In short, the blockstore must not lose data from new writes during the move.
func (b *Blockstore) movingGC(ctx context.Context) error {
	// this inlines moveLock/moveUnlock for the initial state check to prevent a second move
	// while one is in progress without clobbering state
	b.moveMx.Lock()
	if b.moveState != moveStateNone {
		b.moveMx.Unlock()
		return fmt.Errorf("move in progress")
	}

	b.moveState = moveStateLock
	for b.rlock > 0 {
		b.moveCond.Wait()
	}

	b.moveState = moveStateMoving
	b.moveCond.Broadcast()
	b.moveMx.Unlock()

	var newPath string

	defer func() {
		b.lockMove()

		dbNext := b.dbNext
		b.dbNext = nil

		var state bsMoveState
		if dbNext != nil {
			state = moveStateCleanup
		} else {
			state = moveStateNone
		}

		b.unlockMove(state)

		if dbNext != nil {
			// the move failed and we have a left-over db; delete it.
			err := dbNext.Close()
			if err != nil {
				log.Warnf("error closing badger db: %s", err)
			}
			b.deleteDB(newPath)

			b.lockMove()
			b.unlockMove(moveStateNone)
		}
	}()

	// we resolve symlinks to create the new path in the adjacent to the old path.
	// this allows the user to symlink the db directory into a separate filesystem.
	basePath := b.opts.Dir
	linkPath, err := filepath.EvalSymlinks(basePath)
	if err != nil {
		return fmt.Errorf("error resolving symlink %s: %w", basePath, err)
	}

	if basePath == linkPath {
		newPath = basePath
	} else {
		// we do this dance to create a name adjacent to the current one, while avoiding clown
		// shoes with multiple moves (i.e. we can't just take the basename of the linkPath, as it
		// could have been created in a previous move and have the timestamp suffix, which would then
		// perpetuate itself.
		name := filepath.Base(basePath)
		dir := filepath.Dir(linkPath)
		newPath = filepath.Join(dir, name)
	}
	newPath = fmt.Sprintf("%s.%d", newPath, time.Now().UnixNano())

	log.Infof("moving blockstore from %s to %s", b.opts.Dir, newPath)

	opts := b.opts
	opts.Dir = newPath
	opts.ValueDir = newPath

	dbNew, err := badger.Open(opts.Options)
	if err != nil {
		return fmt.Errorf("failed to open badger blockstore in %s: %w", newPath, err)
	}

	b.lockMove()
	b.dbNext = dbNew
	b.unlockMove(moveStateMoving)

	log.Info("copying blockstore")
	err = b.doCopy(ctx, b.db, b.dbNext)
	if err != nil {
		return fmt.Errorf("error moving badger blockstore to %s: %w", newPath, err)
	}

	b.lockMove()
	dbOld := b.db
	b.db = b.dbNext
	b.dbNext = nil
	b.unlockMove(moveStateCleanup)

	err = dbOld.Close()
	if err != nil {
		log.Warnf("error closing old badger db: %s", err)
	}

	// this is the canonical db path; this is where our db lives.
	dbPath := b.opts.Dir

	// we first move the existing db out of the way, and only delete it after we have symlinked the
	// new db to the canonical path
	backupPath := fmt.Sprintf("%s.old.%d", dbPath, time.Now().Unix())
	if err = os.Rename(dbPath, backupPath); err != nil {
		// this is not catastrophic in the sense that we have not lost any data.
		// but it is pretty bad, as the db path points to the old db, while we are now using to the new
		// db; we can't continue and leave a ticking bomb for the next restart.
		// so a panic is appropriate and user can fix.
		panic(fmt.Errorf("error renaming old badger db dir from %s to %s: %w; USER ACTION REQUIRED", dbPath, backupPath, err)) //nolint
	}

	if err = symlink(newPath, dbPath); err != nil {
		// same here; the db path is pointing to the void. panic and let the user fix.
		panic(fmt.Errorf("error symlinking new badger db dir from %s to %s: %w; USER ACTION REQUIRED", newPath, dbPath, err)) //nolint
	}

	// the delete follows symlinks
	b.deleteDB(backupPath)

	log.Info("moving blockstore done")
	return nil
}

// symlink creates a symlink from path to linkTo; the link is relative if the two are
// in the same directory
func symlink(path, linkTo string) error {
	resolvedPathDir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("error resolving links in %s: %w", path, err)
	}

	resolvedLinkDir, err := filepath.EvalSymlinks(filepath.Dir(linkTo))
	if err != nil {
		return fmt.Errorf("error resolving links in %s: %w", linkTo, err)
	}

	if resolvedPathDir == resolvedLinkDir {
		path = filepath.Base(path)
	}

	return os.Symlink(path, linkTo)
}

// doCopy copies a badger blockstore to another
func (b *Blockstore) doCopy(ctx context.Context, from, to *badger.DB) (defErr error) {
	batch := to.NewWriteBatch()
	defer func() {
		if defErr == nil {
			defErr = batch.Flush()
		}
		if defErr != nil {
			batch.Cancel()
		}
	}()

	return iterateBadger(ctx, from, func(kvs []*badgerstruct.KV) error {
		// check whether context is closed on every kv group
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, kv := range kvs {
			if err := batch.Set(kv.Key, kv.Value); err != nil {
				return err
			}
		}
		return nil
	})
}

var IterateLSMWorkers int // defaults to between( 2, 8, runtime.NumCPU/2 )

func iterateBadger(ctx context.Context, db *badger.DB, iter func([]*badgerstruct.KV) error) error {
	workers := IterateLSMWorkers
	if workers == 0 {
		workers = between(2, 8, runtime.NumCPU()/2)
	}

	stream := db.NewStream()
	stream.NumGo = workers
	stream.LogPrefix = "iterateBadgerKVs"
	stream.Send = func(kvl *badgerstruct.KVList) error {
		kvs := make([]*badgerstruct.KV, 0, len(kvl.Kv))
		for _, kv := range kvl.Kv {
			if kv.Key != nil && kv.Value != nil {
				kvs = append(kvs, kv)
			}
		}
		if len(kvs) == 0 {
			return nil
		}
		return iter(kvs)
	}
	return stream.Orchestrate(ctx)
}

func between(min, max, val int) int {
	if val > max {
		val = max
	}
	if val < min {
		val = min
	}
	return val
}

func (b *Blockstore) deleteDB(path string) {
	// follow symbolic links, otherwise the data will be left behind
	linkPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		log.Warnf("error resolving symlinks in %s", path)
		return
	}

	log.Infof("removing data directory %s", linkPath)
	if err := os.RemoveAll(linkPath); err != nil {
		log.Warnf("error deleting db at %s: %s", linkPath, err)
		return
	}

	if path != linkPath {
		log.Infof("removing link %s", path)
		if err := os.Remove(path); err != nil {
			log.Warnf("error removing symbolic link %s", err)
		}
	}
}

func (b *Blockstore) onlineGC(ctx context.Context, threshold float64, checkFreq time.Duration, check func() error) error {
	b.lockDB()
	defer b.unlockDB()

	// compact first to gather the necessary statistics for GC
	nworkers := runtime.NumCPU() / 2
	if nworkers < 2 {
		nworkers = 2
	}
	if nworkers > 7 { // max out at 1 goroutine per badger level
		nworkers = 7
	}

	err := b.db.Flatten(nworkers)
	if err != nil {
		return err
	}
	checkTick := time.NewTimer(checkFreq)
	defer checkTick.Stop()
	for err == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-checkTick.C:
			err = check()
			checkTick.Reset(checkFreq)
		default:
			err = b.db.RunValueLogGC(threshold)
		}
	}

	if err == badger.ErrNoRewrite {
		// not really an error in this case, it signals the end of GC
		return nil
	}

	return err
}

// CollectGarbage compacts and runs garbage collection on the value log;
// implements the BlockstoreGC trait
func (b *Blockstore) CollectGarbage(ctx context.Context, opts ...blockstore.BlockstoreGCOption) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	var options blockstore.BlockstoreGCOptions
	for _, opt := range opts {
		err := opt(&options)
		if err != nil {
			return err
		}
	}

	if options.FullGC {
		return b.movingGC(ctx)
	}
	threshold := options.Threshold
	if threshold == 0 {
		threshold = defaultGCThreshold
	}
	checkFreq := options.CheckFreq
	if checkFreq < 30*time.Second { // disallow checking more frequently than block time
		checkFreq = 30 * time.Second
	}
	check := options.Check
	if check == nil {
		check = func() error {
			return nil
		}
	}
	return b.onlineGC(ctx, threshold, checkFreq, check)
}

// GCOnce runs garbage collection on the value log;
// implements BlockstoreGCOnce trait
func (b *Blockstore) GCOnce(ctx context.Context, opts ...blockstore.BlockstoreGCOption) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	var options blockstore.BlockstoreGCOptions
	for _, opt := range opts {
		err := opt(&options)
		if err != nil {
			return err
		}
	}
	if options.FullGC {
		return xerrors.Errorf("FullGC option specified for GCOnce but full GC is non incremental")
	}

	threshold := options.Threshold
	if threshold == 0 {
		threshold = defaultGCThreshold
	}

	b.lockDB()
	defer b.unlockDB()

	// Note no compaction needed before single GC as we will hit at most one vlog anyway
	err := b.db.RunValueLogGC(threshold)
	if err == badger.ErrNoRewrite {
		// not really an error in this case, it signals the end of GC
		return nil
	}

	return err
}

// Size returns the aggregate size of the blockstore
func (b *Blockstore) Size() (int64, error) {
	if err := b.access(); err != nil {
		return 0, err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	lsm, vlog := b.db.Size()
	size := lsm + vlog

	if size == 0 {
		// badger reports a 0 size on symlinked directories... sigh
		dir := b.opts.Dir
		entries, err := os.ReadDir(dir)
		if err != nil {
			return 0, err
		}

		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			finfo, err := os.Stat(path)
			if err != nil {
				return 0, err
			}
			size += finfo.Size()
		}
	}

	return size, nil
}

// View implements blockstore.Viewer, which leverages zero-copy read-only
// access to values.
func (b *Blockstore) View(ctx context.Context, cid cid.Cid, fn func([]byte) error) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	return b.db.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(k); err {
		case nil:
			return item.Value(fn)
		case badger.ErrKeyNotFound:
			return ipld.ErrNotFound{Cid: cid}
		default:
			return fmt.Errorf("failed to view block from badger blockstore: %w", err)
		}
	})
}

func (b *Blockstore) Flush(context.Context) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	var nextErr error
	if b.dbNext != nil {
		nextErr = b.dbNext.Sync()
	}

	return multierr.Combine(
		nextErr,
		b.db.Sync(),
	)
}

// Has implements Blockstore.Has.
func (b *Blockstore) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	if err := b.access(); err != nil {
		return false, err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	err := b.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(k)
		return err
	})

	switch err {
	case badger.ErrKeyNotFound:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, fmt.Errorf("failed to check if block exists in badger blockstore: %w", err)
	}
}

// Get implements Blockstore.Get.
func (b *Blockstore) Get(ctx context.Context, cid cid.Cid) (blocks.Block, error) {
	if !cid.Defined() {
		return nil, ipld.ErrNotFound{Cid: cid}
	}

	if err := b.access(); err != nil {
		return nil, err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	var val []byte
	err := b.db.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(k); err {
		case nil:
			val, err = item.ValueCopy(nil)
			return err
		case badger.ErrKeyNotFound:
			return ipld.ErrNotFound{Cid: cid}
		default:
			return fmt.Errorf("failed to get block from badger blockstore: %w", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(val, cid)
}

// GetSize implements Blockstore.GetSize.
func (b *Blockstore) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	if err := b.access(); err != nil {
		return 0, err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	var size int
	err := b.db.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(k); err {
		case nil:
			size = int(item.ValueSize())
		case badger.ErrKeyNotFound:
			return ipld.ErrNotFound{Cid: cid}
		default:
			return fmt.Errorf("failed to get block size from badger blockstore: %w", err)
		}
		return nil
	})
	if err != nil {
		size = -1
	}
	return size, err
}

// Put implements Blockstore.Put.
func (b *Blockstore) Put(ctx context.Context, block blocks.Block) error {
	return b.PutMany(ctx, []blocks.Block{block})
}

// PutMany implements Blockstore.PutMany.
func (b *Blockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	// toReturn tracks the byte slices to return to the pool, if we're using key
	// prefixing. we can't return each slice to the pool after each Set, because
	// badger holds on to the slice.
	var toReturn [][]byte
	if b.prefixing {
		toReturn = make([][]byte, 0, len(blocks))
		defer func() {
			for _, b := range toReturn {
				KeyPool.Put(b)
			}
		}()
	}

	keys := make([][]byte, 0, len(blocks))
	for _, block := range blocks {
		k, pooled := b.PooledStorageKey(block.Cid())
		if pooled {
			toReturn = append(toReturn, k)
		}
		keys = append(keys, k)
	}

	err := b.db.View(func(txn *badger.Txn) error {
		for i, k := range keys {
			switch _, err := txn.Get(k); err {
			case badger.ErrKeyNotFound:
			case nil:
				keys[i] = nil
			default:
				// Something is actually wrong
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	put := func(db *badger.DB) error {
		batch := db.NewWriteBatch()
		defer batch.Cancel()

		for i, block := range blocks {
			k := keys[i]
			if k == nil {
				// skipped because we already have it.
				continue
			}
			if err := batch.Set(k, block.RawData()); err != nil {
				return err
			}
		}

		err := batch.Flush()
		if err != nil {
			return fmt.Errorf("failed to put blocks in badger blockstore: %w", err)
		}

		return nil
	}

	if err := put(b.db); err != nil {
		return err
	}

	if b.dbNext != nil {
		if err := put(b.dbNext); err != nil {
			return err
		}
	}

	return nil
}

// DeleteBlock implements Blockstore.DeleteBlock.
func (b *Blockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	return b.DeleteMany(ctx, []cid.Cid{c})
}

func (b *Blockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	// toReturn tracks the byte slices to return to the pool, if we're using key
	// prefixing. we can't return each slice to the pool after each Set, because
	// badger holds on to the slice.
	var toReturn [][]byte
	if b.prefixing {
		toReturn = make([][]byte, 0, len(cids))
		defer func() {
			for _, b := range toReturn {
				KeyPool.Put(b)
			}
		}()
	}

	batch := b.db.NewWriteBatch()
	defer batch.Cancel()

	for _, cid := range cids {
		k, pooled := b.PooledStorageKey(cid)
		if pooled {
			toReturn = append(toReturn, k)
		}
		if err := batch.Delete(k); err != nil {
			return err
		}
	}

	err := batch.Flush()
	if err != nil {
		err = fmt.Errorf("failed to delete blocks from badger blockstore: %w", err)
	}
	return err
}

// AllKeysChan implements Blockstore.AllKeysChan.
func (b *Blockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	if err := b.access(); err != nil {
		return nil, err
	}

	b.lockDB()
	defer b.unlockDB()

	txn := b.db.NewTransaction(false)
	opts := badger.IteratorOptions{PrefetchSize: 100}
	if b.prefixing {
		opts.Prefix = b.prefix
	}
	iter := txn.NewIterator(opts)

	ch := make(chan cid.Cid)
	go func() {
		defer b.viewers.Done()
		defer close(ch)
		defer iter.Close()

		// NewCidV1 makes a copy of the multihash buffer, so we can reuse it to
		// contain allocs.
		var buf []byte
		for iter.Rewind(); iter.Valid(); iter.Next() {
			if ctx.Err() != nil {
				return // context has fired.
			}
			if !b.isOpen() {
				// open iterators will run even after the database is closed...
				return // closing, yield.
			}
			k := iter.Item().Key()
			if b.prefixing {
				k = k[b.prefixLen:]
			}

			if reqlen := base32.RawStdEncoding.DecodedLen(len(k)); len(buf) < reqlen {
				buf = make([]byte, reqlen)
			}
			if n, err := base32.RawStdEncoding.Decode(buf, k); err == nil {
				select {
				case ch <- cid.NewCidV1(cid.Raw, buf[:n]):
				case <-ctx.Done():
					return
				}
			} else {
				log.Warnf("failed to decode key %s in badger AllKeysChan; err: %s", k, err)
			}
		}
	}()

	return ch, nil
}

// Implementation of BlockstoreIterator interface ------------------------------

func (b *Blockstore) ForEachKey(f func(cid.Cid) error) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	txn := b.db.NewTransaction(false)
	defer txn.Discard()

	opts := badger.IteratorOptions{PrefetchSize: 100}
	if b.prefixing {
		opts.Prefix = b.prefix
	}

	iter := txn.NewIterator(opts)
	defer iter.Close()

	var buf []byte
	for iter.Rewind(); iter.Valid(); iter.Next() {
		if !b.isOpen() {
			return ErrBlockstoreClosed
		}

		k := iter.Item().Key()
		if b.prefixing {
			k = k[b.prefixLen:]
		}

		klen := base32.RawStdEncoding.DecodedLen(len(k))
		if klen > len(buf) {
			buf = make([]byte, klen)
		}

		n, err := base32.RawStdEncoding.Decode(buf, k)
		if err != nil {
			return err
		}

		c := cid.NewCidV1(cid.Raw, buf[:n])

		err = f(c)
		if err != nil {
			return err
		}
	}

	return nil
}

// PooledStorageKey returns the storage key under which this CID is stored.
//
// The key is: prefix + base32_no_padding(cid.Hash)
//
// This method may return pooled byte slice, which MUST be returned to the
// KeyPool if pooled=true, or a leak will occur.
func (b *Blockstore) PooledStorageKey(cid cid.Cid) (key []byte, pooled bool) {
	h := cid.Hash()
	size := base32.RawStdEncoding.EncodedLen(len(h))
	if !b.prefixing { // optimize for branch prediction.
		k := pool.Get(size)
		base32.RawStdEncoding.Encode(k, h)
		return k, true // slicing upto length unnecessary; the pool has already done this.
	}

	size += b.prefixLen
	k := pool.Get(size)
	copy(k, b.prefix)
	base32.RawStdEncoding.Encode(k[b.prefixLen:], h)
	return k, true // slicing upto length unnecessary; the pool has already done this.
}

// StorageKey acts like PooledStorageKey, but attempts to write the storage key
// into the provided slice. If the slice capacity is insufficient, it allocates
// a new byte slice with enough capacity to accommodate the result. This method
// returns the resulting slice.
func (b *Blockstore) StorageKey(dst []byte, cid cid.Cid) []byte {
	h := cid.Hash()
	reqsize := base32.RawStdEncoding.EncodedLen(len(h)) + b.prefixLen
	if reqsize > cap(dst) {
		// passed slice is smaller than required size; create new.
		dst = make([]byte, reqsize)
	} else if reqsize > len(dst) {
		// passed slice has enough capacity, but its length is
		// restricted, expand.
		dst = dst[:cap(dst)]
	}

	if b.prefixing { // optimize for branch prediction.
		copy(dst, b.prefix)
		base32.RawStdEncoding.Encode(dst[b.prefixLen:], h)
	} else {
		base32.RawStdEncoding.Encode(dst, h)
	}
	return dst[:reqsize]
}

// DB is added for lotus-shed needs
// WARNING: THIS IS COMPLETELY UNSAFE; DON'T USE THIS IN PRODUCTION CODE
func (b *Blockstore) DB() *badger.DB {
	return b.db
}
