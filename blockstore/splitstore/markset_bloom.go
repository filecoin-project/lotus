package splitstore

import (
	"crypto/rand"

	"golang.org/x/xerrors"

	bbloom "github.com/ipfs/bbloom"
	cid "github.com/ipfs/go-cid"
	blake2b "github.com/minio/blake2b-simd"
)

const (
	BloomFilterMinSize     = 10_000_000
	BloomFilterProbability = 0.01
)

type BloomMarkSetEnv struct{}

var _ MarkSetEnv = (*BloomMarkSetEnv)(nil)

type BloomMarkSet struct {
	salt []byte
	bf   *bbloom.Bloom
}

var _ MarkSet = (*BloomMarkSet)(nil)

func NewBloomMarkSetEnv() (*BloomMarkSetEnv, error) {
	return &BloomMarkSetEnv{}, nil
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

	return &BloomMarkSet{salt: salt, bf: bf}, nil
}

func (e *BloomMarkSetEnv) Close() error {
	return nil
}

func (s *BloomMarkSet) saltedKey(cid cid.Cid) []byte {
	hash := cid.Hash()
	key := make([]byte, len(s.salt)+len(hash))
	n := copy(key, s.salt)
	copy(key[n:], hash)
	rehash := blake2b.Sum256(key)
	return rehash[:]
}

func (s *BloomMarkSet) Mark(cid cid.Cid) error {
	s.bf.Add(s.saltedKey(cid))
	return nil
}

func (s *BloomMarkSet) Has(cid cid.Cid) (bool, error) {
	return s.bf.Has(s.saltedKey(cid)), nil
}

func (s *BloomMarkSet) Close() error {
	return nil
}
