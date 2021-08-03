package splitstore

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/xerrors"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"go.uber.org/zap"

	cid "github.com/ipfs/go-cid"
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
	seqno   int
	version int

	db   *badger.DB
	path string
}

var _ MarkSet = (*BadgerMarkSet)(nil)
var _ MarkSetVisitor = (*BadgerMarkSet)(nil)

var badgerMarkSetBatchSize = 16384

func NewBadgerMarkSetEnv(path string) (MarkSetEnv, error) {
	msPath := filepath.Join(path, "markset.badger")
	err := os.MkdirAll(msPath, 0755) //nolint:gosec
	if err != nil {
		return nil, xerrors.Errorf("error creating markset directory: %w", err)
	}

	return &BadgerMarkSetEnv{path: msPath}, nil
}

func (e *BadgerMarkSetEnv) create(name string, sizeHint int64) (*BadgerMarkSet, error) {
	path := filepath.Join(e.path, name)

	db, err := openTransientBadgerDB(path)
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

func (e *BadgerMarkSetEnv) Create(name string, sizeHint int64) (MarkSet, error) {
	return e.create(name, sizeHint)
}

func (e *BadgerMarkSetEnv) CreateVisitor(name string, sizeHint int64) (MarkSetVisitor, error) {
	return e.create(name, sizeHint)
}

func (e *BadgerMarkSetEnv) Close() error {
	return os.RemoveAll(e.path)
}

func (s *BadgerMarkSet) Mark(c cid.Cid) error {
	s.mx.Lock()

	if s.pend == nil {
		s.mx.Unlock()
		return errMarkSetClosed
	}

	s.pend[string(c.Hash())] = struct{}{}

	if len(s.pend) < badgerMarkSetBatchSize {
		s.mx.Unlock()
		return nil
	}

	return s.putPending()
}

func (s *BadgerMarkSet) Has(c cid.Cid) (bool, error) {
	s.mx.RLock()
	defer s.mx.RUnlock()

	if s.pend == nil {
		return false, errMarkSetClosed
	}

	key := c.Hash()
	pendKey := string(key)
	_, ok := s.pend[pendKey]
	if ok {
		return true, nil
	}

	for _, wr := range s.writing {
		_, ok := wr[pendKey]
		if ok {
			return true, nil
		}
	}

	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})

	switch err {
	case nil:
		return true, nil

	case badger.ErrKeyNotFound:
		return false, nil

	default:
		return false, xerrors.Errorf("error checking badger markset: %w", err)
	}
}

func (s *BadgerMarkSet) Visit(c cid.Cid) (bool, error) {
	key := c.Hash()
	pendKey := string(key)

	checkPending := func() (bool, error) {
		if s.pend == nil {
			return false, errMarkSetClosed
		}

		if _, ok := s.pend[pendKey]; ok {
			return false, nil
		}

		for _, wr := range s.writing {
			if _, ok := wr[pendKey]; ok {
				return false, nil
			}
		}

		return true, nil
	}

	s.mx.RLock()

	visit, err := checkPending()
	if !visit {
		s.mx.RUnlock()
		return visit, err
	}

	err = s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})

	switch err {
	case nil:
		s.mx.RUnlock()
		return false, nil

	case badger.ErrKeyNotFound:
		// need to upgrade the lock to exclusive in order to write; take the version count to see
		// if there was another write while we were upgrading
		version := s.version
		s.mx.RUnlock()

		s.mx.Lock()

		// we have to do the check dance again
		visit, err := checkPending()
		if !visit {
			s.mx.Unlock()
			return visit, err
		}

		if version != s.version {
			// something was written to the db, we need to check it
			err = s.db.View(func(txn *badger.Txn) error {
				_, err := txn.Get(key)
				return err
			})

			switch err {
			case nil:
				s.mx.Unlock()
				return false, nil

			case badger.ErrKeyNotFound:

			default:
				s.mx.Unlock()
				return false, xerrors.Errorf("error checking badger markset: %w", err)
			}
		}

		s.pend[pendKey] = struct{}{}
		if len(s.pend) < badgerMarkSetBatchSize {
			s.mx.Unlock()
			return true, nil
		}

		if err := s.putPending(); err != nil {
			return false, err
		}

		return true, nil

	default:
		s.mx.RUnlock()
		return false, xerrors.Errorf("error checking badger markset: %w", err)
	}
}

// writer holds the lock
func (s *BadgerMarkSet) putPending() error {
	pend := s.pend
	seqno := s.seqno
	s.seqno++
	s.writing[seqno] = pend
	s.pend = make(map[string]struct{})
	s.mx.Unlock()

	defer func() {
		s.mx.Lock()
		defer s.mx.Unlock()

		s.version++
		delete(s.writing, seqno)
		if len(s.writing) == 0 {
			s.cond.Broadcast()
		}
	}()

	empty := []byte{} // not nil

	batch := s.db.NewWriteBatch()
	defer batch.Cancel()

	for k := range pend {
		if err := batch.Set([]byte(k), empty); err != nil {
			return xerrors.Errorf("error setting batch: %w", err)
		}
	}

	err := batch.Flush()
	if err != nil {
		return xerrors.Errorf("error flushing batch to badger markset: %w", err)
	}

	return nil
}

func (s *BadgerMarkSet) Close() error {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.pend == nil {
		return nil
	}

	for len(s.writing) > 0 {
		s.cond.Wait()
	}

	s.pend = nil
	db := s.db
	s.db = nil

	return closeTransientBadgerDB(db, s.path)
}

func (s *BadgerMarkSet) SetConcurrent() {}

func openTransientBadgerDB(path string) (*badger.DB, error) {
	// clean up first
	err := os.RemoveAll(path)
	if err != nil {
		return nil, xerrors.Errorf("error clearing markset directory: %w", err)
	}

	err = os.MkdirAll(path, 0755) //nolint:gosec
	if err != nil {
		return nil, xerrors.Errorf("error creating markset directory: %w", err)
	}

	opts := badger.DefaultOptions(path)
	opts.SyncWrites = false
	opts.CompactL0OnClose = false
	opts.Compression = options.None
	// Note: We use FileIO for loading modes to avoid memory thrashing and interference
	//       between the system blockstore and the markset.
	//       It was observed that using the default memory mapped option resulted in
	//       significant interference and unacceptably high block validation times once the markset
	//       exceeded 1GB in size.
	opts.TableLoadingMode = options.FileIO
	opts.ValueLogLoadingMode = options.FileIO
	opts.Logger = &badgerLogger{
		SugaredLogger: log.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar(),
		skip2:         log.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
	}

	return badger.Open(opts)
}

func closeTransientBadgerDB(db *badger.DB, path string) error {
	err := db.Close()
	if err != nil {
		return xerrors.Errorf("error closing badger markset: %w", err)
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
