package api

import (
	"fmt"

	xerrors "golang.org/x/xerrors"
)

type Version uint32

func newVer(major, minor, patch uint8) Version {
	return Version(uint32(major)<<16 | uint32(minor)<<8 | uint32(patch))
}

// Ints returns (major, minor, patch) versions
func (ve Version) Ints() (uint32, uint32, uint32) {
	v := uint32(ve)
	return (v & majorOnlyMask) >> 16, (v & minorOnlyMask) >> 8, v & patchOnlyMask
}

func (ve Version) String() string {
	vmj, vmi, vp := ve.Ints()
	return fmt.Sprintf("%d.%d.%d", vmj, vmi, vp)
}

func (ve Version) EqMajorMinor(v2 Version) bool {
	return ve&minorMask == v2&minorMask
}

type NodeType int

const (
	NodeUnknown NodeType = iota

	NodeFull
	NodeMiner
	NodeWorker
)

var RunningNodeType NodeType

func VersionForType(nodeType NodeType) (Version, error) {
	switch nodeType {
	case NodeFull:
		return FullAPIVersion1, nil
	case NodeMiner:
		return MinerAPIVersion0, nil
	case NodeWorker:
		return WorkerAPIVersion0, nil
	default:
		return Version(0), xerrors.Errorf("unknown node type %d", nodeType)
	}
}

// semver versions of the rpc api exposed
var (
	FullAPIVersion0 = newVer(1, 4, 0)
	FullAPIVersion1 = newVer(2, 1, 0)

	MinerAPIVersion0  = newVer(1, 2, 0)
	WorkerAPIVersion0 = newVer(1, 1, 0)
)

//nolint:varcheck,deadcode
const (
	majorMask = 0xff0000
	minorMask = 0xffff00
	patchMask = 0xffffff

	majorOnlyMask = 0xff0000
	minorOnlyMask = 0x00ff00
	patchOnlyMask = 0x0000ff
)
