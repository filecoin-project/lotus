package itests

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
	checkSectors(t, ctx, client, miner, 2, 0)

	// detach preseal path
	require.NoError(t, miner.StorageDetachLocal(ctx, local[id]))

	// check that there are no paths, post checks fail
	sps, err = miner.StorageList(ctx)
	require.NoError(t, err)
	require.Len(t, sps, 0)
	local, err = miner.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, local, 0)

	checkSectors(t, ctx, client, miner, 2, 2)

	// attach a new path
	newId := miner.AddStorage(ctx, t, func(cfg *paths.LocalStorageMeta) {
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
	require.NoError(t, exec.Command("cp", "--recursive", filepath.Join(oldLocal, "sealed"), newLocal).Run())
	require.NoError(t, exec.Command("cp", "--recursive", filepath.Join(oldLocal, "cache"), newLocal).Run())
	require.NoError(t, exec.Command("cp", "--recursive", filepath.Join(oldLocal, "unsealed"), newLocal).Run())

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

	checkSectors(t, ctx, client, miner, 2, 0)

	// remove one sector, one post check fails
	require.NoError(t, os.RemoveAll(filepath.Join(newLocal, "sealed", "s-t01000-0")))
	require.NoError(t, os.RemoveAll(filepath.Join(newLocal, "cache", "s-t01000-0")))
	checkSectors(t, ctx, client, miner, 2, 1)

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

func checkSectors(t *testing.T, ctx context.Context, api kit.TestFullNode, miner kit.TestMiner, expectChecked, expectBad int) {
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

	bad, err := miner.CheckProvable(ctx, info.WindowPoStProofType, tocheck, true)
	require.NoError(t, err)
	require.Len(t, bad, expectBad)
}
