package itests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func TestPathDetachRedeclare(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	var (
		client   kit.TestFullNode
		miner    kit.TestMiner
		wiw, wdw kit.TestWorker
	)
	ens := kit.NewEnsemble(t, kit.LatestActorsAt(-1)).
		FullNode(&client, kit.ThroughRPC()).
		Miner(&miner, &client, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.PresealSectors(2), kit.NoStorage()).
		Worker(&miner, &wiw, kit.ThroughRPC(), kit.NoStorage(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWinningPoSt})).
		Worker(&miner, &wdw, kit.ThroughRPC(), kit.NoStorage(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt})).
		Start()

	ens.InterconnectAll()

	// check there's only one path
	sps, err := miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)

	var id storiface.ID
	for s := range sps {
		id = s
	}

	local, err := miner.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 1)
	require.Greater(t, len(local[id]), 1)

	oldLocal := local[id]

	// check sectors
	checkSectors(ctx, t, client, miner, 2, 0)

	// detach preseal path
	require.NoError(t, miner.StorageDetachLocal(ctx, local[id]))

	// check that there are no paths, post checks fail
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 0)
	local, err = miner.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 0)

	checkSectors(ctx, t, client, miner, 2, 2)

	// attach a new path
	newId := miner.AddStorage(ctx, t, func(cfg *storiface.LocalStorageMeta) {
		cfg.CanStore = true
	})

	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	local, err = miner.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 1)
	require.Greater(t, len(local[newId]), 1)

	newLocal := local[newId]

	// move sector data to the new path

	// note: dest path already exist so we only want to .Join src
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(oldLocal, "sealed"), newLocal).Run())
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(oldLocal, "cache"), newLocal).Run())
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(oldLocal, "unsealed"), newLocal).Run())

	// check that sector files aren't indexed, post checks fail
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 0)

	// redeclare sectors
	require.NoError(t, miner.StorageRedeclareLocal(ctx, nil, false))

	// check that sector files exist, post checks work
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 2)

	checkSectors(ctx, t, client, miner, 2, 0)

	// remove one sector, one post check fails
	require.NoError(t, os.RemoveAll(filepath.Join(newLocal, "sealed", "s-t01000-0")))
	require.NoError(t, os.RemoveAll(filepath.Join(newLocal, "cache", "s-t01000-0")))
	checkSectors(ctx, t, client, miner, 2, 1)

	// redeclare with no drop, still see sector in the index
	require.NoError(t, miner.StorageRedeclareLocal(ctx, nil, false))

	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 2)

	// redeclare with drop, don't see the sector in the index
	require.NoError(t, miner.StorageRedeclareLocal(ctx, nil, true))

	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 1)
	require.Equal(t, abi.SectorNumber(1), sps[newId][0].SectorID.Number)
}

func TestPathDetachRedeclareWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	var (
		client          kit.TestFullNode
		miner           kit.TestMiner
		wiw, wdw, sealw kit.TestWorker
	)
	ens := kit.NewEnsemble(t, kit.LatestActorsAt(-1)).
		FullNode(&client, kit.ThroughRPC()).
		Miner(&miner, &client, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.PresealSectors(2), kit.NoStorage()).
		Worker(&miner, &wiw, kit.ThroughRPC(), kit.NoStorage(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWinningPoSt})).
		Worker(&miner, &wdw, kit.ThroughRPC(), kit.NoStorage(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt})).
		Worker(&miner, &sealw, kit.ThroughRPC(), kit.NoStorage(), kit.WithSealWorkerTasks).
		Start()

	ens.InterconnectAll()

	// check there's only one path on the miner, none on the worker
	sps, err := miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)

	var id storiface.ID
	for s := range sps {
		id = s
	}

	local, err := miner.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 1)
	require.Greater(t, len(local[id]), 1)

	oldLocal := local[id]

	local, err = sealw.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 0)

	// check sectors
	checkSectors(ctx, t, client, miner, 2, 0)

	// detach preseal path from the miner
	require.NoError(t, miner.StorageDetachLocal(ctx, oldLocal))

	// check that there are no paths, post checks fail
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 0)
	local, err = miner.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 0)

	checkSectors(ctx, t, client, miner, 2, 2)

	// attach a new path
	newId := sealw.AddStorage(ctx, t, func(cfg *storiface.LocalStorageMeta) {
		cfg.CanStore = true
	})

	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	local, err = sealw.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 1)
	require.Greater(t, len(local[newId]), 1)

	newLocalTemp := local[newId]

	// move sector data to the new path

	// note: dest path already exist so we only want to .Join src
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(oldLocal, "sealed"), newLocalTemp).Run())
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(oldLocal, "cache"), newLocalTemp).Run())
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(oldLocal, "unsealed"), newLocalTemp).Run())

	// check that sector files aren't indexed, post checks fail
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 0)

	// redeclare sectors
	require.NoError(t, sealw.StorageRedeclareLocal(ctx, nil, false))

	// check that sector files exist, post checks work
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 2)

	checkSectors(ctx, t, client, miner, 2, 0)

	// drop the path from the worker
	require.NoError(t, sealw.StorageDetachLocal(ctx, newLocalTemp))
	local, err = sealw.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 0)

	// add a new one again, and move the sectors there
	newId = sealw.AddStorage(ctx, t, func(cfg *storiface.LocalStorageMeta) {
		cfg.CanStore = true
	})

	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	local, err = sealw.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 1)
	require.Greater(t, len(local[newId]), 1)

	newLocal := local[newId]

	// move sector data to the new path

	// note: dest path already exist so we only want to .Join src
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(newLocalTemp, "sealed"), newLocal).Run())
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(newLocalTemp, "cache"), newLocal).Run())
	require.NoError(t, exec.Command("cp", "-R", filepath.Join(newLocalTemp, "unsealed"), newLocal).Run())

	// redeclare sectors
	require.NoError(t, sealw.StorageRedeclareLocal(ctx, nil, false))

	// check that sector files exist, post checks work
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 2)

	checkSectors(ctx, t, client, miner, 2, 0)

	// remove one sector, one check fails
	require.NoError(t, os.RemoveAll(filepath.Join(newLocal, "sealed", "s-t01000-0")))
	require.NoError(t, os.RemoveAll(filepath.Join(newLocal, "cache", "s-t01000-0")))
	checkSectors(ctx, t, client, miner, 2, 1)

	// redeclare with no drop, still see sector in the index
	require.NoError(t, sealw.StorageRedeclareLocal(ctx, nil, false))

	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 2)

	// redeclare with drop, don't see the sector in the index
	require.NoError(t, sealw.StorageRedeclareLocal(ctx, nil, true))

	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)
	require.Len(t, sps[newId], 1)
	require.Equal(t, abi.SectorNumber(1), sps[newId][0].SectorID.Number)
}

func TestPathDetachShared(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	var (
		client          kit.TestFullNode
		miner           kit.TestMiner
		wiw, wdw, sealw kit.TestWorker
	)
	ens := kit.NewEnsemble(t, kit.LatestActorsAt(-1)).
		FullNode(&client, kit.ThroughRPC()).
		Miner(&miner, &client, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.PresealSectors(2), kit.NoStorage()).
		Worker(&miner, &wiw, kit.ThroughRPC(), kit.NoStorage(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWinningPoSt})).
		Worker(&miner, &wdw, kit.ThroughRPC(), kit.NoStorage(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt})).
		Worker(&miner, &sealw, kit.ThroughRPC(), kit.NoStorage(), kit.WithSealWorkerTasks).
		Start()

	ens.InterconnectAll()

	// check there's only one path on the miner, none on the worker
	sps, err := miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)

	var id storiface.ID
	for s := range sps {
		id = s
	}

	// check that there's only one URL for the path (provided by the miner node)
	si, err := miner.StorageInfo(ctx, id)
	require.NoError(t, err)
	require.Len(t, si.URLs, 1)

	local, err := miner.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 1)
	require.Greater(t, len(local[id]), 1)

	minerLocal := local[id]

	local, err = sealw.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 0)

	// share the genesis sector path with the worker
	require.NoError(t, sealw.StorageAddLocal(ctx, minerLocal))

	// still just one path, but accessible from two places
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)

	// should see 2 urls now
	si, err = miner.StorageInfo(ctx, id)
	require.NoError(t, err)
	require.Len(t, si.URLs, 2)

	// drop the path from the worker
	require.NoError(t, sealw.StorageDetachLocal(ctx, minerLocal))

	// the path is still registered
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 1)

	// but with just one URL (the miner)
	si, err = miner.StorageInfo(ctx, id)
	require.NoError(t, err)
	require.Len(t, si.URLs, 1)

	// now also drop from the minel and check that the path is gone
	require.NoError(t, miner.StorageDetachLocal(ctx, minerLocal))

	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 0)
}

func checkSectors(ctx context.Context, t *testing.T, api kit.TestFullNode, miner kit.TestMiner, expectChecked, expectBad int) {
	addr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	mid, err := address.IDFromAddress(addr)
	require.NoError(t, err)

	info, err := api.StateMinerInfo(ctx, addr, types.EmptyTSK)
	require.NoError(t, err)

	partitions, err := api.StateMinerPartitions(ctx, addr, 0, types.EmptyTSK)
	require.NoError(t, err)
	par := partitions[0]

	sectorInfos, err := api.StateMinerSectors(ctx, addr, &par.LiveSectors, types.EmptyTSK)
	require.NoError(t, err)

	var tocheck []storiface.SectorRef
	for _, info := range sectorInfos {
		si := abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: info.SectorNumber,
		}

		tocheck = append(tocheck, storiface.SectorRef{
			ProofType: info.SealProof,
			ID:        si,
		})
	}

	require.Len(t, tocheck, expectChecked)

	bad, err := miner.CheckProvable(ctx, info.WindowPoStProofType, tocheck)
	require.NoError(t, err)
	require.Len(t, bad, expectBad)
}
