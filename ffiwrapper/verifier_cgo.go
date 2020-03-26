//+build cgo

package ffiwrapper

import (
	"context"
	"golang.org/x/xerrors"

	"go.opencensus.io/trace"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/lotus/storage/sectorstorage/stores"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"
)

func (sb *Sealer) ComputeElectionPoSt(ctx context.Context, miner abi.ActorID, sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, winners []abi.PoStCandidate) ([]abi.PoStProof, error) {
	challengeSeed[31] = 0

	privsects, err := sb.pubSectorToPriv(ctx, miner, sectorInfo, nil) // TODO: faults
	if err != nil {
		return nil, err
	}

	return ffi.GeneratePoSt(miner, privsects, challengeSeed, winners)
}

func (sb *Sealer) GenerateFallbackPoSt(ctx context.Context, miner abi.ActorID, sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) (storage.FallbackPostOut, error) {
	privsectors, err := sb.pubSectorToPriv(ctx, miner, sectorInfo, faults)
	if err != nil {
		return storage.FallbackPostOut{}, err
	}

	challengeCount := fallbackPostChallengeCount(uint64(len(sectorInfo)), uint64(len(faults)))
	challengeSeed[31] = 0

	candidates, err := ffi.GenerateCandidates(miner, challengeSeed, challengeCount, privsectors)
	if err != nil {
		return storage.FallbackPostOut{}, err
	}

	winners := make([]abi.PoStCandidate, len(candidates))
	for idx := range winners {
		winners[idx] = candidates[idx].Candidate
	}

	proof, err := ffi.GeneratePoSt(miner, privsectors, challengeSeed, winners)
	return storage.FallbackPostOut{
		PoStInputs: ffiToStorageCandidates(candidates),
		Proof:      proof,
	}, err
}

func (sb *Sealer) GenerateEPostCandidates(ctx context.Context, miner abi.ActorID, sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]storage.PoStCandidateWithTicket, error) {
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

func (sb *Sealer) pubSectorToPriv(ctx context.Context, mid abi.ActorID, sectorInfo []abi.SectorInfo, faults []abi.SectorNumber) (ffi.SortedPrivateSectorInfo, error) {
	fmap := map[abi.SectorNumber]struct{}{}
	for _, fault := range faults {
		fmap[fault] = struct{}{}
	}

	var out []ffi.PrivateSectorInfo
	for _, s := range sectorInfo {
		if _, faulty := fmap[s.SectorNumber]; faulty {
			continue
		}

		paths, done, err := sb.sectors.AcquireSector(ctx, abi.SectorID{Miner: mid, Number: s.SectorNumber}, stores.FTCache|stores.FTSealed, 0, false)
		if err != nil {
			return ffi.SortedPrivateSectorInfo{}, xerrors.Errorf("acquire sector paths: %w", err)
		}
		done() // TODO: This is a tiny bit suboptimal

		postProofType, err := s.RegisteredProof.RegisteredPoStProof()
		if err != nil {
			return ffi.SortedPrivateSectorInfo{}, xerrors.Errorf("acquiring registered PoSt proof from sector info %+v: %w", s, err)
		}

		out = append(out, ffi.PrivateSectorInfo{
			CacheDirPath:     paths.Cache,
			PoStProofType:    postProofType,
			SealedSectorPath: paths.Sealed,
			SectorInfo:       s,
		})
	}

	return ffi.NewSortedPrivateSectorInfo(out...), nil
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
