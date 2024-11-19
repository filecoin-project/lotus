package storiface

import (
	"strings"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
)

// ID identifies sector storage by UUID. One sector storage should map to one
//
//	filesystem, local or networked / shared by multiple machines
type ID string

const IDSep = "."

type IDList []ID

func (il IDList) String() string {
	l := make([]string, len(il))
	for i, id := range il {
		l[i] = string(id)
	}
	return strings.Join(l, IDSep)
}

func ParseIDList(s string) IDList {
	strs := strings.Split(s, IDSep)
	out := make([]ID, len(strs))
	for i, str := range strs {
		out[i] = ID(str)
	}
	return out
}

type Group = string

type StorageInfo struct {
	// ID is the UUID of the storage path
	ID ID

	// URLs for remote access
	URLs []string // TODO: Support non-http transports

	// Storage path weight; higher number means that the path will be preferred more often
	Weight uint64

	// MaxStorage is the number of bytes allowed to be used by files in the
	// storage path
	MaxStorage uint64

	// CanSeal is true when the path is allowed to be used for io-intensive
	// sealing operations
	CanSeal bool

	// CanStore is true when the path is allowed to be used for long-term storage
	CanStore bool

	// Groups is the list of path groups this path belongs to
	Groups []Group

	// AllowTo is the list of paths to which data from this path can be moved to
	AllowTo []Group

	// AllowTypes lists sector file types which are allowed to be put into this
	// path. If empty, all file types are allowed.
	//
	// Valid values:
	// - "unsealed"
	// - "sealed"
	// - "cache"
	// - "update"
	// - "update-cache"
	// Any other value will generate a warning and be ignored.
	AllowTypes []string

	// DenyTypes lists sector file types which aren't allowed to be put into this
	// path.
	//
	// Valid values:
	// - "unsealed"
	// - "sealed"
	// - "cache"
	// - "update"
	// - "update-cache"
	// Any other value will generate a warning and be ignored.
	DenyTypes []string

	// AllowMiners lists miner IDs which are allowed to store their sector data into
	// this path. If empty, all miner IDs are allowed
	AllowMiners []string

	// DenyMiners lists miner IDs which are denied to store their sector data into
	// this path
	DenyMiners []string
}

type HealthReport struct {
	Stat fsutil.FsStat
	Err  string
}

type SectorStorageInfo struct {
	ID       ID
	URLs     []string // TODO: Support non-http transports
	BaseURLs []string
	Weight   uint64

	CanSeal  bool
	CanStore bool

	Primary bool

	AllowTypes  []string
	DenyTypes   []string
	AllowMiners []string
	DenyMiners  []string
}

type Decl struct {
	abi.SectorID
	SectorFileType
}

type StoragePath struct {
	ID     ID
	Weight uint64

	LocalPath string

	CanSeal  bool
	CanStore bool
}
