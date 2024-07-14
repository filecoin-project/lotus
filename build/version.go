package build

import (
	"os"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

type BuildVersion string

var CurrentCommit string
var BuildType = buildconstants.BuildType                 // Deprecated: Use buildconstants.BuildType instead
var BuildMainnet = buildconstants.BuildMainnet           // Deprecated: Use buildconstants.BuildMainnet instead
var Build2k = buildconstants.Build2k                     // Deprecated: Use buildconstants.Build2k instead
var BuildDebug = buildconstants.BuildDebug               // Deprecated: Use buildconstants.BuildDebug instead
var BuildCalibnet = buildconstants.BuildCalibnet         // Deprecated: Use buildconstants.BuildCalibnet instead
var BuildInteropnet = buildconstants.BuildInteropnet     // Deprecated: Use buildconstants.BuildInteropnet instead
var BuildButterflynet = buildconstants.BuildButterflynet // Deprecated: Use buildconstants.BuildButterflynet instead

func BuildTypeString() string {
	switch BuildType {
	case buildconstants.BuildDefault:
		return ""
	case buildconstants.BuildMainnet:
		return "+mainnet"
	case buildconstants.Build2k:
		return "+2k"
	case buildconstants.BuildDebug:
		return "+debug"
	case buildconstants.BuildCalibnet:
		return "+calibnet"
	case buildconstants.BuildInteropnet:
		return "+interopnet"
	case buildconstants.BuildButterflynet:
		return "+butterflynet"
	default:
		return "+huh?"
	}
}

// NodeBuildVersion is the local build version of the Lotus daemon
const NodeBuildVersion string = "1.28.0-rc5"

func NodeUserVersion() BuildVersion {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion(NodeBuildVersion)
	}

	return BuildVersion(NodeBuildVersion + BuildTypeString() + CurrentCommit)
}

// MinerBuildVersion is the local build version of the Lotus miner
const MinerBuildVersion = "1.28.0-rc5"

func MinerUserVersion() BuildVersion {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion(MinerBuildVersion)
	}

	return BuildVersion(MinerBuildVersion + BuildTypeString() + CurrentCommit)
}
