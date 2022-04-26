package storiface

import (
	"strings"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
)

// ID identifies sector storage by UUID. One sector storage should map to one
//  filesystem, local or networked / shared by multiple machines
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
	ID         ID
	URLs       []string // TODO: Support non-http transports
	Weight     uint64
	MaxStorage uint64

	CanSeal  bool
	CanStore bool

	Groups  []Group
	AllowTo []Group
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
