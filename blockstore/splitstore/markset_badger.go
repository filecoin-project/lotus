package splitstore

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/system"
)

type BadgerMarkSetEnv struct {
	path string
}

var _ MarkSetEnv = (*BadgerMarkSetEnv)(nil)

type BadgerMarkSet struct {
	mx      sync.RWMutex
	cond    sync.Cond
	pend    map[string]struct{}
	writing map[int]map[string]struct{}
	writers int
	seqno   int
	version int
	persist bool

	db   *badger.DB
	path string
}

var _ MarkSet = (*BadgerMarkSet)(nil)

var badgerMarkSetBatchSize = 16384

func NewBadgerMarkSetEnv(path string) (MarkSetEnv, error) {
	msPath := filepath.Join(path, "markset.badger")
	err := os.MkdirAll(msPath, 0755) //nolint:gosec
	if err != nil {
		return nil, xerrors.Errorf("error creating markset directory: %w", err)
	}

	return &BadgerMarkSetEnv{path: msPath}, nil
}

func (e *BadgerMarkSetEnv) New(name string, sizeHint int64) (MarkSet, error) {
	path := filepath.Join(e.path, name)

	db, err := openBadgerDB(path, false)
	if err != nil {
		return nil, xerrors.Errorf("error creating badger db: %w", err)
	}

	ms := &BadgerMarkSet{
		pend:    make(map[string]struct{}),
		writing: make(map[int]map[string]struct{}),
		db:      db,
		path:    path,
	}
	ms.cond.L = &ms.mx

	return ms, nil
}

func (e *BadgerMarkSetEnv) Recover(name string) (MarkSet, error) {
	path := filepath.Join(e.path, name)

	if _, err := os.Stat(path); err != nil {
		return nil, xerrors.Errorf("error stating badger db path: %w", err)
	}

	db, err := openBadgerDB(path, true)
	if err != nil {
		return nil, xerrors.Errorf("error creating badger db: %w", err)
	}

	ms := &BadgerMarkSet{
		pend:    make(map[string]struct{}),
		writing: make(map[int]map[string]struct{}),
		db:      db,
		path:    path,
		persist: true,
	}
	ms.cond.L = &ms.mx

	return ms, nil
}

func (e *BadgerMarkSetEnv) Close() error {
	return nil
}

func (s *BadgerMarkSet) BeginCriticalSection() error {
	s.mx.Lock()

	if s.persist {
		s.mx.Unlock()
		return nil
	}

	var write bool
	var seqno int
	if len(s.pend) > 0 {
		write = true
		seqno = s.nextBatch()
	}

	s.persist = true
	s.mx.Unlock()

	if write {
		// all writes sync once persist is true
		return s.write(seqno)
	}

	// wait for any pending writes and sync
	s.mx.Lock()
	for s.writers > 0 {
		s.cond.Wait()
	}
	s.mx.Unlock()

	return s.db.Sync()
}

func (s *BadgerMarkSet) EndCriticalSection() {
	s.mx.Lock()
	defer s.mx.Unlock()

	s.persist = false
}

func (s *BadgerMarkSet) Mark(c cid.Cid) error {
	s.mx.Lock()
	if s.pend == nil {
		s.mx.Unlock()
		return errMarkSetClosed
	}

	write, seqno := s.put(string(c.Hash()))
	s.mx.Unlock()

	if write {
		return s.write(seqno)
	}

	return nil
}

func (s *BadgerMarkSet) MarkMany(batch []cid.Cid) error {
	s.mx.Lock()
	if s.pend == nil {
		s.mx.Unlock()
		return errMarkSetClosed
	}

	write, seqno := s.putMany(batch)
	s.mx.Unlock()

	if write {
		return s.write(seqno)
	}

	return nil
}

func (s *BadgerMarkSet) Has(c cid.Cid) (bool, error) {
	s.mx.RLock()
	defer s.mx.RUnlock()

	key := c.Hash()
	pendKey := string(key)

	has, err := s.tryPending(pendKey)
	if has || err != nil {
		return has, err
	}

	return s.tryDB(key)
}

func (s *BadgerMarkSet) Visit(c cid.Cid) (bool, error) {
	key := c.Hash()
	pendKey := string(key)

	s.mx.RLock()

	has, err := s.tryPending(pendKey)
	if has || err != nil {
		s.mx.RUnlock()
		return false, err
	}

	has, err = s.tryDB(key)
	if has || err != nil {
		s.mx.RUnlock()
		return false, err
	}

	// we need to upgrade the lock to exclusive in order to write; take the version count to see
	// if there was another write while we were upgrading
	version := s.version
	s.mx.RUnlock()

	s.mx.Lock()
	// we have to do the check dance again
	has, err = s.tryPending(pendKey)
	if has || err != nil {
		s.mx.Unlock()
		return false, err
	}

	if version != s.version {
		// something was written to the db, we need to check it
		has, err = s.tryDB(key)
		if has || err != nil {
			s.mx.Unlock()
			return false, err
		}
	}

	write, seqno := s.put(pendKey)
	s.mx.Unlock()

	if write {
		err = s.write(seqno)
	}

	return true, err
}

// reader holds the (r)lock
func (s *BadgerMarkSet) tryPending(key string) (has bool, err error) {
	if s.pend == nil {
		return false, errMarkSetClosed
	}

	if _, ok := s.pend[key]; ok {
		return true, nil
	}

	for _, wr := range s.writing {
		if _, ok := wr[key]; ok {
			return true, nil
		}
	}

	return false, nil
}

func (s *BadgerMarkSet) tryDB(key []byte) (has bool, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})

	switch err {
	case nil:
		return true, nil

	case badger.ErrKeyNotFound:
		return false, nil

	default:
		return false, err
	}
}

// writer holds the exclusive lock
func (s *BadgerMarkSet) put(key string) (write bool, seqno int) {
	s.pend[key] = struct{}{}
	if !s.persist && len(s.pend) < badgerMarkSetBatchSize {
		return false, 0
	}

	seqno = s.nextBatch()
	return true, seqno
}

func (s *BadgerMarkSet) putMany(batch []cid.Cid) (write bool, seqno int) {
	for _, c := range batch {
		key := string(c.Hash())
		s.pend[key] = struct{}{}
	}

	if !s.persist && len(s.pend) < badgerMarkSetBatchSize {
		return false, 0
	}

	seqno = s.nextBatch()
	return true, seqno
}

func (s *BadgerMarkSet) nextBatch() int {
	seqno := s.seqno
	s.seqno++
	s.writing[seqno] = s.pend
	s.pend = make(map[string]struct{})
	return seqno
}

func (s *BadgerMarkSet) write(seqno int) (err error) {
	s.mx.Lock()
	if s.pend == nil {
		s.mx.Unlock()
		return errMarkSetClosed
	}

	pend := s.writing[seqno]
	s.writers++
	s.mx.Unlock()

	defer func() {
		s.mx.Lock()
		defer s.mx.Unlock()

		if err == nil {
			delete(s.writing, seqno)
			s.version++
		}

		s.writers--
		if s.writers == 0 {
			s.cond.Broadcast()
		}
	}()

	empty := []byte{} // not nil

	batch := s.db.NewWriteBatch()
	defer batch.Cancel()

	for k := range pend {
		if err = batch.Set([]byte(k), empty); err != nil {
			return xerrors.Errorf("error setting batch: %w", err)
		}
	}

	err = batch.Flush()
	if err != nil {
		return xerrors.Errorf("error flushing batch to badger markset: %w", err)
	}

	s.mx.RLock()
	persist := s.persist
	s.mx.RUnlock()

	if persist && !system.BadgerFsyncDisable { // WARNING: disabling sync makes recovery from crash during critical section unsound
		return s.db.Sync()
	}

	return nil
}

func (s *BadgerMarkSet) Close() error {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.pend == nil {
		return nil
	}

	for s.writers > 0 {
		s.cond.Wait()
	}

	s.pend = nil
	db := s.db
	s.db = nil

	return closeBadgerDB(db, s.path, s.persist)
}

func openBadgerDB(path string, recover bool) (*badger.DB, error) {
	// if it is not a recovery, clean up first
	if !recover {
		err := os.RemoveAll(path)
		if err != nil {
			return nil, xerrors.Errorf("error clearing markset directory: %w", err)
		}

		err = os.MkdirAll(path, 0755) //nolint:gosec
		if err != nil {
			return nil, xerrors.Errorf("error creating markset directory: %w", err)
		}
	}

	opts := badger.DefaultOptions(path)
	// we manually sync when we are in critical section
	opts.SyncWrites = false
	// no need to do that
	opts.CompactL0OnClose = false
	// we store hashes, not much to gain by compression
	opts.Compression = options.None
	// Note: We use FileIO for loading modes to avoid memory thrashing and interference
	//       between the system blockstore and the markset.
	//       It was observed that using the default memory mapped option resulted in
	//       significant interference and unacceptably high block validation times once the markset
	//       exceeded 1GB in size.
	opts.TableLoadingMode = options.FileIO
	opts.ValueLogLoadingMode = options.FileIO
	// We increase the number of L0 tables before compaction to make it unlikely to
	// be necessary.
	opts.NumLevelZeroTables = 20      // default is 5
	opts.NumLevelZeroTablesStall = 30 // default is 10
	// increase the number of compactors from default 2 so that if we ever have to
	// compact, it is fast
	if runtime.NumCPU()/2 > opts.NumCompactors {
		opts.NumCompactors = runtime.NumCPU() / 2
	}
	opts.Logger = &badgerLogger{
		SugaredLogger: log.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar(),
		skip2:         log.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
	}

	return badger.Open(opts)
}

func closeBadgerDB(db *badger.DB, path string, persist bool) error {
	err := db.Close()
	if err != nil {
		return xerrors.Errorf("error closing badger markset: %w", err)
	}

	if persist {
		return nil
	}

	err = os.RemoveAll(path)
	if err != nil {
		return xerrors.Errorf("error deleting badger markset: %w", err)
	}

	return nil
}

// badger logging through go-log
type badgerLogger struct {
	*zap.SugaredLogger
	skip2 *zap.SugaredLogger
}

func (b *badgerLogger) Warningf(format string, args ...interface{}) {}
func (b *badgerLogger) Infof(format string, args ...interface{})    {}
func (b *badgerLogger) Debugf(format string, args ...interface{})   {}
