package build

import (
	"os"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

type BuildVersion string

var CurrentCommit string
var BuildType = buildconstants.BuildType // Deprecated: Use buildconstants.BuildType instead

const (
	BuildDefault      = buildconstants.BuildDefault      // Deprecated: Use buildconstants.BuildDefault instead
	BuildMainnet      = buildconstants.BuildMainnet      // Deprecated: Use buildconstants.BuildMainnet instead
	Build2k           = buildconstants.Build2k           // Deprecated: Use buildconstants.Build2k instead
	BuildDebug        = buildconstants.BuildDebug        // Deprecated: Use buildconstants.BuildDebug instead
	BuildCalibnet     = buildconstants.BuildCalibnet     // Deprecated: Use buildconstants.BuildCalibnet instead
	BuildInteropnet   = buildconstants.BuildInteropnet   // Deprecated: Use buildconstants.BuildInteropnet instead
	BuildButterflynet = buildconstants.BuildButterflynet // Deprecated: Use buildconstants.BuildButterflynet instead
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

// NodeBuildVersion is the local build version of the Lotus daemon
const NodeBuildVersion string = "1.27.2-dev"

func NodeUserVersion() BuildVersion {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion(NodeBuildVersion)
	}

	return BuildVersion(NodeBuildVersion + BuildTypeString() + CurrentCommit)
}

// MinerBuildVersion is the local build version of the Lotus miner
const MinerBuildVersion = "1.27.2-dev"

func MinerUserVersion() BuildVersion {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion(MinerBuildVersion)
	}

	return BuildVersion(MinerBuildVersion + BuildTypeString() + CurrentCommit)
}
