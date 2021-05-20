package storiface

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
)

const (
	// FTUnsealed means a bitmask containing only an unsealed sector file.
	FTUnsealed SectorFileType = 1 << iota
	// FTSealed means a bitmask containing only a sealed sector file.
	FTSealed
	// FTCache means a bitmask containing only the index/"cached" files directory used for proof generation
	// for a sealed sector.
	FTCache

	// FileTypes is the number of possible unique values that a SectorFileType bitmask can contain.
	FileTypes = iota
)

// PathTypes is used to iterate over all the possible unique values in a SectorFileType bitmask.
var PathTypes = []SectorFileType{FTUnsealed, FTSealed, FTCache}

const (
	// FTNone means an empty bitmask of type `SectorFileType`.
	FTNone SectorFileType = 0
)

const FSOverheadDen = 10

var FSOverheadSeal = map[SectorFileType]int{ // 10x overheads
	FTUnsealed: FSOverheadDen,
	FTSealed:   FSOverheadDen,
	FTCache:    141, // 11 layers + D(2x ssize) + C + R
}

var FsOverheadFinalized = map[SectorFileType]int{
	FTUnsealed: FSOverheadDen,
	FTSealed:   FSOverheadDen,
	FTCache:    2,
}

// SectorFileType is a bitmask that represents a set of sector file types.
type SectorFileType int

func (t SectorFileType) String() string {
	switch t {
	case FTUnsealed:
		return "unsealed"
	case FTSealed:
		return "sealed"
	case FTCache:
		return "cache"
	default:
		return fmt.Sprintf("<unknown %d>", t)
	}
}

// Has returns true if the bitmask contains the given single file type.
func (t SectorFileType) Has(singleType SectorFileType) bool {
	return t&singleType == singleType
}

// SealSpaceUse returns the total sealing space that will be used to store all file types in `t` for a sector of size `ssize`.
func (t SectorFileType) SealSpaceUse(ssize abi.SectorSize) (uint64, error) {
	var need uint64
	for _, pathType := range PathTypes {
		if !t.Has(pathType) {
			continue
		}

		oh, ok := FSOverheadSeal[pathType]
		if !ok {
			return 0, xerrors.Errorf("no seal overhead info for %s", pathType)
		}

		need += uint64(oh) * uint64(ssize) / FSOverheadDen
	}

	return need, nil
}

// All returns all the file types contained in the bitmask.
func (t SectorFileType) All() [FileTypes]bool {
	var out [FileTypes]bool

	for i := range out {
		out[i] = t&(1<<i) > 0
	}

	return out
}

// SectorPaths is used to store two types of information
// 1. The Storage IDs used to store the Unsealed, Sealed & Cached sector files for the sector with ID `ID`.
// 2. The absolute sector store paths used to store the Unsealed, Sealed & Cached sector files for the sector with ID `ID`.
type SectorPaths struct {
	ID abi.SectorID

	Unsealed string
	Sealed   string
	Cache    string
}

// ParseSectorID extracts the sector ID from the sector name.
func ParseSectorID(baseName string) (abi.SectorID, error) {
	var n abi.SectorNumber
	var mid abi.ActorID
	read, err := fmt.Sscanf(baseName, "s-t0%d-%d", &mid, &n)
	if err != nil {
		return abi.SectorID{}, xerrors.Errorf("sscanf sector name ('%s'): %w", baseName, err)
	}

	if read != 2 {
		return abi.SectorID{}, xerrors.Errorf("parseSectorID expected to scan 2 values, got %d", read)
	}

	return abi.SectorID{
		Miner:  mid,
		Number: n,
	}, nil
}

func SectorName(sid abi.SectorID) string {
	return fmt.Sprintf("s-t0%d-%d", sid.Miner, sid.Number)
}

// PathByType returns the  storageID/absolute sector store path  of the given fileType in the SectorPaths.
func PathByType(sps SectorPaths, fileType SectorFileType) string {
	switch fileType {
	case FTUnsealed:
		return sps.Unsealed
	case FTSealed:
		return sps.Sealed
	case FTCache:
		return sps.Cache
	}

	panic("requested unknown path type")
}

// PathByType sets the  storageID/absolute sector store path of the given fileType in the SectorPaths.
func SetPathByType(sps *SectorPaths, fileType SectorFileType, p string) {
	switch fileType {
	case FTUnsealed:
		sps.Unsealed = p
	case FTSealed:
		sps.Sealed = p
	case FTCache:
		sps.Cache = p
	}
}
