package build

// Version is the local build version, set by build system
const Version = "0.1.0"

// APIVersion is a hex semver version of the rpc api exposed
//
//                   M M  P
//                   A I  A
//                   J N  T
//                   O O  C
//                   R R  H
//                   |\vv/|
//                   vv  vv
const APIVersion = 0x000100

const (
	MajorMask = 0xff0000
	MinorMask = 0xffff00
	PatchMask = 0xffffff

	MajorOnlyMask = 0xff0000
	MinorOnlyMask = 0x00ff00
	PatchOnlyMask = 0x0000ff
c)

// VersionInts returns (major, minor, patch) versions
func VersionInts(version uint32) (uint32, uint32, uint32) {
	return (version & MajorOnlyMask) >> 16, (version & MinorOnlyMask) >> 8, version & PatchOnlyMask
}
