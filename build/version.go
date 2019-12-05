package build

// Version is the local build version, set by build system
const Version = "0.10.0"

// APIVersion is a hex semver version of the rpc api exposed
//
//                   M M  P
//                   A I  A
//                   J N  T
//                   O O  C
//                   R R  H
//                   |\vv/|
//                   vv  vv
const APIVersion = 0x000a01

const (
	MajorMask = 0xff0000
	MinorMask = 0xffff00
	PatchMask = 0xffffff
)

// VersionInts returns (major, minor, patch) versions
func VersionInts(version uint32) (uint32, uint32, uint32) {
	return (version & MajorMask) >> 16, (version & MinorMask) >> 8, version & PatchMask
}
