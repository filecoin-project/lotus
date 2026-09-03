package buildconstants

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	"github.com/filecoin-project/go-state-types/network"
)

// UpgradeHeightUnscheduled parks an upgrade beyond any epoch the chain will reach, marking it
// as not yet scheduled. Networks use it until the upgrade has a real height.
const UpgradeHeightUnscheduled = abi.ChainEpoch(999999999999999)

const solsticeRewardWeightPercent = reward19.Denom / 100

type SolsticeRewardWeightParams struct {
	VStart uint64
	Floor  uint64
	Cap    uint64
}

type SolsticeRewardBootstrapParams struct {
	SWATimelockEpochs                 abi.ChainEpoch
	ConsensusWeightRampDurationEpochs abi.ChainEpoch
	ConsensusWeight                   SolsticeRewardWeightParams
	ServiceWeight                     SolsticeRewardWeightParams
	SWAActor                          address.Address
	SRAActor                          address.Address
	InitialOrchestrator               address.Address
}

// NeutralSolsticeRewardBootstrapParams defines a consensus-only bootstrap with no explicit stream dependencies.
var NeutralSolsticeRewardBootstrapParams = SolsticeRewardBootstrapParams{
	ConsensusWeight: SolsticeRewardWeightParams{
		VStart: reward19.Denom,
		Floor:  reward19.Denom,
		Cap:    reward19.Denom,
	},
	SWAActor: builtin.SystemActorAddr,
}

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
const TestNetworkVersion = network.Version29

/* inline-gen end */
