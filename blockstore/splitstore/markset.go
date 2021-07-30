package splitstore

import (
	"errors"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
)

var errMarkSetClosed = errors.New("markset closed")

// MarkSet is a utility to keep track of seen CID, and later query for them.
//
// * If the expected dataset is large, it can be backed by a datastore (e.g. bbolt).
// * If a probabilistic result is acceptable, it can be backed by a bloom filter
type MarkSet interface {
	Mark(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Close() error
	SetConcurrent()
}

type MarkSetVisitor interface {
	ObjectVisitor
	Close() error
	SetConcurrent()
}

type MarkSetEnv interface {
	Create(name string, sizeHint int64) (MarkSet, error)
	CreateVisitor(name string, sizeHint int64) (MarkSetVisitor, error)
	Close() error
}

func OpenMarkSetEnv(path string, mtype string) (MarkSetEnv, error) {
	switch mtype {
	case "bloom":
		return NewBloomMarkSetEnv()
	case "map":
		return NewMapMarkSetEnv()
	case "badger":
		return NewBadgerMarkSetEnv(path)
	default:
		return nil, xerrors.Errorf("unknown mark set type %s", mtype)
	}
}
