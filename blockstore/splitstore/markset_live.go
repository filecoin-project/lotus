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

// liveMarkSet is a MarkSet whose Visit understands codecs.
//
// Marks are keyed by multihash, but only DAG-CBOR is scanned for links. A mark placed through a
// codec without links (as raw bytes identical to a DAG-CBOR block would be) may be shadowing
// that block so its links may never have been walked. Such a multihash stays maybe shadowed until a
// DAG-CBOR visit resolves it, so a multihash is visited at most twice.
//
// This assumes DAG-CBOR is the only codec walks scan; another would need its own resolution.
type liveMarkSet struct {
	MarkSet

	mx sync.RWMutex
	// multihashes marked through a codec without links -> whether that mark may still be hiding an
	// unscanned DAG-CBOR block
	maybeShadowed map[string]bool
}

func newLiveMarkSet(ms MarkSet) *liveMarkSet {
	return &liveMarkSet{MarkSet: ms, maybeShadowed: make(map[string]bool)}
}

func (s *liveMarkSet) Visit(c cid.Cid) (bool, error) {
	_, visit, err := s.markLive(c)
	return visit, err
}

// markLive marks c live, reporting whether the mark is new and whether the walk should visit c.
func (s *liveMarkSet) markLive(c cid.Cid) (fresh, visit bool, err error) {
	key := string(c.Hash())

	if !scansLinks(c) {
		// before marking, a walker that sees the mark must see this
		s.noteShadow(key)

		fresh, err = s.MarkSet.Visit(c)
		return fresh, fresh, err
	}

	fresh, err = s.MarkSet.Visit(c)
	if err != nil || fresh {
		return fresh, fresh, err
	}

	return false, s.resolveShadow(key), nil
}

// walked reports whether c is marked and that its mark is known to not be hiding unscanned links.
func (s *liveMarkSet) walked(c cid.Cid) (bool, error) {
	mark, err := s.Has(c)
	if err != nil || !mark {
		return false, err
	}

	return !scansLinks(c) || !s.isMaybeShadowed(string(c.Hash())), nil
}

// noteShadow records key as maybe shadowed, unless a DAG-CBOR visit has already resolved it.
func (s *liveMarkSet) noteShadow(key string) {
	s.mx.RLock()
	_, ok := s.maybeShadowed[key]
	s.mx.RUnlock()
	if ok {
		return
	}

	s.mx.Lock()
	defer s.mx.Unlock()

	if _, ok := s.maybeShadowed[key]; !ok {
		s.maybeShadowed[key] = true
	}
}

func (s *liveMarkSet) isMaybeShadowed(key string) bool {
	s.mx.RLock()
	defer s.mx.RUnlock()

	return s.maybeShadowed[key]
}

// resolveShadow resolves a maybe shadowed key, reporting whether this caller did so and must
// therefore visit the DAG-CBOR block.
func (s *liveMarkSet) resolveShadow(key string) bool {
	if !s.isMaybeShadowed(key) {
		return false
	}

	s.mx.Lock()
	defer s.mx.Unlock()

	if !s.maybeShadowed[key] {
		return false
	}

	s.maybeShadowed[key] = false
	return true
}

func (s *SplitStore) newLiveMarkSet(name string, sizeHint int64) (*liveMarkSet, error) {
	ms, err := s.markSetEnv.New(name, sizeHint)
	if err != nil {
		return nil, err
	}

	return newLiveMarkSet(ms), nil
}
