package buildconstants

import "github.com/filecoin-project/go-state-types/network"

var BuildType int

const (
	BuildDefault      = 0
	BuildMainnet      = 0x1
	Build2k           = 0x2
	BuildDebug        = 0x3
	BuildCalibnet     = 0x4
	BuildInteropnet   = 0x5
	BuildButterflynet = 0x7
)

var Devnet = true

// Used by tests and some obscure tooling
/* inline-gen template
const TestNetworkVersion = network.Version{{.latestNetworkVersion}}
/* inline-gen start */
const TestNetworkVersion = network.Version23

/* inline-gen end */
