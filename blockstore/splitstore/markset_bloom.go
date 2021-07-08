package splitstore

import (
	"crypto/rand"
	"crypto/sha256"
	"sync"

	"golang.org/x/xerrors"

	bbloom "github.com/ipfs/bbloom"
	cid "github.com/ipfs/go-cid"
)

const (
	BloomFilterMinSize     = 10_000_000
	BloomFilterProbability = 0.01
)

type BloomMarkSetEnv struct {
	ts bool
}

var _ MarkSetEnv = (*BloomMarkSetEnv)(nil)

type BloomMarkSet struct {
	salt []byte
	mx   sync.Mutex
	bf   *bbloom.Bloom
	ts   bool
}

var _ MarkSet = (*BloomMarkSet)(nil)

func NewBloomMarkSetEnv(ts bool) (*BloomMarkSetEnv, error) {
	return &BloomMarkSetEnv{ts: ts}, nil
}

func (e *BloomMarkSetEnv) Create(name string, sizeHint int64) (MarkSet, error) {
	size := int64(BloomFilterMinSize)
	for size < sizeHint {
		size += BloomFilterMinSize
	}

	salt := make([]byte, 4)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, xerrors.Errorf("error reading salt: %w", err)
	}

	bf, err := bbloom.New(float64(size), BloomFilterProbability)
	if err != nil {
		return nil, xerrors.Errorf("error creating bloom filter: %w", err)
	}

	return &BloomMarkSet{salt: salt, bf: bf, ts: e.ts}, nil
}

func (e *BloomMarkSetEnv) Close() error {
	return nil
}

func (s *BloomMarkSet) saltedKey(cid cid.Cid) []byte {
	hash := cid.Hash()
	key := make([]byte, len(s.salt)+len(hash))
	n := copy(key, s.salt)
	copy(key[n:], hash)
	rehash := sha256.Sum256(key)
	return rehash[:]
}

func (s *BloomMarkSet) Mark(cid cid.Cid) error {
	if s.ts {
		s.mx.Lock()
		defer s.mx.Unlock()
	}

	if s.bf == nil {
		return errMarkSetClosed
	}

	s.bf.Add(s.saltedKey(cid))
	return nil
}

func (s *BloomMarkSet) Has(cid cid.Cid) (bool, error) {
	if s.ts {
		s.mx.Lock()
		defer s.mx.Unlock()
	}

	if s.bf == nil {
		return false, errMarkSetClosed
	}

	return s.bf.Has(s.saltedKey(cid)), nil
}

func (s *BloomMarkSet) Close() error {
	if s.ts {
		s.mx.Lock()
		defer s.mx.Unlock()
	}
	s.bf = nil
	return nil
}
