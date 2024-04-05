package badgerbs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	// Temporary, to get zstd 1.5.6 and https://github.com/facebook/zstd/pull/3895#issuecomment-2067746853
	// Switch back fully to datadog when they upgrade ( valyala has panic()s in their code 😰 )
	zstdv "github.com/valyala/gozstd"

	"github.com/DataDog/zstd"
	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	badgerstruct "github.com/dgraph-io/badger/v2/pb"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logger "github.com/ipfs/go-log/v2"
	pool "github.com/libp2p/go-buffer-pool"
	"github.com/multiformats/go-base32"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/multiformats/go-varint"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
)

type supportedMultihash struct {
	cidMaker         cid.Prefix
	journalShortCode byte // for blake2b multihashes this saves 3 bytes in the journal, which is 26bil*3 ~~ 72GiB of space for archival nodes at time of writing
}

// hardcoded hash list for now
// justification 🧵 https://filecoinproject.slack.com/archives/CRK2LKYHW/p1711381656211189?thread_ts=1711264671.316169&cid=CRK2LKYHW
var (
	supportedMultihashes = map[string]supportedMultihash{
		"\xA0\xE4\x02\x20": {
			cid.NewPrefixV1(uint64(multicodec.Raw), uint64(multicodec.Blake2b256)),
			1,
		},
		"\x12\x20": {
			cid.NewPrefixV1(uint64(multicodec.Raw), uint64(multicodec.Sha2_256)),
			0,
		},
	}

	tryOrder = []string{"\xA0\xE4\x02\x20", "\x12\x20"}
)

const (
	supportedHashLen   = 256 / 8
	mhJournalFilename  = "MultiHashes.bin"
	mhJournalRecordLen = 1 + supportedHashLen // journalShortCode prefix + 256 bits hash

	binkeyBits = 128 // could I safely drop this to 96? 80? at 26bil blocks every byte counts...
	binkeyLen  = binkeyBits / 8
)

var (
	ZstdCompressionWorkers = 0  // defaults to between( 2, 32, runtime.NumCPU/2 )
	ZstdCompressionLevel   = 18 // https://github.com/facebook/zstd/pull/3895#issuecomment-2067746853
)

func zstdCompressionWorkersActual() int {
	if ZstdCompressionWorkers != 0 {
		return ZstdCompressionWorkers
	}
	return between(2, 32, runtime.NumCPU()/2)
}

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

	// Whether to check for legacy base32-encoded keys on Get/Delete
	QueryLegacyKeys bool
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
	// moveStateMoving signifies that there is a move  in a progress
	moveStateMoving
	// moveStateCleanup signifies that a move has completed or aborted and we are cleaning up
	moveStateCleanup
	// moveStateLock signifies that an exclusive lock has been acquired
	moveStateLock
)

type flushWriter interface {
	io.WriteCloser
	Sync() error
}

// Blockstore is a badger-backed IPLD blockstore.
type Blockstore struct {
	stateLk sync.RWMutex
	state   bsState
	viewers sync.WaitGroup

	moveMx    sync.Mutex
	moveCond  sync.Cond
	moveState bsMoveState
	rlock     int

	db        *badger.DB
	mhJournal flushWriter

	dbNext        *badger.DB // when moving
	mhJournalNext flushWriter

	opts Options

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

	if !opts.ReadOnly {
		bs.mhJournal, err = openJournal(opts.Dir)
		if err != nil {
			return nil, err
		}
	}

	return bs, nil
}

var fadvWriter func(uintptr) error

func openJournal(dir string) (*os.File, error) {
	fh, err := os.OpenFile(
		dir+"/"+mhJournalFilename,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE,
		0644,
	)
	if err != nil {
		return nil, err
	}

	if fadvWriter != nil {
		if err := fadvWriter(fh.Fd()); err != nil {
			return nil, err
		}
	}

	return fh, nil
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

	var err error

	if errDb := b.db.Close(); errDb != nil {
		errDb = xerrors.Errorf("failure closing the badger blockstore: %w", errDb)
		log.Warn(errDb)
		err = errDb
	}

	if b.mhJournal != nil {
		if errMj := b.mhJournal.Close(); errMj != nil {
			errMj = xerrors.Errorf("failure closing the multihash journal: %w", errMj)
			log.Warn(errMj)
			if err == nil {
				err = errMj
			}
		}
	}

	return err
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
// then they must be peristed to the old blockstore.
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
		mhJournalNext := b.mhJournalNext
		b.mhJournalNext = nil

		var state bsMoveState
		if dbNext != nil {
			state = moveStateCleanup
		} else {
			state = moveStateNone
		}

		b.unlockMove(state)

		if dbNext != nil {
			// the move failed and we have a left-over db; delete it.
			if err := dbNext.Close(); err != nil {
				log.Warnf("error closing badger db: %s", err)
			}
			if err := mhJournalNext.Close(); err != nil {
				log.Warnf("error closing multihash journal: %s", err)
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
	opts.ReadOnly = false // by definition the new copy is writable (we are just about to write to it)

	dbNew, err := badger.Open(opts.Options)
	if err != nil {
		return fmt.Errorf("failed to open badger blockstore in %s: %w", newPath, err)
	}
	mhjNew, err := openJournal(opts.Dir)
	if err != nil {
		return err
	}

	b.lockMove()
	b.dbNext = dbNew
	b.mhJournalNext = mhjNew
	b.unlockMove(moveStateMoving)

	log.Info("copying blockstore")
	err = b.doCopy(ctx, b.db, b.dbNext, b.mhJournalNext)
	if err != nil {
		return fmt.Errorf("error moving badger blockstore to %s: %w", newPath, err)
	}

	b.lockMove()
	dbOld := b.db
	b.db = b.dbNext
	mhjOld := b.mhJournal
	b.mhJournal = b.mhJournalNext
	b.dbNext = nil
	b.mhJournalNext = nil
	b.unlockMove(moveStateCleanup)

	if err := dbOld.Close(); err != nil {
		log.Warnf("error closing old badger db: %s", err)
	}
	if mhjOld != nil {
		if err := mhjOld.Close(); err != nil {
			log.Warnf("error closing old multihash journal: %s", err)
		}
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

func findCidForPartialKV(kv *badgerstruct.KV) (c cid.Cid, smh supportedMultihash, err error) {
	decV, wasPooled, decErr := decodeValue(kv.Value, true)
	if decErr != nil {
		err = decErr
		return
	}
	if wasPooled {
		defer pool.Put(decV)
	}

	// this is so so SO nasty... 🤮
	for _, pref := range tryOrder {
		trySmh := supportedMultihashes[pref]
		c, err = trySmh.cidMaker.Sum(decV)
		if err != nil {
			return
		}
		h := c.Hash()
		if bytes.Equal(kv.Key, h[len(h)-supportedHashLen:len(h)-supportedHashLen+binkeyLen]) {
			smh = trySmh
			break
		}
		c = cid.Undef
	}

	if !c.Defined() {
		err = xerrors.Errorf("none of the available mutihashers produced a hash starting with 0x%X", kv.Key)
	}
	return
}

// doCopy copies a badger blockstore to another
func (b *Blockstore) doCopy(ctx context.Context, from, to *badger.DB, jrnlFh io.Writer) (defErr error) {
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

		jrnlSlab := pool.Get(len(kvs) * mhJournalRecordLen)
		defer pool.Put(jrnlSlab)
		jrnl := jrnlSlab[:0]

		eg, _ := errgroup.WithContext(ctx)
		eg.SetLimit(zstdCompressionWorkersActual())

		for _, kv := range kvs {

			// pass through already encoded value
			if len(kv.Key) == binkeyLen {
				// nasty way to recreate the hash from the payload alone 🤮
				// worth it however for maintaining journal consistency
				c, smh, err := findCidForPartialKV(kv)
				if err != nil {
					return err
				}

				if err := batch.Set(kv.Key, kv.Value); err != nil {
					return err
				}

				// add a journal record
				// NOTE: this could very well result in duplicates
				// there isn't much we can do about this right now...
				mh := c.Hash()
				jrnl = append(jrnl, smh.journalShortCode)
				jrnl = append(jrnl, mh[len(mh)-supportedHashLen:]...)

				continue
			}

			// we have a legacy non-encoded key: remake it and then asynchoronously its value
			// do this regardless of b.opts.QueryLegacyKeys
			//
			// can not use a pooled decode buffer, as it gets held by the badger batch
			mh := make([]byte, varint.MaxLenUvarint63+supportedHashLen)
			n, err := base32.RawStdEncoding.Decode(mh, kv.Key[b.prefixLen:])
			if err != nil {
				return xerrors.Errorf("undecodeable key 0x%X: %s", kv.Key[b.prefixLen:], err)
			}
			mh = mh[:n:n]
			smh, err := isMultihashSupported(mh)
			if err != nil {
				return xerrors.Errorf("unsupported multihash for key 0x%X: %w", kv.Key[b.prefixLen:], err)
			}

			// add a journal record
			// NOTE: this could very well result in duplicates
			// there isn't much we can do about this right now...
			jrnl = append(jrnl, smh.journalShortCode)
			jrnl = append(jrnl, mh[len(mh)-supportedHashLen:]...)

			v := kv.Value
			eg.Go(func() error {
				var err error
				v, err = encodeValue(v, mh)
				if err != nil {
					return err
				}
				return batch.Set(
					mh[len(mh)-supportedHashLen:len(mh)-supportedHashLen+binkeyLen:len(mh)-supportedHashLen+binkeyLen], // we checked the multihash digest is hashLen (256bits) long
					v,
				)
			})

		}

		if err := eg.Wait(); err != nil {
			return err
		}
		if _, err := jrnlFh.Write(jrnl); err != nil {
			return xerrors.Errorf("failed to write multihashes to journal: %w", err)
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
	// follow symbolic links, otherwise the data wil be left behind
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
	var size int64

	// do not use b.db.Size(): since we are storing data outside of usual
	// badger files it can't be accurate anyway. Just sum up the dir sizes
	// without even trying to lock the db
	//
	// moreover: badger reports a 0 size on symlinked directories anyway
	dir := b.opts.Dir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		finfo, _ := os.Stat(path) // ignore potential error: if we are in a compaction an .sst might disappear on us
		size += finfo.Size()
	}

	return size, nil
}

func isMultihashSupported(mh []byte) (supportedMultihash, error) {
	var smh supportedMultihash
	mhDec, err := multihash.Decode(mh)
	if err != nil {
		return smh, xerrors.Errorf("unexpected error decoding multihash 0x%X: %s", mh, err)
	}
	if mhDec.Length != supportedHashLen {
		return smh, xerrors.Errorf("unsupported hash length of %d bits", mhDec.Length*8)
	}
	smh, found := supportedMultihashes[string(mh[:len(mh)-supportedHashLen])]
	if !found {
		return smh, xerrors.Errorf("unsupported multihash prefix 0x%X", mh[:len(mh)-supportedHashLen])
	}
	return smh, nil
}

// https://tools.ietf.org/html/rfc8478#section-3.1.1
const (
	zstdMagic    = "\x28\xB5\x2F\xFD"
	zstdMagicLen = len(zstdMagic)
)

/*
Each value keyed by a new-style shortened multihash receives a single varint prefix, with the uint64 value:

  (3bit store-type)            // 0 for as-is, 1 for basic zstd compression, remainder reserved
    +
  (one reserved bit)     << 3  // always 0 for now
    +
  (3bit compressability) << 4  // 0 if store-type is 0, otherwise `c := (origLen-compLen) * 8 / origLen`
                               // (using integer math, `origLen > compLen > 0` holds for any non-0 store-type)
    +
  (IPLD block data size) << 7  // 0 if store-type is 0
*/

func encodeValue(input, k []byte) ([]byte, error) {
	if len(input) == 0 {
		return []byte{0}, nil
	}

	maxClen := zstd.CompressBound(len(input))

	// figure out pooling in a future commit
	buf := make(
		[]byte,
		varint.MaxLenUvarint63+maxClen,
	)

	// a scope to contain the Compress output on purpose: ensures we always operate within buf
	var compLen int
	{
		// temporary use zstdv until we get datadog 1.5.6
		cOut := zstdv.CompressLevel(
			buf[varint.MaxLenUvarint63-zstdMagicLen:varint.MaxLenUvarint63-zstdMagicLen], // arrange so that the zstdMagic ends where max-varint would end
			input,
			ZstdCompressionLevel,
		)
		if hdr := buf[varint.MaxLenUvarint63-zstdMagicLen : varint.MaxLenUvarint63]; !bytes.Equal(hdr, []byte(zstdMagic)) {
			return nil, xerrors.Errorf("zstandard compressor produced stream prefixed with 0x%X instead of the expected 0x%X for key 0x%X", hdr, zstdMagic, k)
		}
		if len(cOut) > maxClen {
			return nil, xerrors.Errorf("zstandard compressor produced stream of %d bytes larger than the %d indicated CompressBound() for key 0x%X", len(cOut), maxClen, k)
		}
		compLen = len(cOut) - zstdMagicLen
	}

	// check if compression is worthwhile at all
	if compLen >= len(input) {
		buf[0] = 0 // stored as-is
		copy(buf[1:], input)
		return buf[0 : len(input)+1 : len(input)+1], nil
	}

	pref := uint64(
		1 + // store-type 1
			(((len(input) - compLen) * 8 / len(input)) << 4) +
			(len(input) << 7),
	)

	n := varint.PutUvarint(
		// place the varint so it ends where the data (post zstdMagic) begins
		buf[varint.MaxLenUvarint63-varint.UvarintSize(pref):],
		pref,
	)

	return buf[varint.MaxLenUvarint63-n : varint.MaxLenUvarint63+compLen : varint.MaxLenUvarint63+compLen], nil

}

// badgerGet is a basic tri-state:  value+nil  nil+nil  nil+err
func (b *Blockstore) badgerGet(t *badger.Txn, mk badgerMultiKey) (*valueItem, error) {
	switch item, err := t.Get(mk.binKey()); err {
	case nil:
		return &valueItem{item, mk.binKey()}, nil
	case badger.ErrKeyNotFound:
		if !b.opts.QueryLegacyKeys {
			return nil, nil
		}
		// try again with legacy ...
		switch item, err := t.Get(mk.legacyKey()); err {
		case nil:
			return &valueItem{item, mk.legacyKey()}, nil
		case badger.ErrKeyNotFound:
			return nil, nil
		default:
			return nil, err
		}
	default:
		return nil, err
	}
}

type valueItem struct {
	badgerItem *badger.Item
	currentKey []byte
}

func (vi *valueItem) size() (int, error) {
	if len(vi.currentKey) != binkeyLen {
		// legacy key
		return int(vi.badgerItem.ValueSize()), nil
	}

	sz := int(-1)
	err := vi.badgerItem.Value(func(val []byte) error {
		if len(val) == 0 {
			return xerrors.New("unexpected zero-length record")
		}

		if val[0] == 0 { // storage type 0
			sz = len(val) - 1
			return nil
		}

		pref, _, err := varint.FromUvarint(val)
		if err != nil {
			return err
		}
		sz = int(pref >> 7)
		return nil
	})

	return sz, err
}
func (vi *valueItem) block(c cid.Cid) (blocks.Block, error) {
	payload, err := vi.badgerItem.ValueCopy(nil)
	if err != nil {
		return nil, err
	}
	if len(vi.currentKey) == binkeyLen {
		var err error
		payload, _, err = decodeValue(payload, false)
		if err != nil {
			return nil, err
		}
	}
	if err := checkHash(c, payload, vi.currentKey); err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(payload, c)
}
func (vi *valueItem) view(c cid.Cid, f func(val []byte) error) error {
	return vi.badgerItem.Value(func(payload []byte) error {
		if len(vi.currentKey) == binkeyLen {
			var isPooled bool
			var err error
			payload, isPooled, err = decodeValue(payload, true) // maybe pooled combined with a defer below
			if err != nil {
				return err
			}
			if isPooled {
				defer pool.Put(payload)
			}
		}
		if err := checkHash(c, payload, vi.currentKey); err != nil {
			return err
		}
		return f(payload)
	})
}

func checkHash(c cid.Cid, b, k []byte) error {
	rehash, err := c.Prefix().Sum(b)
	if err != nil {
		return err
	}
	if !rehash.Equals(c) {
		return xerrors.Errorf("multihash mismatch for cid %s (badger key 0x%X): value hashes to 0x%X, but expected multihash 0x%X", c, k, []byte(rehash.Hash()), []byte(c.Hash()))
	}
	return nil

}
func decodeValue(in []byte, pooledReturn bool) ([]byte, bool, error) {
	if len(in) == 0 {
		return nil, false, xerrors.New("corrupted zero-length record")
	}
	if in[0] == 0 {
		return in[1:], false, nil
	}

	pref, n, err := varint.FromUvarint(in)
	if err != nil {
		return nil, false, err
	}

	storType := pref & 0b111
	if storType != 1 {
		return nil, false, xerrors.Errorf("unexpected StorageType %d", storType)
	}

	var bufOut []byte
	if pooledReturn {
		bufOut = pool.Get(int(pref >> 7))
	} else {
		bufOut = make([]byte, int(pref>>7))
	}
	bufDec := pool.Get(zstdMagicLen + len(in) - n)
	defer pool.Put(bufDec)

	// eventually switch to a pooled zstd.NewCtx()
	// https://github.com/DataDog/zstd/pull/130#issuecomment-2067955171
	_, err = zstd.DecompressInto(
		bufOut,
		append(append(bufDec[:0], zstdMagic...), in[n:]...),
	)
	if err != nil {
		if pooledReturn {
			pool.Put(bufOut)
		}
		return nil, false, xerrors.Errorf("decompression failed: %w", err)
	}

	return bufOut, pooledReturn, nil
}

// View implements blockstore.Viewer, which leverages zero-copy read-only
// access to values.
func (b *Blockstore) View(ctx context.Context, c cid.Cid, fn func([]byte) error) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	mk, err := b.pooledMultiKey(c)
	if err != nil {
		return err
	}
	defer pool.Put(mk)

	return b.db.View(func(txn *badger.Txn) error {
		val, err := b.badgerGet(txn, mk)
		if err != nil {
			return fmt.Errorf("failed to view block from badger blockstore: %w", err)
		} else if val == nil {
			return ipld.ErrNotFound{Cid: c}
		}
		return val.view(c, fn)
	})
}

func (b *Blockstore) Flush(context.Context) error {
	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	// fsync the new db first
	if b.dbNext != nil {
		if err := b.dbNext.Sync(); err != nil {
			return err
		}
		if err := b.mhJournalNext.Sync(); err != nil {
			return err
		}
	}

	if err := b.db.Sync(); err != nil {
		return err
	}
	if b.mhJournal != nil {
		if err := b.mhJournal.Sync(); err != nil {
			return err
		}
	}

	return nil
}

// Has implements Blockstore.Has.
func (b *Blockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	if err := b.access(); err != nil {
		return false, err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	mk, err := b.pooledMultiKey(c)
	if err != nil {
		return false, err
	}
	defer pool.Put(mk)

	var canHaz bool
	err = b.db.View(func(txn *badger.Txn) error {
		val, err := b.badgerGet(txn, mk)
		if val != nil {
			canHaz = true
		}
		return err
	})

	if err != nil {
		return false, fmt.Errorf("failed to check if block exists in badger blockstore: %w", err)
	}
	return canHaz, nil
}

// Get implements Blockstore.Get.
func (b *Blockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	if !c.Defined() {
		return nil, ipld.ErrNotFound{Cid: c}
	}

	if err := b.access(); err != nil {
		return nil, err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	mk, err := b.pooledMultiKey(c)
	if err != nil {
		return nil, err
	}
	defer pool.Put(mk)

	var blk blocks.Block
	err = b.db.View(func(txn *badger.Txn) error {
		val, err := b.badgerGet(txn, mk)
		if err != nil {
			return fmt.Errorf("failed to get block from badger blockstore: %w", err)
		} else if val == nil {
			return ipld.ErrNotFound{Cid: c}
		}
		blk, err = val.block(c)
		return err
	})
	return blk, err
}

// GetSize implements Blockstore.GetSize.
func (b *Blockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if err := b.access(); err != nil {
		return 0, err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	mk, err := b.pooledMultiKey(c)
	if err != nil {
		return -1, err
	}
	defer pool.Put(mk)

	size := -1
	err = b.db.View(func(txn *badger.Txn) error {
		val, err := b.badgerGet(txn, mk)

		if err != nil {
			return fmt.Errorf("failed to get block size from badger blockstore: %w", err)
		} else if val == nil {
			return ipld.ErrNotFound{Cid: c}
		}

		size, err = val.size()
		return err
	})

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

	type kv struct {
		mk  badgerMultiKey
		val []byte
	}

	// kvs/defer() must be declared before (thus happen after) the NewWriteBatch below
	kvs := make([]kv, 0, len(blocks))
	defer func() {
		for _, kv := range kvs {
			pool.Put(kv.mk)
		}
	}()
	for i := range blocks {
		mk, err := b.pooledMultiKey(blocks[i].Cid())
		if err != nil {
			return err
		}
		kvs = append(kvs, kv{mk: mk})
	}

	jrnlSlab := pool.Get(len(blocks) * mhJournalRecordLen)
	defer pool.Put(jrnlSlab)
	jrnl := jrnlSlab[:0]

	var eg errgroup.Group
	eg.SetLimit(zstdCompressionWorkersActual())

	if err := b.db.View(func(txn *badger.Txn) error {
		for i := range kvs {
			val, err := b.badgerGet(txn, kvs[i].mk)
			if err != nil {
				// Something is actually wrong
				return err
			} else if val == nil {
				// Got to insert that, check it is supported, write journal, encode value
				mh := blocks[i].Cid().Hash()
				smh, err := isMultihashSupported(mh)
				if err != nil {
					return xerrors.Errorf("unsupported multihash for cid %s: %w", blocks[i].Cid(), err)
				}

				i := i
				eg.Go(func() error {
					kvs[i].val, err = encodeValue(blocks[i].RawData(), kvs[i].mk)
					return err
				})

				// add a journal record
				jrnl = append(jrnl, smh.journalShortCode)
				jrnl = append(jrnl, mh[len(mh)-supportedHashLen:]...)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	put := func(db *badger.DB, mhj flushWriter) error {
		batch := db.NewWriteBatch()
		defer batch.Cancel()

		for i := range kvs {
			if kvs[i].val != nil {
				if err := batch.Set(kvs[i].mk.binKey(), kvs[i].val); err != nil {
					return err
				}
			}
		}

		if err := batch.Flush(); err != nil {
			return xerrors.Errorf("failed to put blocks in badger blockstore: %w", err)
		}

		if _, err := mhj.Write(jrnl); err != nil {
			return xerrors.Errorf("failed to write multihashes to journal: %w", err)
		}

		return nil
	}

	if err := put(b.db, b.mhJournal); err != nil {
		return err
	}

	if b.dbNext != nil {
		if err := put(b.dbNext, b.mhJournalNext); err != nil {
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

	// keys/defer() must be declared before (thus happen after) the NewWriteBatch below
	multikeys := make([]badgerMultiKey, len(cids))
	defer func() {
		for _, mk := range multikeys {
			if mk != nil {
				pool.Put(mk)
			}
		}
	}()
	var err error
	for i := range cids {
		multikeys[i], err = b.pooledMultiKey(cids[i])
		if err != nil {
			return err
		}
	}

	batch := b.db.NewWriteBatch()
	defer batch.Cancel()

	// only delete keys we are known to have
	// blindly calling batch.Delete() clutters the logs with delete markers
	if err := b.db.View(func(txn *badger.Txn) error {
		for _, mk := range multikeys {

			variants := append(
				make([][]byte, 0, 2),
				mk.binKey(),
			)
			if b.opts.QueryLegacyKeys {
				variants = append(variants, mk.legacyKey())
			}

			for _, k := range variants {
				_, err := txn.Get(k)
				if err == badger.ErrKeyNotFound {
					continue
				} else if err != nil {
					return err
				}
				// key does exist
				if err := batch.Delete(k); err != nil {
					return err
				}
			}
		}

		return batch.Flush()
	}); err != nil {
		return fmt.Errorf("failed to delete blocks from badger blockstore: %w", err)
	}

	return nil
}

// AllKeysChan implements Blockstore.AllKeysChan.
var EnableHorriblyInefficientAllKeysChanMethod bool

func (b *Blockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {

	if !EnableHorriblyInefficientAllKeysChanMethod {
		return nil, xerrors.New("method AllKeysChan() is not supported, use ForEachKey() instead")
	}

	// reimplement to use strictly as test of the below
	// this is useless in production anyway 🫠
	cids := make([]cid.Cid, 0, 1<<20)

	if err := b.ForEachKey(func(c cid.Cid) error {
		cids = append(cids, c)
		return nil
	}); err != nil {
		return nil, err
	}

	ch := make(chan cid.Cid)
	go func() {
		defer close(ch)
		for _, c := range cids {
			select {
			case <-ctx.Done():
				return
			case ch <- c:
				// keep looping
			}
		}
	}()

	return ch, nil
}

// Implementation of BlockstoreIterator interface
func (b *Blockstore) ForEachKey(f func(cid.Cid) error) error {
	// unfortunately this is used in splitstore, so have to reimplement horribly 🫠
	mhBuf := pool.Get(varint.MaxLenUvarint63 + supportedHashLen)
	defer pool.Put(mhBuf)

	if err := b.access(); err != nil {
		return err
	}
	defer b.viewers.Done()

	b.lockDB()
	defer b.unlockDB()

	return iterateBadger(context.Background(), b.db, func(kvs []*badgerstruct.KV) error {
		if !b.isOpen() {
			return ErrBlockstoreClosed
		}

		for _, kv := range kvs {

			var c cid.Cid

			if len(kv.Key) != binkeyLen {
				if !b.opts.QueryLegacyKeys {
					continue
				}
				// this is a legacy key: just use it
				n, err := base32.RawStdEncoding.Decode(mhBuf, kv.Key[b.prefixLen:])
				if err != nil {
					return err
				}
				c = cid.NewCidV1(uint64(multicodec.Raw), mhBuf[:n])
			} else {
				var err error
				c, _, err = findCidForPartialKV(kv)
				if err != nil {
					return err
				}
			}

			if err := f(c); err != nil {
				return err
			}
		}

		return nil
	})
}

// HashOnRead implements Blockstore.HashOnRead. It is not supported by this
// blockstore.
func (b *Blockstore) HashOnRead(t bool) {
	if !t {
		log.Warnf("attempt to disable HashOnRead on badger blockstore: this is always unconditionally enabled, ignoring")
	}
}

type badgerMultiKey []byte

func (bk badgerMultiKey) binKey() []byte    { return bk[:binkeyLen] }
func (bk badgerMultiKey) legacyKey() []byte { return bk[binkeyLen:] }

// returns an amalgam binary/legacy key to be used for the necessary lookups
func (b *Blockstore) pooledMultiKey(c cid.Cid) (badgerMultiKey, error) {
	mh := c.Hash()

	if _, err := isMultihashSupported(mh); err != nil {
		return nil, err
	}
	binkey := mh[len(mh)-supportedHashLen : len(mh)-supportedHashLen+binkeyLen]

	if !b.opts.QueryLegacyKeys {
		mk := pool.Get(binkeyLen)
		copy(mk, binkey)
		return mk, nil
	}

	legacyLen := base32.RawStdEncoding.EncodedLen(len(mh))

	if !b.prefixing {
		mk := pool.Get(binkeyLen + legacyLen)
		copy(mk, binkey)
		base32.RawStdEncoding.Encode(mk[binkeyLen:], mh)
		return mk, nil
	}

	mk := pool.Get(binkeyLen + b.prefixLen + legacyLen)
	copy(mk, binkey)
	copy(mk[binkeyLen:], b.prefix)
	base32.RawStdEncoding.Encode(mk[binkeyLen+b.prefixLen:], mh)

	return mk, nil
}

// this method is added for lotus-shed needs
// WARNING: THIS IS COMPLETELY UNSAFE; DONT USE THIS IN PRODUCTION CODE
func (b *Blockstore) DB() *badger.DB {
	return b.db
}
