package build

import (
	"os"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

// NodeBuildVersion is the local build version of the Lotus daemon
const NodeBuildVersion string = "1.34.4-dev"

func NodeUserVersion() BuildVersion {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion(NodeBuildVersion)
	}

	return BuildVersion(NodeBuildVersion + buildconstants.BuildTypeString() + CurrentCommit)
}

// MinerBuildVersion is the local build version of the Lotus miner
const MinerBuildVersion = "1.34.4-dev"

func MinerUserVersion() BuildVersion {
	if os.Getenv("LOTUS_VERSION_IGNORE_COMMIT") == "1" {
		return BuildVersion(MinerBuildVersion)
	}

	return BuildVersion(MinerBuildVersion + buildconstants.BuildTypeString() + CurrentCommit)
}

var BuildType = buildconstants.BuildType                 // Deprecated: Use buildconstants.BuildType instead
var BuildMainnet = buildconstants.BuildMainnet           // Deprecated: Use buildconstants.BuildMainnet instead
var Build2k = buildconstants.Build2k                     // Deprecated: Use buildconstants.Build2k instead
var BuildDebug = buildconstants.BuildDebug               // Deprecated: Use buildconstants.BuildDebug instead
var BuildCalibnet = buildconstants.BuildCalibnet         // Deprecated: Use buildconstants.BuildCalibnet instead
var BuildInteropnet = buildconstants.BuildInteropnet     // Deprecated: Use buildconstants.BuildInteropnet instead
var BuildButterflynet = buildconstants.BuildButterflynet // Deprecated: Use buildconstants.BuildButterflynet instead

var BuildTypeString = buildconstants.BuildTypeString // Deprecated: Use buildconstants.BuildTypeString instead

type BuildVersion string

var CurrentCommit string
