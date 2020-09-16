package main

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/runtime/proof"
	"github.com/ipfs/go-datastore"
)

type cachingVerifier struct {
	ds      datastore.Datastore
	backend ffiwrapper.Verifier
}

func (cv *cachingVerifier) VerifySeal(svi proof.SealVerifyInfo) (bool, error) {
	svi.MarshalCBOR(nil)
	return cv.backend.VerifySeal(svi)
}
func (cv *cachingVerifier) VerifyWinningPoSt(ctx context.Context, info proof.WinningPoStVerifyInfo) (bool, error) {
	info.MarshalCBOR(nil)
	return cv.backend.VerifyWinningPoSt(ctx, info)
}
func (cv *cachingVerifier) VerifyWindowPoSt(ctx context.Context, info proof.WindowPoStVerifyInfo) (bool, error) {
	info.MarshalCBOR(nil)
	return cv.backend.VerifyWindowPoSt(ctx, info)
}
func (cv *cachingVerifier) GenerateWinningPoStSectorChallenge(ctx context.Context, proofType abi.RegisteredPoStProof, a abi.ActorID, rnd abi.PoStRandomness, u uint64) ([]uint64, error) {
	return cv.backend.GenerateWinningPoStSectorChallenge(ctx, proofType, a, rnd, u)
}

var _ ffiwrapper.Verifier = (*cachingVerifier)(nil)
