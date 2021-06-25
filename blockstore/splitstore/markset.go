package splitstore

import (
	"path/filepath"
	"sync"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
)

// MarkSet is a utility to keep track of seen CID, and later query for them.
//
// * If the expected dataset is large, it can be backed by a datastore (e.g. bbolt).
// * If a probabilistic result is acceptable, it can be backed by a bloom filter (default).
type MarkSet interface {
	Mark(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Close() error
}

// markBytes is deliberately a non-nil empty byte slice for serialization.
var markBytes = []byte{}

type MarkSetEnv interface {
	Create(name string, sizeHint int64) (MarkSet, error)
	Close() error
}

func OpenMarkSetEnv(path string, mtype string) (MarkSetEnv, error) {
	switch mtype {
	case "", "bloom":
		return NewBloomMarkSetEnv(false)
	case "bloomts":
		return NewBloomMarkSetEnv(true)
	case "map":
		return NewMapMarkSetEnv(false)
	case "mapts":
		return NewMapMarkSetEnv(true)
	case "bolt":
		return NewBoltMarkSetEnv(filepath.Join(path, "markset.bolt"))
	default:
		return nil, xerrors.Errorf("unknown mark set type %s", mtype)
	}
}

type MapMarkSetEnv struct {
	ts bool
}

var _ MarkSetEnv = (*MapMarkSetEnv)(nil)

type MapMarkSet struct {
	mx   sync.Mutex
	cids map[cid.Cid]struct{}

	ts bool
}

var _ MarkSet = (*MapMarkSet)(nil)

func NewMapMarkSetEnv(ts bool) (*MapMarkSetEnv, error) {
	return &MapMarkSetEnv{ts: ts}, nil
}

func (e *MapMarkSetEnv) Create(name string, sizeHint int64) (MarkSet, error) {
	return &MapMarkSet{
		cids: make(map[cid.Cid]struct{}),
		ts:   e.ts,
	}, nil
}

func (e *MapMarkSetEnv) Close() error {
	return nil
}

func (s *MapMarkSet) Mark(cid cid.Cid) error {
	if s.ts {
		s.mx.Lock()
		defer s.mx.Unlock()
	}

	s.cids[cid] = struct{}{}
	return nil
}

func (s *MapMarkSet) Has(cid cid.Cid) (bool, error) {
	if s.ts {
		s.mx.Lock()
		defer s.mx.Unlock()
	}

	_, ok := s.cids[cid]
	return ok, nil
}

func (s *MapMarkSet) Close() error {
	return nil
}
