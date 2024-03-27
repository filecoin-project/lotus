package storiface

import "github.com/filecoin-project/go-state-types/abi"

type PathType string

const (
	PathStorage PathType = "storage"
	PathSealing PathType = "sealing"
)

type AcquireMode string

const (
	AcquireMove AcquireMode = "move"
	AcquireCopy AcquireMode = "copy"
)

type SectorLock struct {
	Sector abi.SectorID
	Write  [FileTypes]uint
	Read   [FileTypes]uint
}

type SectorLocks struct {
	Locks []SectorLock
}

type AcquireSettings struct {
	Into *PathsWithIDs
}

type AcquireOption func(*AcquireSettings)

func AcquireInto(pathIDs PathsWithIDs) AcquireOption {
	return func(settings *AcquireSettings) {
		settings.Into = &pathIDs
	}
}
