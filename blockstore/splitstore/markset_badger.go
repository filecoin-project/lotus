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
	writers int
	seqno   int

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

func (e *BadgerMarkSetEnv) Create(name string, sizeHint int64) (MarkSet, error) {
	path := filepath.Join(e.path, name)

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
	opts.Compression = options.None
	opts.Logger = &badgerLogger{
		SugaredLogger: log.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar(),
		skip2:         log.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, xerrors.Errorf("error creating badger markset: %w", err)
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

	pend := s.pend
	seqno := s.seqno
	s.seqno++
	s.writing[seqno] = pend
	s.pend = make(map[string]struct{})
	s.writers++
	s.mx.Unlock()

	defer func() {
		s.mx.Lock()
		defer s.mx.Unlock()

		delete(s.writing, seqno)
		s.writers--
		if s.writers == 0 {
			s.cond.Broadcast()
		}
	}()

	empty := []byte{} // not nil

	batch := s.db.NewWriteBatch()
	defer batch.Cancel()

	for k := range pend {
		if err := batch.Set([]byte(k), empty); err != nil {
			return err
		}
	}

	err := batch.Flush()
	if err != nil {
		return xerrors.Errorf("error flushing batch to badger markset: %w", err)
	}

	return nil
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

	err := db.Close()
	if err != nil {
		return xerrors.Errorf("error closing badger markset: %w", err)
	}

	err = os.RemoveAll(s.path)
	if err != nil {
		return xerrors.Errorf("error deleting badger markset: %w", err)
	}

	return nil
}

func (s *BadgerMarkSet) SetConcurrent() {}

// badger logging through go-log
type badgerLogger struct {
	*zap.SugaredLogger
	skip2 *zap.SugaredLogger
}

func (b *badgerLogger) Warningf(format string, args ...interface{}) {}
func (b *badgerLogger) Infof(format string, args ...interface{})    {}
func (b *badgerLogger) Debugf(format string, args ...interface{})   {}
