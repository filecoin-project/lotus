package build

// Version is the local build version, set by build system
const Version = "0.0.0"

// APIVersion is a hex semver version of the rpc api exposed
//
//                   M M  P
//                   A I  A
//                   J N  T
//                   O O  C
//                   R R  H
//                   |\vv/|
//                   vv  vv
const APIVersion = 0x000001

const (
	MajorMask = 0xff0000
	MinorMask = 0xffff00
	PatchMask = 0xffffff
)