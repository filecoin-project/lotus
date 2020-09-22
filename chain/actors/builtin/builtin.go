package builtin

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"

	smoothing0 "github.com/filecoin-project/specs-actors/actors/util/smoothing"

	"github.com/filecoin-project/go-state-types/network"
)

type Version int

const (
	Version0 = iota
)

// Converts a network version into a specs-actors version.
func VersionForNetwork(version network.Version) Version {
	switch version {
	case network.Version0, network.Version1, network.Version2:
		return Version0
	default:
		panic(fmt.Sprintf("unsupported network version %d", version))
	}
}

// TODO: Why does actors have 2 different versions of this?
type FilterEstimate = smoothing0.FilterEstimate

func FromV0FilterEstimate(v0 smoothing0.FilterEstimate) FilterEstimate {
	return (FilterEstimate)(v0)
}

// Doesn't change between actors v0 and v1
func QAPowerForWeight(size abi.SectorSize, duration abi.ChainEpoch, dealWeight, verifiedWeight abi.DealWeight) abi.StoragePower {
	return miner0.QAPowerForWeight(size, duration, dealWeight, verifiedWeight)
}
