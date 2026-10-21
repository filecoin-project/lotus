package sealer

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type moveSelTestWorker struct {
	paths []storiface.StoragePath
}

func (w *moveSelTestWorker) TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return map[sealtasks.TaskType]struct{}{sealtasks.TTFetch: {}}, nil
}

func (w *moveSelTestWorker) Paths(context.Context) ([]storiface.StoragePath, error) {
	return w.paths, nil
}

func (w *moveSelTestWorker) Utilization() float64 {
	return 0
}

func TestMoveSelectorPrefersLocalMove(t *testing.T) {
	ctx := context.Background()
	spt := abi.RegisteredSealProof_StackedDrg2KiBV1_1
	sector := abi.SectorID{Miner: 1000, Number: 1}

	seal := storiface.StoragePath{ID: "seal", Weight: 1, CanSeal: true}
	store := storiface.StoragePath{ID: "store", Weight: 1, CanStore: true}

	index := paths.NewMemIndex(nil)
	for _, p := range []storiface.StoragePath{seal, store} {
		require.NoError(t, index.StorageAttach(ctx, storiface.StorageInfo{
			ID:       p.ID,
			Weight:   p.Weight,
			CanSeal:  p.CanSeal,
			CanStore: p.CanStore,
		}, fsutil.FsStat{
			Capacity:    1 << 40,
			Available:   1 << 40,
			FSAvailable: 1 << 40,
		}))
	}
	require.NoError(t, index.StorageDeclareSector(ctx, seal.ID, sector, storiface.FTSealed, true))
	require.NoError(t, index.StorageDeclareSector(ctx, seal.ID, sector, storiface.FTCache, true))

	both := &moveSelTestWorker{paths: []storiface.StoragePath{seal, store}}
	storeOnly := &moveSelTestWorker{paths: []storiface.StoragePath{store}}
	sealOnly := &moveSelTestWorker{paths: []storiface.StoragePath{seal}}

	for _, allowRemote := range []bool{false, true} {
		t.Run(fmt.Sprintf("allowRemote=%t", allowRemote), func(t *testing.T) {
			sel := newMoveSelector(index, sector, storiface.FTCache|storiface.FTSealed, storiface.PathStorage, sector.Miner, allowRemote)

			// has the sealing path with the sector and a long-term storage path
			ok, pref, err := sel.Ok(ctx, sealtasks.TTFetch, spt, both)
			require.NoError(t, err)
			require.True(t, ok)
			require.True(t, pref)

			// would have to fetch the sector from another worker
			ok, pref, err = sel.Ok(ctx, sealtasks.TTFetch, spt, storeOnly)
			require.NoError(t, err)
			require.Equal(t, allowRemote, ok)
			require.False(t, pref)

			// has nowhere to move the sector to
			ok, pref, err = sel.Ok(ctx, sealtasks.TTFetch, spt, sealOnly)
			require.NoError(t, err)
			require.False(t, ok)
			require.False(t, pref)
		})
	}
}
