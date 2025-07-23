package storiface

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
)

// FTUnsealed represents an unsealed sector file.
// FTSealed represents a sealed sector file.
// FTCache represents a cache sector file.
// FTUpdate represents an update sector file.
// FTUpdateCache represents an update cache sector file.
// FTPiece represents a Piece Park sector file.
// FileTypes represents the total number of file.
//
// The SectorFileType type is an integer type that represents different sector file.
// It has several methods to manipulate and query the file.
// The String method returns a string representation of the file.
// The Strings method returns a slice of string representations of all the file that are set in the receiver object.
// The AllSet method returns a slice of all the file that are set in the receiver object.
// The Has method checks whether a specific file type is set in the receiver object.
// The SealSpaceUse method calculates the space used by the receiver object in sealing a sector of a given size.
// The SubAllowed method removes selected file from the receiver object based on a list of allowed and denied file.
// The Unset method removes selected file from the receiver object.
// The AnyAllowed method checks whether any file in the receiver object are allowed based on a list of allowed and denied file.
// The Allowed method checks whether all file in the receiver object are allowed based on a list of allowed and denied file.
// The StoreSpaceUse method calculates the space used by the receiver object in storing a sector of a given size.
// The All method returns an array that represents which file are set in the receiver object.
// The IsNone method checks whether the receiver object represents no file.
const (
	// "regular" sectors
	FTUnsealed SectorFileType = 1 << iota
	FTSealed
	FTCache

	// snap
	FTUpdate
	FTUpdateCache

	// Piece Park
	FTPiece

	FileTypes = iota
)

// PathTypes is a slice of SectorFileType that represents different types of sector file paths.
// It contains the following types of sector file paths:
// - FTUnsealed: represents regular unsealed sectors
// - FTSealed: represents sealed sectors
// - FTCache: represents cache sectors
// - FTUpdate: represents snap sectors
// - FTUpdateCache: represents snap cache sectors
// - FTPiece: represents Piece Park sectors
var PathTypes = []SectorFileType{FTUnsealed, FTSealed, FTCache, FTUpdate, FTUpdateCache, FTPiece}

// FTNone represents a sector file type of none. This constant is used in the StorageLock method to specify that a sector should not have any file locked.
// Example usage:
// err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTSealed|storiface.FTUnsealed|storiface.FTCache)
const (
	FTNone SectorFileType = 0
)

// FTAll represents the combination of all available sector file.
// It is a variable of type SectorFileType.
// The value of FTAll is calculated by iterating over the PathTypes slice and using the |= operator to perform a bitwise OR operation on each path type.
// The result is assigned to the variable out and returned.
// FTAll is immediately invoked as a function using the anonymous function syntax, so the result is returned as soon as it is calculated.
var FTAll = func() (out SectorFileType) {
	for _, pathType := range PathTypes {
		out |= pathType
	}
	return out
}()

// FSOverheadDen represents the constant value 10, which is used to calculate the overhead in various storage space utilization calculations.
const FSOverheadDen = 10

// FSOverheadSeal is a map that represents the overheads for different SectorFileType in sectors which are being sealed.
var FSOverheadSeal = map[SectorFileType]int{ // 10x overheads
	FTUnsealed:    FSOverheadDen,
	FTSealed:      FSOverheadDen,
	FTUpdate:      FSOverheadDen,
	FTUpdateCache: FSOverheadDen*2 + 1,
	FTCache:       141, // 11 layers + D(2x ssize) + C + R'
	FTPiece:       FSOverheadDen,
}

// sector size * disk / fs overhead.  FSOverheadDen is like the unit of sector size

// FsOverheadFinalized is a map that represents the finalized overhead for different types of SectorFileType.
// The keys in the map are the SectorFileType values, and the values are integers representing the overhead.
// It is used to calculate the storage space usage for different types of sectors, as shown in the example below:
// The overhead value is retrieved from FsOverheadFinalized by using the SectorFileType value as the key.
// If the overhead value is not found in the map, an error is returned indicating that there is no finalized
// overhead information for the given sector type.
var FsOverheadFinalized = map[SectorFileType]int{
	FTUnsealed:    FSOverheadDen,
	FTSealed:      FSOverheadDen,
	FTUpdate:      FSOverheadDen,
	FTUpdateCache: 1,
	FTCache:       1,
	FTPiece:       FSOverheadDen,
}

// SectorFileType represents the type of a sector file
// TypeFromString converts a string to a SectorFileType
type SectorFileType int

// TypeFromString converts a string representation of a SectorFileType to its corresponding value.
// It returns the SectorFileType and nil error if the string matches one of the existing types.
// If the string does not match any type, it returns 0 and an error.
func TypeFromString(s string) (SectorFileType, error) {
	switch s {
	case "unsealed":
		return FTUnsealed, nil
	case "sealed":
		return FTSealed, nil
	case "cache":
		return FTCache, nil
	case "update":
		return FTUpdate, nil
	case "update-cache":
		return FTUpdateCache, nil
	case "piece":
		return FTPiece, nil
	default:
		return 0, xerrors.Errorf("unknown sector file type '%s'", s)
	}
}

// String returns a string representation of the SectorFileType.
func (t SectorFileType) String() string {
	switch t {
	case FTUnsealed:
		return "unsealed"
	case FTSealed:
		return "sealed"
	case FTCache:
		return "cache"
	case FTUpdate:
		return "update"
	case FTUpdateCache:
		return "update-cache"
	case FTPiece:
		return "piece"
	default:
		return fmt.Sprintf("<unknown %d %v>", t, (t & ((1 << FileTypes) - 1)).Strings())
	}
}

// Strings returns a slice of strings representing the names of the SectorFileType values that are set in the receiver value.
// Example usage:
//
//	fileType := SectorFileType(FTSealed | FTCache)
//	names := fileType.Strings() // names = ["sealed", "cache"]
//	fmt.Println(names)
func (t SectorFileType) Strings() []string {
	var out []string
	for _, fileType := range PathTypes {
		if fileType&t == 0 {
			continue
		}

		out = append(out, fileType.String())
	}
	return out
}

// AllSet returns a slice of SectorFileType values that are set in the SectorFileType receiver value
func (t SectorFileType) AllSet() []SectorFileType {
	var out []SectorFileType
	for _, fileType := range PathTypes {
		if fileType&t == 0 {
			continue
		}

		out = append(out, fileType)
	}
	return out
}

// Has checks if the SectorFileType has a specific singleType.
func (t SectorFileType) Has(singleType SectorFileType) bool {
	return t&singleType == singleType
}

// SealSpaceUse calculates the amount of space needed for sealing the sector
// based on the given sector size. It iterates over the different path types
// and calculates the space needed for each path type using the FSOverheadSeal
// map. The overhead value is multiplied by the sector size and divided by the
// FSOverheadDen constant. The total space needed is accumulated and returned.
// If there is no seal overhead information for a particular path type, an error
// is returned.
//
// Example usage:
//
//	fileType := FTSealed | FTCache
//	sectorSize := abi.SectorSize(32 << 20) // 32 MiB
//	spaceNeeded, err := fileType.SealSpaceUse(sectorSize)
//
// Parameters:
//
//	ssize: The size of the sector
//
// Returns:
//
//	uint64: The amount of space needed for sealing the sector
//	error: If there is no seal overhead information for a path type
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

// SubAllowed takes in two parameters: allowTypes and denyTypes, both of which are slices of strings.
// If allowTypes is not empty, the method sets a denyMask with all bits set to 1, and then iterates over each allowType,
// converting it to a SectorFileType using the TypeFromString function and unsetting the corresponding bit in the denyMask.
// If a string in allowTypes cannot be converted to a valid SectorFileType, it is ignored.
// After processing allowTypes, the method iterates over each denyType, converting it to a SectorFileType using the TypeFromString function
// and setting the corresponding bit in the denyMask.
// If a string in denyTypes cannot be converted to a valid SectorFileType, it is ignored.
// Finally, the method returns the bitwise AND of the original SectorFileType and the denyMask.
// The returned SectorFileType will only allow the types specified in allowTypes and exclude the types specified in denyTypes.
func (t SectorFileType) SubAllowed(allowTypes []string, denyTypes []string) SectorFileType {
	var denyMask SectorFileType // 1s deny

	if len(allowTypes) > 0 {
		denyMask = ^denyMask

		for _, allowType := range allowTypes {
			pt, err := TypeFromString(allowType)
			if err != nil {
				// we've told the user about this already, don't spam logs and ignore
				continue
			}

			denyMask = denyMask & (^pt) // unset allowed types
		}
	}

	for _, denyType := range denyTypes {
		pt, err := TypeFromString(denyType)
		if err != nil {
			// we've told the user about this already, don't spam logs and ignore
			continue
		}
		denyMask |= pt
	}

	return t & denyMask
}

// Unset removes the specified sector file type(s) from the current SectorFileType value.
// It performs a bitwise AND operation between the current value and the bitwise complement of the toUnset value.
// The result is returned as a new SectorFileType value.
// Any bits that are set in toUnset will be cleared in the result.
// Usage: result = value.Unset(typesToUnset)
func (t SectorFileType) Unset(toUnset SectorFileType) SectorFileType {
	return t &^ toUnset
}

// AnyAllowed checks if the SectorFileType has any allowed types and no denied types.
func (t SectorFileType) AnyAllowed(allowTypes []string, denyTypes []string) bool {
	return t.SubAllowed(allowTypes, denyTypes) != t
}

// Allowed checks if the SectorFileType is allowed based on the given allowTypes and denyTypes.
// Returns true if the SectorFileType is allowed, otherwise false.
func (t SectorFileType) Allowed(allowTypes []string, denyTypes []string) bool {
	return t.SubAllowed(allowTypes, denyTypes) == 0
}

// StoreSpaceUse calculates the space used for storing sectors of a specific file type.
// It takes the sector size as input and returns the total space needed in bytes and an error, if any.
// The calculation is based on the finalized overhead information for the file type.
// If the overhead information is not available for a particular file type, an error will be returned.
func (t SectorFileType) StoreSpaceUse(ssize abi.SectorSize) (uint64, error) {
	var need uint64
	for _, pathType := range PathTypes {
		if !t.Has(pathType) {
			continue
		}

		oh, ok := FsOverheadFinalized[pathType]
		if !ok {
			return 0, xerrors.Errorf("no finalized overhead info for %s", pathType)
		}

		need += uint64(oh) * uint64(ssize) / FSOverheadDen
	}

	return need, nil
}

// All returns an array indicating whether each FileTypes flag is set in the SectorFileType.
func (t SectorFileType) All() [FileTypes]bool {
	var out [FileTypes]bool

	for i := range out {
		out[i] = t&(1<<i) > 0
	}

	return out
}

// IsNone checks if the SectorFileType value is equal to zero.
// It returns true if the value is zero, indicating that the type is none.
func (t SectorFileType) IsNone() bool {
	return t == 0
}

// SectorPaths represents the paths for different sector files.
type SectorPaths struct {
	ID abi.SectorID

	Unsealed    string
	Sealed      string
	Cache       string
	Update      string
	UpdateCache string
	Piece       string
}

// HasAllSet checks if all paths of a SectorPaths struct are set for a given SectorFileType.
func (sp SectorPaths) HasAllSet(ft SectorFileType) bool {
	for _, fileType := range ft.AllSet() {
		if PathByType(sp, fileType) == "" {
			return false
		}
	}

	return true
}

// Subset returns a new instance of SectorPaths that contains only the paths specified by the filter SectorFileType.
// It iterates over each fileType in the filter, retrieves the corresponding path from the original SectorPaths instance, and sets it in the new instance.
// Finally, it sets the ID field of the new instance to be the same as the original instance.
func (sp SectorPaths) Subset(filter SectorFileType) SectorPaths {
	var out SectorPaths

	for _, fileType := range filter.AllSet() {
		SetPathByType(&out, fileType, PathByType(sp, fileType))
	}

	out.ID = sp.ID

	return out
}

// ParseSectorID parses a sector ID from a given base name.
// It expects the format "s-t0%d-%d", where the first %d represents the miner ID
// and the second %d represents the sector number.
//
// Parameters:
// - baseName: The base name from which to parse the sector ID.
//
// Returns:
// - abi.SectorID: The parsed sector ID.
// - error: An error if parsing fails.
//
// Example usage:
//
//	id, err := ParseSectorID(baseName)
//	if err != nil {
//	  // handle error
//	}
//	// use id
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

// SectorName returns the name of a sector in the format "s-t0<Miner>-<Number>"
//
// Parameters:
//   - sid: The sector ID
//
// Returns:
//   - The name of the sector as a string
func SectorName(sid abi.SectorID) string {
	return fmt.Sprintf("s-t0%d-%d", sid.Miner, sid.Number)
}

// PathByType returns the path associated with the specified fileType in the given SectorPaths.
// It panics if the requested path type is unknown.
func PathByType(sps SectorPaths, fileType SectorFileType) string {
	switch fileType {
	case FTUnsealed:
		return sps.Unsealed
	case FTSealed:
		return sps.Sealed
	case FTCache:
		return sps.Cache
	case FTUpdate:
		return sps.Update
	case FTUpdateCache:
		return sps.UpdateCache
	case FTPiece:
		return sps.Piece
	}

	panic("requested unknown path type")
}

func SetPathByType(sps *SectorPaths, fileType SectorFileType, p string) {
	switch fileType {
	case FTUnsealed:
		sps.Unsealed = p
	case FTSealed:
		sps.Sealed = p
	case FTCache:
		sps.Cache = p
	case FTUpdate:
		sps.Update = p
	case FTUpdateCache:
		sps.UpdateCache = p
	case FTPiece:
		sps.Piece = p
	}
}

// PathsWithIDs represents paths and IDs for sector files.
type PathsWithIDs struct {
	Paths SectorPaths
	IDs   SectorPaths
}

// HasAllSet checks if all paths and IDs in PathsWithIDs have a corresponding path set for the specified SectorFileType.
// It returns true if all paths and IDs are set, and false otherwise.
func (p PathsWithIDs) HasAllSet(ft SectorFileType) bool {
	return p.Paths.HasAllSet(ft) && p.IDs.HasAllSet(ft)
}
