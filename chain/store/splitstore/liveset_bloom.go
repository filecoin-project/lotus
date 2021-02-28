package splitstore

import (
	"math/rand"

	"golang.org/x/xerrors"

	bbloom "github.com/ipfs/bbloom"
	cid "github.com/ipfs/go-cid"
	blake2b "github.com/minio/blake2b-simd"
)

const (
	BloomFilterSize        = 50_000_000
	BloomFilterProbability = 0.01
)

type BloomLiveSetEnv struct{}

var _ LiveSetEnv = (*BloomLiveSetEnv)(nil)

type BloomLiveSet struct {
	salt []byte
	bf   *bbloom.Bloom
}

var _ LiveSet = (*BloomLiveSet)(nil)

func NewBloomLiveSetEnv() (*BloomLiveSetEnv, error) {
	return &BloomLiveSetEnv{}, nil
}

func (e *BloomLiveSetEnv) NewLiveSet(name string) (LiveSet, error) {
	salt := make([]byte, 4)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, xerrors.Errorf("error reading salt: %w", err)
	}

	bf, err := bbloom.New(float64(BloomFilterSize), float64(BloomFilterProbability))
	if err != nil {
		return nil, xerrors.Errorf("error creating bloom filter: %w", err)
	}

	return &BloomLiveSet{salt: salt, bf: bf}, nil
}

func (e *BloomLiveSetEnv) Close() error {
	return nil
}

func (s *BloomLiveSet) saltedKey(cid cid.Cid) []byte {
	hash := cid.Hash()
	key := make([]byte, len(s.salt)+len(hash))
	n := copy(key, s.salt)
	copy(key[n:], hash)
	rehash := blake2b.Sum256(key)
	return rehash[:]
}

func (s *BloomLiveSet) Mark(cid cid.Cid) error {
	s.bf.Add(s.saltedKey(cid))
	return nil
}

func (s *BloomLiveSet) Has(cid cid.Cid) (bool, error) {
	return s.bf.Has(s.saltedKey(cid)), nil
}

func (s *BloomLiveSet) Close() error {
	return nil
}
