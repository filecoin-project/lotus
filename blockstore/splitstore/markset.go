package splitstore

import (
	"errors"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

var errMarkSetClosed = errors.New("markset closed")

// MarkSet is an interface for tracking CIDs during chain and object walks
type MarkSet interface {
	ObjectVisitor
	Mark(cid.Cid) error
	MarkMany([]cid.Cid) error
	Has(cid.Cid) (bool, error)
	Close() error

	// BeginCriticalSection ensures that the markset is persisted to disk for recovery in case
	// of abnormal termination during the critical section span.
	BeginCriticalSection() error
	// EndCriticalSection ends the critical section span.
	EndCriticalSection()
}

type MarkSetEnv interface {
	// New creates a new markset within the environment.
	// name is a unique name for this markset, mapped to the filesystem for on-disk persistence.
	// sizeHint is a hint about the expected size of the markset
	New(name string, sizeHint int64) (MarkSet, error)
	// Recover recovers an existing markset persisted on-disk.
	Recover(name string) (MarkSet, error)
	// Close closes the markset
	Close() error
}

func OpenMarkSetEnv(path string, mtype string) (MarkSetEnv, error) {
	switch mtype {
	case "map":
		return NewMapMarkSetEnv(path)
	case "badger":
		return NewBadgerMarkSetEnv(path)
	default:
		return nil, xerrors.Errorf("unknown mark set type %s", mtype)
	}
}
