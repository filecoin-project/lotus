package buildconstants

import "github.com/filecoin-project/go-state-types/network"

const (
	BuildDefault = iota
	BuildMainnet
	Build2k
	BuildDebug
	BuildCalibnet
	BuildInteropnet
	unusedFormerNerpanet // removed in https://github.com/filecoin-project/lotus/pull/7373/files#diff-4592eccb93b506c1e7e175be9b631c7ccdeed4c1c5c4173a1ecd6d974e105190L15
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

// The agent string used by the node and reported to other nodes in the network.
const UserAgent = "lotus"

// Used by tests and some obscure tooling
/* inline-gen template
const TestNetworkVersion = network.Version{{.latestNetworkVersion}}
/* inline-gen start */
const TestNetworkVersion = network.Version28

/* inline-gen end */
