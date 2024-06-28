//go:build cgo
// +build cgo

package proofsffi

import (
	"context"

	"go.opencensus.io/trace"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/chain/proofs"
)

var _ proofs.Verifier = ProofVerifier

type proofVerifier struct{}

var ProofVerifier = proofVerifier{}

func (proofVerifier) VerifySeal(info proof.SealVerifyInfo) (bool, error) {
	return ffi.VerifySeal(info)
}

func (proofVerifier) VerifyAggregateSeals(aggregate proof.AggregateSealVerifyProofAndInfos) (bool, error) {
	return ffi.VerifyAggregateSeals(aggregate)
}

func (proofVerifier) VerifyReplicaUpdate(update proof.ReplicaUpdateInfo) (bool, error) {
	return ffi.SectorUpdate.VerifyUpdateProof(update)
}

func (proofVerifier) VerifyWinningPoSt(ctx context.Context, info proof.WinningPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	_, span := trace.StartSpan(ctx, "VerifyWinningPoSt")
	defer span.End()

	return ffi.VerifyWinningPoSt(info)
}

func (proofVerifier) VerifyWindowPoSt(ctx context.Context, info proof.WindowPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	_, span := trace.StartSpan(ctx, "VerifyWindowPoSt")
	defer span.End()

	return ffi.VerifyWindowPoSt(info)
}

func (proofVerifier) GenerateWinningPoStSectorChallenge(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, eligibleSectorCount uint64) ([]uint64, error) {
	randomness[31] &= 0x3f
	return ffi.GenerateWinningPoStSectorChallenge(proofType, minerID, randomness, eligibleSectorCount)
}
