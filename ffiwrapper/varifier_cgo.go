//+build cgo

package ffiwrapper

import (
	"context"

	"go.opencensus.io/trace"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"
)

func (sb *SectorBuilder) ComputeElectionPoSt(ctx context.Context, miner abi.ActorID, sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, winners []abi.PoStCandidate) ([]abi.PoStProof, error) {
	challengeSeed[31] = 0

	privsects, err := sb.pubSectorToPriv(ctx, miner, sectorInfo, nil) // TODO: faults
	if err != nil {
		return nil, err
	}

	return ffi.GeneratePoSt(miner, privsects, challengeSeed, winners)
}

func (sb *SectorBuilder) GenerateEPostCandidates(ctx context.Context, miner abi.ActorID, sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]storage.PoStCandidateWithTicket, error) {
	privsectors, err := sb.pubSectorToPriv(ctx, miner, sectorInfo, faults)
	if err != nil {
		return nil, err
	}

	challengeSeed[31] = 0

	challengeCount := ElectionPostChallengeCount(uint64(len(sectorInfo)), uint64(len(faults)))
	pc, err := ffi.GenerateCandidates(miner, challengeSeed, challengeCount, privsectors)
	if err != nil {
		return nil, err
	}

	return ffiToStorageCandidates(pc), nil
}

func ffiToStorageCandidates(pc []ffi.PoStCandidateWithTicket) []storage.PoStCandidateWithTicket {
	out := make([]storage.PoStCandidateWithTicket, len(pc))
	for i := range out {
		out[i] = storage.PoStCandidateWithTicket{
			Candidate: pc[i].Candidate,
			Ticket:    pc[i].Ticket,
		}
	}

	return out
}

var _ Verifier = ProofVerifier

type proofVerifier struct{}

var ProofVerifier = proofVerifier{}

func (proofVerifier) VerifySeal(info abi.SealVerifyInfo) (bool, error) {
	return ffi.VerifySeal(info)
}

func (proofVerifier) VerifyElectionPost(ctx context.Context, info abi.PoStVerifyInfo) (bool, error) {
	return verifyPost(ctx, info)
}

func (proofVerifier) VerifyFallbackPost(ctx context.Context, info abi.PoStVerifyInfo) (bool, error) {
	return verifyPost(ctx, info)
}

func verifyPost(ctx context.Context, info abi.PoStVerifyInfo) (bool, error) {
	_, span := trace.StartSpan(ctx, "VerifyPoSt")
	defer span.End()

	info.Randomness[31] = 0

	return ffi.VerifyPoSt(info)
}
