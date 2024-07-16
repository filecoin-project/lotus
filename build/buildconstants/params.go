package buildconstants

import "github.com/filecoin-project/go-state-types/network"

const (
	BuildDefault = iota
	BuildMainnet
	Build2k
	BuildDebug
	BuildCalibnet
	BuildInteropnet
	unused1
	BuildButterflynet
)

var BuildType int

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

var Devnet = true

// Used by tests and some obscure tooling
/* inline-gen template
const TestNetworkVersion = network.Version{{.latestNetworkVersion}}
/* inline-gen start */
const TestNetworkVersion = network.Version23

/* inline-gen end */
