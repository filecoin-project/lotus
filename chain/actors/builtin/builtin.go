package builtin

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	proof0 "github.com/filecoin-project/specs-actors/actors/runtime/proof"

	smoothing0 "github.com/filecoin-project/specs-actors/actors/util/smoothing"
	smoothing1 "github.com/filecoin-project/specs-actors/v2/actors/util/smoothing"

	"github.com/filecoin-project/go-state-types/network"
)

type Version int

const (
	Version0 = iota
	Version1
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
type SectorInfo = proof0.SectorInfo
type PoStProof = proof0.PoStProof
type FilterEstimate = smoothing0.FilterEstimate

func FromV0FilterEstimate(v0 smoothing0.FilterEstimate) FilterEstimate {
	return (FilterEstimate)(v0)
}

// Doesn't change between actors v0 and v1
func QAPowerForWeight(size abi.SectorSize, duration abi.ChainEpoch, dealWeight, verifiedWeight abi.DealWeight) abi.StoragePower {
	return miner0.QAPowerForWeight(size, duration, dealWeight, verifiedWeight)
}

func FromV1FilterEstimate(v1 smoothing1.FilterEstimate) FilterEstimate {
	return (FilterEstimate)(v1)
}
