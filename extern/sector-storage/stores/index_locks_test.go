package stores

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
)

var aSector = abi.SectorID{
	Miner:  2,
	Number: 9000,
}

func TestCanLock(t *testing.T) {
	lk := sectorLock{
		r: [FileTypes]uint{},
		w: FTNone,
	}

	require.Equal(t, true, lk.canLock(FTUnsealed, FTNone))
	require.Equal(t, true, lk.canLock(FTNone, FTUnsealed))

	ftAll := FTUnsealed | FTSealed | FTCache

	require.Equal(t, true, lk.canLock(ftAll, FTNone))
	require.Equal(t, true, lk.canLock(FTNone, ftAll))

	lk.r[0] = 1 // unsealed read taken

	require.Equal(t, true, lk.canLock(FTUnsealed, FTNone))
	require.Equal(t, false, lk.canLock(FTNone, FTUnsealed))

	require.Equal(t, true, lk.canLock(ftAll, FTNone))
	require.Equal(t, false, lk.canLock(FTNone, ftAll))

	require.Equal(t, true, lk.canLock(FTNone, FTSealed|FTCache))
	require.Equal(t, true, lk.canLock(FTUnsealed, FTSealed|FTCache))

	lk.r[0] = 0

	lk.w = FTSealed

	require.Equal(t, true, lk.canLock(FTUnsealed, FTNone))
	require.Equal(t, true, lk.canLock(FTNone, FTUnsealed))

	require.Equal(t, false, lk.canLock(FTSealed, FTNone))
	require.Equal(t, false, lk.canLock(FTNone, FTSealed))

	require.Equal(t, false, lk.canLock(ftAll, FTNone))
	require.Equal(t, false, lk.canLock(FTNone, ftAll))
}

func TestIndexLocksSeq(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)

	ilk := &indexLocks{
		locks: map[abi.SectorID]*sectorLock{},
	}

	require.NoError(t, ilk.StorageLock(ctx, aSector, FTNone, FTUnsealed))
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	require.NoError(t, ilk.StorageLock(ctx, aSector, FTNone, FTUnsealed))
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	require.NoError(t, ilk.StorageLock(ctx, aSector, FTNone, FTUnsealed))
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	require.NoError(t, ilk.StorageLock(ctx, aSector, FTUnsealed, FTNone))
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	require.NoError(t, ilk.StorageLock(ctx, aSector, FTNone, FTUnsealed))
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	require.NoError(t, ilk.StorageLock(ctx, aSector, FTNone, FTUnsealed))
	cancel()
}

func TestIndexLocksBlockOn(t *testing.T) {
	test := func(r1 SectorFileType, w1 SectorFileType, r2 SectorFileType, w2 SectorFileType) func(t *testing.T) {
		return func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			ilk := &indexLocks{
				locks: map[abi.SectorID]*sectorLock{},
			}

			require.NoError(t, ilk.StorageLock(ctx, aSector, r1, w1))

			sch := make(chan struct{})
			go func() {
				ctx, cancel := context.WithCancel(context.Background())

				sch <- struct{}{}

				require.NoError(t, ilk.StorageLock(ctx, aSector, r2, w2))
				cancel()

				sch <- struct{}{}
			}()

			<-sch

			select {
			case <-sch:
				t.Fatal("that shouldn't happen")
			case <-time.After(40 * time.Millisecond):
			}

			cancel()

			select {
			case <-sch:
			case <-time.After(time.Second):
				t.Fatal("timed out")
			}
		}
	}

	t.Run("readBlocksWrite", test(FTUnsealed, FTNone, FTNone, FTUnsealed))
	t.Run("writeBlocksRead", test(FTNone, FTUnsealed, FTUnsealed, FTNone))
	t.Run("writeBlocksWrite", test(FTNone, FTUnsealed, FTNone, FTUnsealed))
}

func TestIndexLocksBlockWonR(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ilk := &indexLocks{
		locks: map[abi.SectorID]*sectorLock{},
	}

	require.NoError(t, ilk.StorageLock(ctx, aSector, FTUnsealed, FTNone))

	sch := make(chan struct{})
	go func() {
		ctx, cancel := context.WithCancel(context.Background())

		sch <- struct{}{}

		require.NoError(t, ilk.StorageLock(ctx, aSector, FTNone, FTUnsealed))
		cancel()

		sch <- struct{}{}
	}()

	<-sch

	select {
	case <-sch:
		t.Fatal("that shouldn't happen")
	case <-time.After(40 * time.Millisecond):
	}

	cancel()

	select {
	case <-sch:
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
