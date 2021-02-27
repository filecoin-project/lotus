package splitstore

import (
	"fmt"
	"path/filepath"

	cid "github.com/ipfs/go-cid"
)

type LiveSet interface {
	Mark(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Close() error
}

var markBytes = []byte{}

type LiveSetEnv interface {
	NewLiveSet(name string) (LiveSet, error)
	Close() error
}

func NewLiveSetEnv(path string, useLMDB bool) (LiveSetEnv, error) {
	if useLMDB {
		return NewLMDBLiveSetEnv(filepath.Join(path, "sweep.lmdb"))
	}

	return nil, fmt.Errorf("TODO: non-lmdb livesets")
}
