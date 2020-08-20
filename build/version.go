package build

import "fmt"

var CurrentCommit string
var BuildType int

const (
	BuildDefault = 0
	Build2k      = 0x1
	BuildDebug   = 0x3
)

func buildType() string {
	switch BuildType {
	case BuildDefault:
		return ""
	case BuildDebug:
		return "+debug"
	case Build2k:
		return "+2k"
	default:
		return "+huh?"
	}
}

// BuildVersion is the local build version, set by build system
const BuildVersion = "0.4.6"

func UserVersion() string {
	return BuildVersion + buildType() + CurrentCommit
}

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

// APIVersion is a semver version of the rpc api exposed
var APIVersion Version = newVer(0, 11, 0)

//nolint:varcheck,deadcode
const (
	majorMask = 0xff0000
	minorMask = 0xffff00
	patchMask = 0xffffff

	majorOnlyMask = 0xff0000
	minorOnlyMask = 0x00ff00
	patchOnlyMask = 0x0000ff
)
