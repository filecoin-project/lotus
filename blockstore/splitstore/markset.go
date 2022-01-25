package splitstore

import (
	"errors"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
)

var errMarkSetClosed = errors.New("markset closed")

// MarkSet is an interface for tracking CIDs during chain and object walks
type MarkSet interface {
	ObjectVisitor
	Mark(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Close() error
}

type MarkSetEnv interface {
	// Create creates a new markset within the environment.
	// name is a unique name for this markset, mapped to the filesystem in disk-backed environments
	// sizeHint is a hint about the expected size of the markset
	Create(name string, sizeHint int64) (MarkSet, error)
	// Close closes the markset
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
