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
	MarkSet
	ObjectVisitor
}

type MarkSetEnv interface {
	// Create creates a new markset within the environment.
	// name is a unique name for this markset, mapped to the filesystem in disk-backed environments
	// sizeHint is a hint about the expected size of the markset
	Create(name string, sizeHint int64) (MarkSet, error)
	// CreateVisitor is like Create, but returns a wider interface that supports atomic visits.
	// It may not be supported by some markset types (e.g. bloom).
	CreateVisitor(name string, sizeHint int64) (MarkSetVisitor, error)
	// SupportsVisitor returns true if the marksets created by this environment support the visitor interface.
	SupportsVisitor() bool
	Close() error
}

func OpenMarkSetEnv(path string, mtype string) (MarkSetEnv, error) {
	switch mtype {
	case "map":
		return NewMapMarkSetEnv()
	case "badger":
		return NewBadgerMarkSetEnv(path)
	default:
		return nil, xerrors.Errorf("unknown mark set type %s", mtype)
	}
}
