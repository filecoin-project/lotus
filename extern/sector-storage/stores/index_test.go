package stores

import (
	"context"
	"testing"

	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

func init() {
	logging.SetLogLevel("stores", "DEBUG")
}

func newTestStorage() StorageInfo {
	return StorageInfo{
		ID:       ID(uuid.New().String()),
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

	i := NewIndex()
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

	i := NewIndex()
	stor1 := newTestStorage()
	stor1.AllowTo = []Group{"grp1"}
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

	i := NewIndex()

	stor1 := newTestStorage()
	stor1.AllowTo = []Group{"grp1"}

	stor2 := newTestStorage()
	stor2.Groups = []Group{"grp1"}

	stor3 := newTestStorage()
	stor3.Groups = []Group{"grp2"}

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
