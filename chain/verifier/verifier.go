package verifier

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"
)

type Verifier interface {
	VerifySeal(proof.SealVerifyInfo) (bool, error)
	VerifyAggregateSeals(aggregate proof.AggregateSealVerifyProofAndInfos) (bool, error)
	VerifyReplicaUpdate(update proof.ReplicaUpdateInfo) (bool, error)
	VerifyWinningPoSt(ctx context.Context, info proof.WinningPoStVerifyInfo) (bool, error)
	VerifyWindowPoSt(ctx context.Context, info proof.WindowPoStVerifyInfo) (bool, error)

	GenerateWinningPoStSectorChallenge(context.Context, abi.RegisteredPoStProof, abi.ActorID, abi.PoStRandomness, uint64) ([]uint64, error)
}
