package splitstore

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

type MapMarkSetEnv struct {
	path string
}

var _ MarkSetEnv = (*MapMarkSetEnv)(nil)

type MapMarkSet struct {
	mx  sync.RWMutex
	set map[string]struct{}

	persist bool
	file    *os.File
	buf     *bufio.Writer

	path string
}

var _ MarkSet = (*MapMarkSet)(nil)

func NewMapMarkSetEnv(path string) (*MapMarkSetEnv, error) {
	msPath := filepath.Join(path, "markset.map")
	err := os.MkdirAll(msPath, 0755) //nolint:gosec
	if err != nil {
		return nil, xerrors.Errorf("error creating markset directory: %w", err)
	}

	return &MapMarkSetEnv{path: msPath}, nil
}

func (e *MapMarkSetEnv) New(name string, sizeHint int64) (MarkSet, error) {
	path := filepath.Join(e.path, name)
	return &MapMarkSet{
		set:  make(map[string]struct{}, sizeHint),
		path: path,
	}, nil
}

func (e *MapMarkSetEnv) Recover(name string) (MarkSet, error) {
	path := filepath.Join(e.path, name)
	s := &MapMarkSet{
		set:  make(map[string]struct{}),
		path: path,
	}

	in, err := os.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("error opening markset file for read: %w", err)
	}
	defer in.Close() //nolint:errcheck

	// wrap a buffered reader to make this faster
	buf := bufio.NewReader(in)
	for {
		var sz byte
		if sz, err = buf.ReadByte(); err != nil {
			break
		}

		key := make([]byte, int(sz))
		if _, err = io.ReadFull(buf, key); err != nil {
			break
		}

		s.set[string(key)] = struct{}{}
	}

	if err != io.EOF {
		return nil, xerrors.Errorf("error reading markset file: %w", err)
	}

	file, err := os.OpenFile(s.path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return nil, xerrors.Errorf("error opening markset file for write: %w", err)
	}

	s.persist = true
	s.file = file
	s.buf = bufio.NewWriter(file)

	return s, nil
}

func (e *MapMarkSetEnv) Close() error {
	return nil
}

func (s *MapMarkSet) BeginCriticalSection() error {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.set == nil {
		return errMarkSetClosed
	}

	if s.persist {
		return nil
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return xerrors.Errorf("error opening markset file: %w", err)
	}

	// wrap a buffered writer to make this faster
	s.buf = bufio.NewWriter(file)
	for key := range s.set {
		if err := s.writeKey([]byte(key), false); err != nil {
			_ = file.Close()
			s.buf = nil
			return err
		}
	}
	if err := s.buf.Flush(); err != nil {
		_ = file.Close()
		s.buf = nil
		return xerrors.Errorf("error flushing markset file buffer: %w", err)
	}

	s.file = file
	s.persist = true

	return nil
}

func (s *MapMarkSet) EndCriticalSection() {
	s.mx.Lock()
	defer s.mx.Unlock()

	if !s.persist {
		return
	}

	_ = s.file.Close()
	_ = os.Remove(s.path)
	s.file = nil
	s.buf = nil
	s.persist = false
}

func (s *MapMarkSet) Mark(c cid.Cid) error {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.set == nil {
		return errMarkSetClosed
	}

	hash := c.Hash()
	s.set[string(hash)] = struct{}{}

	if s.persist {
		if err := s.writeKey(hash, true); err != nil {
			return err
		}

		if err := s.file.Sync(); err != nil {
			return xerrors.Errorf("error syncing markset: %w", err)
		}
	}

	return nil
}

func (s *MapMarkSet) MarkMany(batch []cid.Cid) error {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.set == nil {
		return errMarkSetClosed
	}

	for _, c := range batch {
		hash := c.Hash()
		s.set[string(hash)] = struct{}{}

		if s.persist {
			if err := s.writeKey(hash, false); err != nil {
				return err
			}
		}
	}

	if s.persist {
		if err := s.buf.Flush(); err != nil {
			return xerrors.Errorf("error flushing markset buffer to disk: %w", err)
		}

		if err := s.file.Sync(); err != nil {
			return xerrors.Errorf("error syncing markset: %w", err)
		}
	}

	return nil
}

func (s *MapMarkSet) Has(cid cid.Cid) (bool, error) {
	s.mx.RLock()
	defer s.mx.RUnlock()

	if s.set == nil {
		return false, errMarkSetClosed
	}

	_, ok := s.set[string(cid.Hash())]
	return ok, nil
}

func (s *MapMarkSet) Visit(c cid.Cid) (bool, error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.set == nil {
		return false, errMarkSetClosed
	}

	hash := c.Hash()
	key := string(hash)
	if _, ok := s.set[key]; ok {
		return false, nil
	}

	s.set[key] = struct{}{}

	if s.persist {
		if err := s.writeKey(hash, true); err != nil {
			return false, err
		}
		if err := s.file.Sync(); err != nil {
			return false, xerrors.Errorf("error syncing markset: %w", err)
		}
	}

	return true, nil
}

func (s *MapMarkSet) Close() error {
	s.mx.Lock()
	defer s.mx.Unlock()

	if s.set == nil {
		return nil
	}

	s.set = nil

	if s.file != nil {
		if err := s.file.Close(); err != nil {
			log.Warnf("error closing markset file: %s", err)
		}

		if !s.persist {
			if err := os.Remove(s.path); err != nil {
				log.Warnf("error removing markset file: %s", err)
			}
		}
	}

	return nil
}

func (s *MapMarkSet) writeKey(k []byte, flush bool) error {
	if err := s.buf.WriteByte(byte(len(k))); err != nil {
		return xerrors.Errorf("error writing markset key length to disk: %w", err)
	}
	if _, err := s.buf.Write(k); err != nil {
		return xerrors.Errorf("error writing markset key to disk: %w", err)
	}
	if flush {
		if err := s.buf.Flush(); err != nil {
			return xerrors.Errorf("error flushing markset buffer to disk: %w", err)
		}
	}

	return nil
}
