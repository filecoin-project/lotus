//go:build cgo

package ffiwrapper

import (
	"context"

	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func (sb *Sealer) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	randomness[31] &= 0x3f
	if len(sectorInfo) == 0 {
		return nil, xerrors.Errorf("must provide sectors for winning post")
	}
	ppt, err := sectorInfo[0].SealProof.RegisteredWinningPoStProof()
	if err != nil {
		return nil, xerrors.Errorf("failed to convert to winning post proof: %w", err)
	}

	privsectors, skipped, done, err := sb.pubExtendedSectorToPriv(ctx, minerID, sectorInfo, nil, ppt) // TODO: FAULTS?
	if err != nil {
		return nil, err
	}
	defer done()
	if len(skipped) > 0 {
		return nil, xerrors.Errorf("pubSectorToPriv skipped sectors: %+v", skipped)
	}

	return ffi.GenerateWinningPoSt(minerID, privsectors, randomness)
}

func (sb *Sealer) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, postProofType abi.RegisteredPoStProof, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, []abi.SectorID, error) {
	randomness[31] &= 0x3f
	privsectors, skipped, done, err := sb.pubExtendedSectorToPriv(ctx, minerID, sectorInfo, nil, postProofType)
	if err != nil {
		return nil, nil, xerrors.Errorf("gathering sector info: %w", err)
	}

	defer done()

	if len(skipped) > 0 {
		return nil, skipped, xerrors.Errorf("pubSectorToPriv skipped some sectors")
	}

	proof, faulty, err := ffi.GenerateWindowPoSt(minerID, privsectors, randomness)

	var faultyIDs []abi.SectorID
	for _, f := range faulty {
		faultyIDs = append(faultyIDs, abi.SectorID{
			Miner:  minerID,
			Number: f,
		})
	}
	return proof, faultyIDs, err
}

func (sb *Sealer) pubExtendedSectorToPriv(ctx context.Context, mid abi.ActorID, sectorInfo []proof.ExtendedSectorInfo, faults []abi.SectorNumber, postProofType abi.RegisteredPoStProof) (ffi.SortedPrivateSectorInfo, []abi.SectorID, func(), error) {
	fmap := map[abi.SectorNumber]struct{}{}
	for _, fault := range faults {
		fmap[fault] = struct{}{}
	}

	var doneFuncs []func()
	done := func() {
		for _, df := range doneFuncs {
			df()
		}
	}

	var skipped []abi.SectorID
	var out []ffi.PrivateSectorInfo
	for _, s := range sectorInfo {
		if _, faulty := fmap[s.SectorNumber]; faulty {
			continue
		}

		sid := storiface.SectorRef{
			ID:        abi.SectorID{Miner: mid, Number: s.SectorNumber},
			ProofType: s.SealProof,
		}
		proveUpdate := s.SectorKey != nil
		var cache string
		var sealed string
		if proveUpdate {
			log.Debugf("Posting over updated sector for sector id: %d", s.SectorNumber)
			paths, d, err := sb.sectors.AcquireSector(ctx, sid, storiface.FTUpdateCache|storiface.FTUpdate, 0, storiface.PathStorage)
			if err != nil {
				log.Warnw("failed to acquire FTUpdateCache and FTUpdate of sector, skipping", "sector", sid.ID, "error", err)
				skipped = append(skipped, sid.ID)
				continue
			}
			doneFuncs = append(doneFuncs, d)
			cache = paths.UpdateCache
			sealed = paths.Update
		} else {
			log.Debugf("Posting over sector key sector for sector id: %d", s.SectorNumber)
			paths, d, err := sb.sectors.AcquireSector(ctx, sid, storiface.FTCache|storiface.FTSealed, 0, storiface.PathStorage)
			if err != nil {
				log.Warnw("failed to acquire FTCache and FTSealed of sector, skipping", "sector", sid.ID, "error", err)
				skipped = append(skipped, sid.ID)
				continue
			}
			doneFuncs = append(doneFuncs, d)
			cache = paths.Cache
			sealed = paths.Sealed
		}

		ffiInfo := proof.SectorInfo{
			SealProof:    s.SealProof,
			SectorNumber: s.SectorNumber,
			SealedCID:    s.SealedCID,
		}
		out = append(out, ffi.PrivateSectorInfo{
			CacheDirPath:     cache,
			PoStProofType:    postProofType,
			SealedSectorPath: sealed,
			SectorInfo:       ffiInfo,
		})
	}

	return ffi.NewSortedPrivateSectorInfo(out...), skipped, done, nil
}

var _ storiface.Verifier = ProofVerifier

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
