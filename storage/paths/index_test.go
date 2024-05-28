package paths

import (
	"context"
	"testing"

	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func init() {
	_ = logging.SetLogLevel("stores", "DEBUG")
}

func newTestStorage() storiface.StorageInfo {
	return storiface.StorageInfo{
		ID:       storiface.ID(uuid.New().String()),
		CanSeal:  true,
		CanStore: true,
		Groups:   nil,
		AllowTo:  nil,
	}
}

var bigFsStat = fsutil.FsStat{
	Capacity:    1 << 40,
	Available:   1 << 40,
	FSAvailable: 1 << 40,
	Reserved:    0,
	Max:         0,
	Used:        0,
}

const s32g = 32 << 30

func TestFindSimple(t *testing.T) {
	ctx := context.Background()

	i := NewMemIndex(nil)
	stor1 := newTestStorage()
	stor2 := newTestStorage()

	require.NoError(t, i.StorageAttach(ctx, stor1, bigFsStat))
	require.NoError(t, i.StorageAttach(ctx, stor2, bigFsStat))

	s1 := abi.SectorID{
		Miner:  12,
		Number: 34,
	}

	{
		si, err := i.StorageFindSector(ctx, s1, storiface.FTSealed, s32g, true)
		require.NoError(t, err)
		require.Len(t, si, 0)
	}

	require.NoError(t, i.StorageDeclareSector(ctx, stor1.ID, s1, storiface.FTSealed, true))

	{
		si, err := i.StorageFindSector(ctx, s1, storiface.FTSealed, s32g, false)
		require.NoError(t, err)
		require.Len(t, si, 1)
		require.Equal(t, stor1.ID, si[0].ID)
	}

	{
		si, err := i.StorageFindSector(ctx, s1, storiface.FTSealed, s32g, true)
		require.NoError(t, err)
		require.Len(t, si, 2)
	}
}

func TestFindNoAllow(t *testing.T) {
	ctx := context.Background()

	i := NewMemIndex(nil)
	stor1 := newTestStorage()
	stor1.AllowTo = []storiface.Group{"grp1"}
	stor2 := newTestStorage()

	require.NoError(t, i.StorageAttach(ctx, stor1, bigFsStat))
	require.NoError(t, i.StorageAttach(ctx, stor2, bigFsStat))

	s1 := abi.SectorID{
		Miner:  12,
		Number: 34,
	}
	require.NoError(t, i.StorageDeclareSector(ctx, stor1.ID, s1, storiface.FTSealed, true))

	{
		si, err := i.StorageFindSector(ctx, s1, storiface.FTSealed, s32g, false)
		require.NoError(t, err)
		require.Len(t, si, 1)
		require.Equal(t, stor1.ID, si[0].ID)
	}

	{
		si, err := i.StorageFindSector(ctx, s1, storiface.FTSealed, s32g, true)
		require.NoError(t, err)
		require.Len(t, si, 1)
		require.Equal(t, stor1.ID, si[0].ID)
	}
}

func TestFindAllow(t *testing.T) {
	ctx := context.Background()

	i := NewMemIndex(nil)

	stor1 := newTestStorage()
	stor1.AllowTo = []storiface.Group{"grp1"}

	stor2 := newTestStorage()
	stor2.Groups = []storiface.Group{"grp1"}

	stor3 := newTestStorage()
	stor3.Groups = []storiface.Group{"grp2"}

	require.NoError(t, i.StorageAttach(ctx, stor1, bigFsStat))
	require.NoError(t, i.StorageAttach(ctx, stor2, bigFsStat))
	require.NoError(t, i.StorageAttach(ctx, stor3, bigFsStat))

	s1 := abi.SectorID{
		Miner:  12,
		Number: 34,
	}
	require.NoError(t, i.StorageDeclareSector(ctx, stor1.ID, s1, storiface.FTSealed, true))

	{
		si, err := i.StorageFindSector(ctx, s1, storiface.FTSealed, s32g, false)
		require.NoError(t, err)
		require.Len(t, si, 1)
		require.Equal(t, stor1.ID, si[0].ID)
	}

	{
		si, err := i.StorageFindSector(ctx, s1, storiface.FTSealed, s32g, true)
		require.NoError(t, err)
		require.Len(t, si, 2)
		if si[0].ID == stor1.ID {
			require.Equal(t, stor1.ID, si[0].ID)
			require.Equal(t, stor2.ID, si[1].ID)
		} else {
			require.Equal(t, stor1.ID, si[1].ID)
			require.Equal(t, stor2.ID, si[0].ID)
		}
	}
}

func TestStorageBestAlloc(t *testing.T) {
	idx := NewMemIndex(nil)

	dummyStorageInfo := storiface.StorageInfo{
		ID:       storiface.ID("dummy"),
		CanSeal:  true,
		CanStore: true,
		URLs:     []string{"http://localhost:9999/"},
		Weight:   10,
		AllowMiners: []string{
			"t001",
		},
	}

	dummyFsStat := fsutil.FsStat{
		Capacity:  10000000000,
		Available: 7000000000,
		Reserved:  100000000,
		Used:      3000000000,
	}

	err := idx.StorageAttach(context.Background(), dummyStorageInfo, dummyFsStat)
	require.NoError(t, err)

	t.Run("PathSealing", func(t *testing.T) {
		result, err := idx.StorageBestAlloc(context.Background(), storiface.FTUnsealed, 123, storiface.PathSealing, 1)
		require.Equal(t, err, nil)
		require.NotNil(t, result)
		require.Equal(t, len(result), 1)
		require.Equal(t, result[0].ID, dummyStorageInfo.ID)
	})

	t.Run("PathStorage", func(t *testing.T) {
		result, err := idx.StorageBestAlloc(context.Background(), storiface.FTUnsealed, 123, storiface.PathStorage, 1)
		require.Equal(t, err, nil)
		require.NotNil(t, result)
		require.Equal(t, len(result), 1)
		require.Equal(t, result[0].ID, dummyStorageInfo.ID)
	})

	t.Run("NotAllowedMiner", func(t *testing.T) {
		_, err := idx.StorageBestAlloc(context.Background(), storiface.FTUnsealed, 123, storiface.PathSealing, 2)
		require.Error(t, err)
	})

	t.Run("NoAvailableSpace", func(t *testing.T) {
		bigSectorSize := abi.SectorSize(10000000000)
		_, err := idx.StorageBestAlloc(context.Background(), storiface.FTUnsealed, bigSectorSize, storiface.PathSealing, 1)
		require.Error(t, err)
	})

	t.Run("AllowedMiner", func(t *testing.T) {
		_, err := idx.StorageBestAlloc(context.Background(), storiface.FTUnsealed, 123, storiface.PathSealing, 1)
		require.NoError(t, err)
	})

}
