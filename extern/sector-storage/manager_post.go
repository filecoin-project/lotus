package sectorstorage

import (
	"context"
	"sync"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	"github.com/hashicorp/go-multierror"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/runtime/proof"
)

func (m *Manager) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.SectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	if !m.sched.winningPoStSched.CanSched(ctx) {
		log.Info("GenerateWinningPoSt run at lotus-miner")
		return m.Prover.GenerateWinningPoSt(ctx, minerID, sectorInfo, randomness)
	}
	return m.generateWinningPoSt(ctx, minerID, sectorInfo, randomness)
}

func (m *Manager) generateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.SectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	randomness[31] &= 0x3f
	ps, skipped, done, err := m.PubSectorToPriv(ctx, minerID, sectorInfo, nil, abi.RegisteredSealProof.RegisteredWinningPoStProof)
	if err != nil {
		return nil, err
	}
	defer done()

	if len(skipped) > 0 {
		return nil, xerrors.Errorf("pubSectorToPriv skipped sectors: %+v", skipped)
	}

	var sectorNums []abi.SectorNumber
	for _, s := range ps.Spsi.Values() {
		sectorNums = append(sectorNums, s.SectorNumber)
	}

	postChallenges, err := m.GeneratePoStFallbackSectorChallenges(ctx, ps.Spsi.Values()[0].PoStProofType, minerID, randomness, sectorNums)
	if err != nil {
		return nil, xerrors.Errorf("generating fallback challenges: %v", err)
	}

	var proofs []proof.PoStProof
	err = m.sched.winningPoStSched.Schedule(ctx, false, func(ctx context.Context, w Worker) error {
		out, err := w.GenerateWinningPoSt(ctx, minerID, ps, randomness, postChallenges)
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
	if !m.sched.windowPoStSched.CanSched(ctx) {
		log.Info("GenerateWindowPoSt run at lotus-miner")
		return m.Prover.GenerateWindowPoSt(ctx, minerID, sectorInfo, randomness)
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

	//get window proof type
	proofType, err := abi.RegisteredSealProof.RegisteredWindowPoStProof(sectorInfo[0].SealProof)
	if err != nil {
		return nil, nil, err
	}

	// Get sorted and de-duplicate sectors info
	ps, skipped, done, err := m.PubSectorToPriv(ctx, minerID, sectorInfo, nil, abi.RegisteredSealProof.RegisteredWindowPoStProof)
	if err != nil {
		return nil, nil, xerrors.Errorf("PubSectorToPriv failed: %+v", err)
	}

	if len(skipped) > 0 {
		return nil, skipped, xerrors.Errorf("skipped = %d", len(skipped))
	}

	defer done()
	partitionSectorsCount, err := builtin6.PoStProofWindowPoStPartitionSectors(proofType)
	if err != nil {
		return nil, nil, xerrors.Errorf("get sectors count of partition failed:%+v", err)
	}

	// The partitions number of this batch
	partitionCount := (len(ps.Spsi.Values()) + int(partitionSectorsCount) - 1) / int(partitionSectorsCount)

	log.Infof("generateWindowPoSt len(partitionSectorsCount):%d len(partitionCount):%d \n", partitionSectorsCount, partitionCount)

	var faults []abi.SectorID
	var flk sync.Mutex
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var sectorNums []abi.SectorNumber
	for _, s := range ps.Spsi.Values() {
		sectorNums = append(sectorNums, s.SectorNumber)
	}

	postChallenges, err := m.GeneratePoStFallbackSectorChallenges(ctx, ps.Spsi.Values()[0].PoStProofType, minerID, randomness, sectorNums)
	if err != nil {
		return nil, nil, xerrors.Errorf("generating fallback challenges: %v", err)
	}

	proofList := make([]proof.PoStProof, partitionCount)
	var wg sync.WaitGroup
	for i := 0; i < partitionCount; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			proof, faultsectors, err := m.generatePartitionWindowPost(cctx, minerID, n, int(partitionSectorsCount), partitionCount, ps, randomness, postChallenges)
			if err != nil {
				retErr = multierror.Append(retErr, xerrors.Errorf("partitionCount:%d err:%+v", i, err))
				if len(faultsectors) > 0 {
					log.Errorf("generateWindowPost groupCount:%d, faults:%d, err: %+v", i, len(faultsectors), err)
					flk.Lock()
					faults = append(faults, faultsectors...)
					flk.Unlock()
				}
				cancel()
				return
			}
			proofList[n] = proof
		}(i)
		time.Sleep(1 * time.Second)
	}

	wg.Wait()

	pl := make([]ffi.PartitionProof, 0)
	for i, pp := range proofList {
		pl[i] = ffi.PartitionProof(pp)
	}

	postProofs, err := ffi.MergeWindowPoStPartitionProofs(proofType, pl)
	if err != nil {
		return nil, nil, xerrors.Errorf("merge windowPoSt partition proofs: %v", err)
	}

	if len(faults) > 0 {
		log.Warnf("GenerateWindowPoSt get faults: %d", len(faults))
		return out, faults, retErr
	}

	out = append(out, *postProofs)
	return out, skipped, retErr
}

func (m *Manager) generatePartitionWindowPost(ctx context.Context, minerID abi.ActorID, index int, psc int, groupCount int, ps ffiwrapper.SortedPrivateSectorInfo, randomness abi.PoStRandomness, postChallenges *ffiwrapper.FallbackChallenges) (proof.PoStProof, []abi.SectorID, error) {
	var faults []abi.SectorID

	start := index * psc
	end := (index + 1) * psc
	if index == groupCount-1 {
		end = len(ps.Spsi.Values())
	}

	log.Infow("generateWindowPost", "start", start, "end", end, "index", index)

	privsectors, err := m.SplitSortedPrivateSectorInfo(ctx, ps, start, end)
	if err != nil {
		return proof.PoStProof{}, faults, xerrors.Errorf("generateWindowPost GetScopeSortedPrivateSectorInfo failed: %w", err)
	}

	var result *ffiwrapper.WindowPoStResult
	err = m.sched.windowPoStSched.Schedule(ctx, true, func(ctx context.Context, w Worker) error {
		out, err := w.GenerateWindowPoSt(ctx, minerID, privsectors, index, start, randomness, postChallenges)
		if err != nil {
			return err
		}

		result = &out
		return nil
	})

	if err != nil {
		return proof.PoStProof{}, faults, err
	}

	if len(result.Skipped) > 0 {
		log.Warnf("generateWindowPost partition:%d, get faults:%d", index, len(result.Skipped))
		return proof.PoStProof{}, result.Skipped, xerrors.Errorf("generatePartitionWindowPoStProofs partition:%d get faults:%d", index, len(result.Skipped))
	}

	return result.PoStProofs, faults, err
}
