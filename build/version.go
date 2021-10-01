package build

import "os"

var CurrentCommit string
var BuildType int

const (
	BuildDefault      = 0
	BuildMainnet      = 0x1
	Build2k           = 0x2
	BuildDebug        = 0x3
	BuildCalibnet     = 0x4
	BuildInteropnet   = 0x5
	BuildNerpanet     = 0x6
	BuildButterflynet = 0x7
)

func buildType() string {
	switch BuildType {
	case BuildDefault:
		return ""
	case BuildMainnet:
		return "+mainnet"
	case Build2k:
		return "+2k"
	case BuildDebug:
		return "+debug"
	case BuildCalibnet:
		return "+calibnet"
	case BuildInteropnet:
		return "+interopnet"
	case BuildNerpanet:
		return "+nerpanet"
	case BuildButterflynet:
		return "+butterflynet"
	default:
		return "+huh?"
	}
}

// BuildVersion is the local build version
const BuildVersion = "1.11.3"

func UserVersion() string {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion
	}

	return BuildVersion + buildType() + CurrentCommit
}
