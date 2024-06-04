package sealer

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func (m *Manager) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	if !m.disableBuiltinWinningPoSt && !m.winningPoStSched.CanSched(ctx) {
		// if builtin PoSt isn't disabled, and there are no workers, compute the PoSt locally

		log.Info("GenerateWinningPoSt run at lotus-miner")
		return m.localProver.GenerateWinningPoSt(ctx, minerID, sectorInfo, randomness)
	}
	return m.generateWinningPoSt(ctx, minerID, sectorInfo, randomness)
}

func (m *Manager) generateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	randomness[31] &= 0x3f

	sectorNums := make([]abi.SectorNumber, len(sectorInfo))
	for i, s := range sectorInfo {
		sectorNums[i] = s.SectorNumber
	}

	if len(sectorInfo) == 0 {
		return nil, xerrors.New("generate window post len(sectorInfo)=0")
	}

	spt := sectorInfo[0].SealProof

	ppt, err := spt.RegisteredWinningPoStProof()
	if err != nil {
		return nil, err
	}

	postChallenges, err := ffi.GeneratePoStFallbackSectorChallenges(ppt, minerID, randomness, sectorNums)
	if err != nil {
		return nil, xerrors.Errorf("generating fallback challenges: %v", err)
	}

	sectorChallenges := make([]storiface.PostSectorChallenge, len(sectorInfo))
	for i, s := range sectorInfo {
		sectorChallenges[i] = storiface.PostSectorChallenge{
			SealProof:    s.SealProof,
			SectorNumber: s.SectorNumber,
			SealedCID:    s.SealedCID,
			Challenge:    postChallenges.Challenges[s.SectorNumber],
			Update:       s.SectorKey != nil,
		}
	}

	var proofs []proof.PoStProof
	err = m.winningPoStSched.Schedule(ctx, false, spt, func(ctx context.Context, w Worker) error {
		out, err := w.GenerateWinningPoSt(ctx, ppt, minerID, sectorChallenges, randomness)
		if err != nil {
			return err
		}
		proofs = out
		return nil
	})
	if err != nil {
		return nil, err
	}

	return proofs, nil
}

func (m *Manager) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, postProofType abi.RegisteredPoStProof, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) (proof []proof.PoStProof, skipped []abi.SectorID, err error) {
	if !m.disableBuiltinWindowPoSt && !m.windowPoStSched.CanSched(ctx) {
		// if builtin PoSt isn't disabled, and there are no workers, compute the PoSt locally

		log.Info("GenerateWindowPoSt run at lotus-miner")
		p, s, err := m.localProver.GenerateWindowPoSt(ctx, minerID, postProofType, sectorInfo, randomness)
		if err != nil {
			return p, s, xerrors.Errorf("local prover: %w", err)
		}

		return p, s, nil
	}

	return m.generateWindowPoSt(ctx, minerID, postProofType, sectorInfo, randomness)
}

func dedupeSectorInfo(sectorInfo []proof.ExtendedSectorInfo) []proof.ExtendedSectorInfo {
	out := make([]proof.ExtendedSectorInfo, 0, len(sectorInfo))
	seen := map[abi.SectorNumber]struct{}{}
	for _, info := range sectorInfo {
		if _, seen := seen[info.SectorNumber]; seen {
			continue
		}
		seen[info.SectorNumber] = struct{}{}
		out = append(out, info)
	}
	return out
}

func (m *Manager) generateWindowPoSt(ctx context.Context, minerID abi.ActorID, ppt abi.RegisteredPoStProof, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, []abi.SectorID, error) {
	var retErr error
	randomness[31] &= 0x3f

	out := make([]proof.PoStProof, 0)

	if len(sectorInfo) == 0 {
		return nil, nil, xerrors.New("generate window post len(sectorInfo)=0")
	}

	spt := sectorInfo[0].SealProof

	maxPartitionSize, err := builtin.PoStProofWindowPoStPartitionSectors(ppt) // todo proxy through chain/actors
	if err != nil {
		return nil, nil, xerrors.Errorf("get sectors count of partition failed:%+v", err)
	}

	// We're supplied the list of sectors that the miner actor expects - this
	//  list contains substitutes for skipped sectors - but we don't care about
	//  those for the purpose of the proof, so for things to work, we need to
	//  dedupe here.
	sectorInfo = dedupeSectorInfo(sectorInfo)

	// The partitions number of this batch
	// ceil(sectorInfos / maxPartitionSize)
	partitionCount := uint64((len(sectorInfo) + int(maxPartitionSize) - 1) / int(maxPartitionSize))

	log.Infof("generateWindowPoSt maxPartitionSize:%d partitionCount:%d", maxPartitionSize, partitionCount)

	var skipped []abi.SectorID
	var flk sync.Mutex
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sort.Slice(sectorInfo, func(i, j int) bool {
		return sectorInfo[i].SectorNumber < sectorInfo[j].SectorNumber
	})

	sectorNums := make([]abi.SectorNumber, len(sectorInfo))
	sectorMap := make(map[abi.SectorNumber]proof.ExtendedSectorInfo)
	for i, s := range sectorInfo {
		sectorNums[i] = s.SectorNumber
		sectorMap[s.SectorNumber] = s
	}

	postChallenges, err := ffi.GeneratePoStFallbackSectorChallenges(ppt, minerID, randomness, sectorNums)
	if err != nil {
		return nil, nil, xerrors.Errorf("generating fallback challenges: %v", err)
	}

	proofList := make([]ffi.PartitionProof, partitionCount)
	var wg sync.WaitGroup
	wg.Add(int(partitionCount))

	for partIdx := uint64(0); partIdx < partitionCount; partIdx++ {
		go func(partIdx uint64) {
			defer wg.Done()

			sectors := make([]storiface.PostSectorChallenge, 0)
			for i := uint64(0); i < maxPartitionSize; i++ {
				si := i + partIdx*maxPartitionSize
				if si >= uint64(len(postChallenges.Sectors)) {
					break
				}

				snum := postChallenges.Sectors[si]
				sinfo := sectorMap[snum]

				sectors = append(sectors, storiface.PostSectorChallenge{
					SealProof:    sinfo.SealProof,
					SectorNumber: snum,
					SealedCID:    sinfo.SealedCID,
					Challenge:    postChallenges.Challenges[snum],
					Update:       sinfo.SectorKey != nil,
				})
			}

			p, sk, err := m.generatePartitionWindowPost(cctx, spt, ppt, minerID, int(partIdx), sectors, randomness)
			if err != nil || len(sk) > 0 {
				log.Errorf("generateWindowPost part:%d, skipped:%d, sectors: %d, err: %+v", partIdx, len(sk), len(sectors), err)
				flk.Lock()
				skipped = append(skipped, sk...)

				if err != nil {
					retErr = multierr.Append(retErr, xerrors.Errorf("partitionIndex:%d err:%+v", partIdx, err))
				}
				flk.Unlock()
			}

			proofList[partIdx] = ffi.PartitionProof(p)
		}(partIdx)
	}

	wg.Wait()

	if len(skipped) > 0 {
		return nil, skipped, multierr.Append(xerrors.Errorf("some sectors (%d) were skipped", len(skipped)), retErr)
	}

	postProofs, err := ffi.MergeWindowPoStPartitionProofs(ppt, proofList)
	if err != nil {
		return nil, skipped, xerrors.Errorf("merge windowPoSt partition proofs: %v", err)
	}

	out = append(out, *postProofs)
	return out, skipped, retErr
}

func (m *Manager) generatePartitionWindowPost(ctx context.Context, spt abi.RegisteredSealProof, ppt abi.RegisteredPoStProof, minerID abi.ActorID, partIndex int, sc []storiface.PostSectorChallenge, randomness abi.PoStRandomness) (proof.PoStProof, []abi.SectorID, error) {
	log.Infow("generateWindowPost", "index", partIndex)

	start := time.Now()

	var result storiface.WindowPoStResult
	err := m.windowPoStSched.Schedule(ctx, true, spt, func(ctx context.Context, w Worker) error {
		out, err := w.GenerateWindowPoSt(ctx, ppt, minerID, sc, partIndex, randomness)
		if err != nil {
			return xerrors.Errorf("post worker: %w", err)
		}

		result = out
		return nil
	})

	log.Warnw("generateWindowPost done", "index", partIndex, "skipped", len(result.Skipped), "took", time.Since(start).String(), "err", err)

	return result.PoStProofs, result.Skipped, err
}

func (m *Manager) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]proof.PoStProof, error) {
	panic("worker-level api shouldn't be called at this level")
}

func (m *Manager) GenerateWindowPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte, partitionIdx int) (proof.PoStProof, error) {
	panic("worker-level api shouldn't be called at this level")
}
