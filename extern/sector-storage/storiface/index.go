package storiface

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
)

// ID identifies sector storage by UUID. One sector storage should map to one
//  filesystem, local or networked / shared by multiple machines
type ID string

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
