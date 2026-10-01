package splitstore

import (
	"sync"

	"github.com/ipfs/go-cid"
)

// scansLinks reports whether walks descend into c's links. Only DAG-CBOR is allowed to link to
// additional blocks.
func scansLinks(c cid.Cid) bool {
	return c.Prefix().Codec == cid.DagCBOR
}

// liveMarkSet is a MarkSet with a codec-aware Visit. Marks are keyed by multihash, so a mark
// placed through an unscanned codec, such as raw bytes identical to a DAG-CBOR block, says
// nothing about whether that block's links were walked. The first DAG-CBOR arrival on such a
// mark is granted one visit.
type liveMarkSet struct {
	MarkSet

	mx sync.RWMutex
	// multihashes marked through an unscanned codec -> whether the DAG-CBOR visit was granted
	shadowed map[string]bool
}

func newLiveMarkSet(ms MarkSet) *liveMarkSet {
	return &liveMarkSet{MarkSet: ms, shadowed: make(map[string]bool)}
}

func (s *liveMarkSet) Visit(c cid.Cid) (bool, error) {
	_, visit, err := s.markLive(c)
	return visit, err
}

// markLive marks c live, reporting whether the mark is new and whether the walk should visit c.
func (s *liveMarkSet) markLive(c cid.Cid) (fresh, visit bool, err error) {
	key := string(c.Hash())

	if !scansLinks(c) {
		// before marking: a walker that sees the mark must see this
		s.mx.RLock()
		_, ok := s.shadowed[key]
		s.mx.RUnlock()
		if !ok {
			s.mx.Lock()
			if _, ok := s.shadowed[key]; !ok {
				s.shadowed[key] = false
			}
			s.mx.Unlock()
		}

		fresh, err = s.MarkSet.Visit(c)
		return fresh, fresh, err
	}

	fresh, err = s.MarkSet.Visit(c)
	if err != nil || fresh {
		return fresh, fresh, err
	}

	s.mx.RLock()
	granted, shadowed := s.shadowed[key]
	s.mx.RUnlock()
	if !shadowed || granted {
		return false, false, nil
	}

	s.mx.Lock()
	defer s.mx.Unlock()

	if s.shadowed[key] {
		return false, false, nil
	}

	s.shadowed[key] = true
	return false, true, nil
}

// walked reports whether c is marked and needs no further visit.
func (s *liveMarkSet) walked(c cid.Cid) (bool, error) {
	mark, err := s.Has(c)
	if err != nil || !mark || !scansLinks(c) {
		return mark, err
	}

	s.mx.RLock()
	defer s.mx.RUnlock()

	granted, shadowed := s.shadowed[string(c.Hash())]
	return !shadowed || granted, nil
}

func (s *SplitStore) newLiveMarkSet(name string, sizeHint int64) (*liveMarkSet, error) {
	ms, err := s.markSetEnv.New(name, sizeHint)
	if err != nil {
		return nil, err
	}

	return newLiveMarkSet(ms), nil
}
