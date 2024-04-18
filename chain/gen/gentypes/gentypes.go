package gentypes

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"
)

type WinningPoStProver interface {
	GenerateCandidates(context.Context, abi.PoStRandomness, uint64) ([]uint64, error)
	ComputeProof(context.Context, []proof7.ExtendedSectorInfo, abi.PoStRandomness, abi.ChainEpoch, network.Version) ([]proof7.PoStProof, error)
}
