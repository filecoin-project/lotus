package sectorstorage

import (
	"context"
	"sort"
	"sync"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/specs-actors/v6/actors/builtin"
	"github.com/filecoin-project/specs-actors/v6/actors/runtime/proof"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

func (m *Manager) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.SectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	if !m.winningPoStSched.CanSched(ctx) {
		log.Info("GenerateWinningPoSt run at lotus-miner")
		return m.localProver.GenerateWinningPoSt(ctx, minerID, sectorInfo, randomness)
	}
	return m.generateWinningPoSt(ctx, minerID, sectorInfo, randomness)
}

func (m *Manager) generateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.SectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	randomness[31] &= 0x3f

	sectorNums := make([]abi.SectorNumber, len(sectorInfo))
	for i, s := range sectorInfo {
		sectorNums[i] = s.SectorNumber
	}

	if len(sectorInfo) == 0 {
		return nil, xerrors.New("generate window post len(sectorInfo)=0")
	}

	ppt, err := sectorInfo[0].SealProof.RegisteredWinningPoStProof()
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
		}
	}

	var proofs []proof.PoStProof
	err = m.winningPoStSched.Schedule(ctx, false, func(ctx context.Context, w Worker) error {
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

func (m *Manager) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.SectorInfo, randomness abi.PoStRandomness) (proof []proof.PoStProof, skipped []abi.SectorID, err error) {
	if !m.windowPoStSched.CanSched(ctx) {
		log.Info("GenerateWindowPoSt run at lotus-miner")
		return m.localProver.GenerateWindowPoSt(ctx, minerID, sectorInfo, randomness)
	}

	return m.generateWindowPoSt(ctx, minerID, sectorInfo, randomness)
}

func (m *Manager) generateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.SectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, []abi.SectorID, error) {
	var retErr error = nil
	randomness[31] &= 0x3f

	out := make([]proof.PoStProof, 0)

	if len(sectorInfo) == 0 {
		return nil, nil, xerrors.New("generate window post len(sectorInfo)=0")
	}

	ppt, err := sectorInfo[0].SealProof.RegisteredWindowPoStProof()
	if err != nil {
		return nil, nil, err
	}

	maxPartitionSize, err := builtin.PoStProofWindowPoStPartitionSectors(ppt) // todo proxy through chain/actors
	if err != nil {
		return nil, nil, xerrors.Errorf("get sectors count of partition failed:%+v", err)
	}

	// The partitions number of this batch
	// ceil(sectorInfos / maxPartitionSize)
	partitionCount := uint64((len(sectorInfo) + int(maxPartitionSize) - 1) / int(maxPartitionSize))

	log.Infof("generateWindowPoSt maxPartitionSize:%d partitionCount:%d \n", maxPartitionSize, partitionCount)

	var skipped []abi.SectorID
	var flk sync.Mutex
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sort.Slice(sectorInfo, func(i, j int) bool {
		return sectorInfo[i].SectorNumber < sectorInfo[j].SectorNumber
	})

	sectorNums := make([]abi.SectorNumber, len(sectorInfo))
	sectorMap := make(map[abi.SectorNumber]proof.SectorInfo)
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
				})
			}

			p, sk, err := m.generatePartitionWindowPost(cctx, ppt, minerID, int(partIdx), sectors, randomness)
			if err != nil {
				retErr = multierror.Append(retErr, xerrors.Errorf("partitionCount:%d err:%+v", partIdx, err))
				if len(sk) > 0 {
					log.Errorf("generateWindowPost groupCount:%d, skipped:%d, err: %+v", partIdx, len(sk), err)
					flk.Lock()
					skipped = append(skipped, sk...)
					flk.Unlock()
				}
				return
			}

			proofList[partIdx] = ffi.PartitionProof(p)
		}(partIdx)
	}

	wg.Wait()

	postProofs, err := ffi.MergeWindowPoStPartitionProofs(ppt, proofList)
	if err != nil {
		return nil, nil, xerrors.Errorf("merge windowPoSt partition proofs: %v", err)
	}

	if len(skipped) > 0 {
		log.Warnf("GenerateWindowPoSt get skipped: %d", len(skipped))
		return out, skipped, retErr
	}

	out = append(out, *postProofs)
	return out, skipped, retErr
}

func (m *Manager) generatePartitionWindowPost(ctx context.Context, ppt abi.RegisteredPoStProof, minerID abi.ActorID, partIndex int, sc []storiface.PostSectorChallenge, randomness abi.PoStRandomness) (proof.PoStProof, []abi.SectorID, error) {
	log.Infow("generateWindowPost", "index", partIndex)

	var result storiface.WindowPoStResult
	err := m.windowPoStSched.Schedule(ctx, true, func(ctx context.Context, w Worker) error {
		out, err := w.GenerateWindowPoSt(ctx, ppt, minerID, sc, partIndex, randomness)
		if err != nil {
			return err
		}

		result = out
		return nil
	})

	log.Warnf("generateWindowPost partition:%d, get skip count:%d", partIndex, len(result.Skipped))

	return result.PoStProofs, result.Skipped, err
}

func (m *Manager) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]proof.PoStProof, error) {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) GenerateWindowPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte, partitionIdx int) (proof.PoStProof, error) {
	//TODO implement me
	panic("implement me")
}
