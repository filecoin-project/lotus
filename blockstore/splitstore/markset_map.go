package splitstore

import (
	"sync"

	cid "github.com/ipfs/go-cid"
)

type MapMarkSetEnv struct {
	ts bool
}

var _ MarkSetEnv = (*MapMarkSetEnv)(nil)

type MapMarkSet struct {
	mx  sync.Mutex
	set map[string]struct{}

	ts bool
}

var _ MarkSet = (*MapMarkSet)(nil)

func NewMapMarkSetEnv(ts bool) (*MapMarkSetEnv, error) {
	return &MapMarkSetEnv{ts: ts}, nil
}

func (e *MapMarkSetEnv) Create(name string, sizeHint int64) (MarkSet, error) {
	return &MapMarkSet{
		set: make(map[string]struct{}, sizeHint),
		ts:  e.ts,
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

	s.set[string(cid.Hash())] = struct{}{}
	return nil
}

func (s *MapMarkSet) Has(cid cid.Cid) (bool, error) {
	if s.ts {
		s.mx.Lock()
		defer s.mx.Unlock()
	}

	_, ok := s.set[string(cid.Hash())]
	return ok, nil
}

func (s *MapMarkSet) Close() error {
	if s.ts {
		s.mx.Lock()
		defer s.mx.Unlock()

	}
	s.set = nil
	return nil
}
