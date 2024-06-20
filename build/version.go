package build

import (
	"os"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

var CurrentCommit string
var BuildType = buildconstants.BuildType

const (
	BuildDefault      = buildconstants.BuildDefault
	BuildMainnet      = buildconstants.BuildMainnet
	Build2k           = buildconstants.Build2k
	BuildDebug        = buildconstants.BuildDebug
	BuildCalibnet     = buildconstants.BuildCalibnet
	BuildInteropnet   = buildconstants.BuildInteropnet
	BuildButterflynet = buildconstants.BuildButterflynet
)

func BuildTypeString() string {
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
	case BuildButterflynet:
		return "+butterflynet"
	default:
		return "+huh?"
	}
}

// BuildVersion is the local build version
const BuildVersion = "1.27.1-dev"

func UserVersion() string {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion
	}

	return BuildVersion + BuildTypeString() + CurrentCommit
}
