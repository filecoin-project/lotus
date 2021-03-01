package splitstore

import (
	"path/filepath"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
)

type LiveSet interface {
	Mark(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Close() error
}

var markBytes = []byte{}

type LiveSetEnv interface {
	NewLiveSet(name string, sizeHint int64) (LiveSet, error)
	Close() error
}

func NewLiveSetEnv(path string, liveSetType string) (LiveSetEnv, error) {
	switch liveSetType {
	case "", "bloom":
		return NewBloomLiveSetEnv()
	case "bolt":
		return NewBoltLiveSetEnv(filepath.Join(path, "sweep.bolt"))
	default:
		return nil, xerrors.Errorf("unknown live set type %s", liveSetType)
	}
}
