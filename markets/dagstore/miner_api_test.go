package dagstore

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/dagstore/mount"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	piecestoreimpl "github.com/filecoin-project/go-fil-markets/piecestore/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
)

const unsealedSectorID = abi.SectorNumber(1)
const sealedSectorID = abi.SectorNumber(2)

func TestLotusAccessorFetchUnsealedPiece(t *testing.T) {
	ctx := context.Background()

	cid1, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	unsealedSectorData := "unsealed"
	sealedSectorData := "sealed"
	mockData := map[abi.SectorNumber]string{
		unsealedSectorID: unsealedSectorData,
		sealedSectorID:   sealedSectorData,
	}

	testCases := []struct {
		name        string
		deals       []abi.SectorNumber
		fetchedData string
		isUnsealed  bool

		expectErr bool
	}{{
		// Expect error if there is no deal info for piece CID
		name:      "no deals",
		expectErr: true,
	}, {
		// Expect the API to always fetch the unsealed deal (because it's
		// cheaper than fetching the sealed deal)
		name:        "prefer unsealed deal",
		deals:       []abi.SectorNumber{unsealedSectorID, sealedSectorID},
		fetchedData: unsealedSectorData,
		isUnsealed:  true,
	}, {
		// Expect the API to unseal the data if there are no unsealed deals
		name:        "unseal if necessary",
		deals:       []abi.SectorNumber{sealedSectorID},
		fetchedData: sealedSectorData,
		isUnsealed:  false,
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ps := getPieceStore(t)
			rpn := &mockRPN{
				sectors: mockData,
			}
			api := NewMinerAPI(ps, rpn, 100)
			require.NoError(t, api.Start(ctx))

			// Add deals to piece store
			for _, sectorID := range tc.deals {
				dealInfo := piecestore.DealInfo{
					SectorID: sectorID,
				}
				err = ps.AddDealForPiece(cid1, dealInfo)
				require.NoError(t, err)
			}

			// Fetch the piece
			r, err := api.FetchUnsealedPiece(ctx, cid1)
			if tc.expectErr {
				require.Error(t, err)
				return
			}

			// Check that the returned reader is for the correct piece
			require.NoError(t, err)
			bz, err := io.ReadAll(r)
			require.NoError(t, err)

			require.Equal(t, tc.fetchedData, string(bz))

			uns, err := api.IsUnsealed(ctx, cid1)
			require.NoError(t, err)
			require.Equal(t, tc.isUnsealed, uns)
		})
	}
}

func TestLotusAccessorGetUnpaddedCARSize(t *testing.T) {
	ctx := context.Background()
	cid1, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	ps := getPieceStore(t)
	rpn := &mockRPN{}
	api := NewMinerAPI(ps, rpn, 100)
	require.NoError(t, api.Start(ctx))

	// Add a deal with data Length 10
	dealInfo := piecestore.DealInfo{
		Length: 10,
	}
	err = ps.AddDealForPiece(cid1, dealInfo)
	require.NoError(t, err)

	// Check that the data length is correct
	len, err := api.GetUnpaddedCARSize(ctx, cid1)
	require.NoError(t, err)
	require.EqualValues(t, 10, len)
}

func TestThrottle(t *testing.T) {
	ctx := context.Background()
	cid1, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	ps := getPieceStore(t)
	rpn := &mockRPN{
		sectors: map[abi.SectorNumber]string{
			unsealedSectorID: "foo",
		},
	}
	api := NewMinerAPI(ps, rpn, 3)
	require.NoError(t, api.Start(ctx))

	// Add a deal with data Length 10
	dealInfo := piecestore.DealInfo{
		SectorID: unsealedSectorID,
		Length:   10,
	}
	err = ps.AddDealForPiece(cid1, dealInfo)
	require.NoError(t, err)

	// hold the lock to block.
	rpn.lk.Lock()

	// fetch the piece concurrently.
	errgrp, ctx := errgroup.WithContext(context.Background())
	for i := 0; i < 10; i++ {
		errgrp.Go(func() error {
			r, err := api.FetchUnsealedPiece(ctx, cid1)
			if err == nil {
				_ = r.Close()
			}
			return err
		})
	}

	time.Sleep(500 * time.Millisecond)
	require.EqualValues(t, 3, atomic.LoadInt32(&rpn.calls)) // throttled

	// allow to proceed.
	rpn.lk.Unlock()

	// allow all to finish.
	err = errgrp.Wait()
	require.NoError(t, err)

	require.EqualValues(t, 10, atomic.LoadInt32(&rpn.calls)) // throttled

}

func getPieceStore(t *testing.T) piecestore.PieceStore {
	ps, err := piecestoreimpl.NewPieceStore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	require.NoError(t, err)

	ch := make(chan struct{}, 1)
	ps.OnReady(func(_ error) {
		ch <- struct{}{}
	})

	err = ps.Start(context.Background())
	require.NoError(t, err)
	<-ch
	return ps
}

type mockRPN struct {
	calls   int32        // guarded by atomic
	lk      sync.RWMutex // lock to simulate blocks.
	sectors map[abi.SectorNumber]string
}

func (m *mockRPN) UnsealSector(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (io.ReadCloser, error) {
	return m.UnsealSectorAt(ctx, sectorID, offset, length)
}

func (m *mockRPN) UnsealSectorAt(ctx context.Context, sectorID abi.SectorNumber, pieceOffset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (mount.Reader, error) {
	atomic.AddInt32(&m.calls, 1)
	m.lk.RLock()
	defer m.lk.RUnlock()

	data, ok := m.sectors[sectorID]
	if !ok {
		panic("sector not found")
	}
	return struct {
		io.ReadCloser
		io.ReaderAt
		io.Seeker
	}{
		ReadCloser: io.NopCloser(bytes.NewBuffer([]byte(data[:]))),
	}, nil
}

func (m *mockRPN) IsUnsealed(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (bool, error) {
	return sectorID == unsealedSectorID, nil
}

func (m *mockRPN) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	panic("implement me")
}

func (m *mockRPN) GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error) {
	panic("implement me")
}

func (m *mockRPN) SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *paych.SignedVoucher, proof []byte, expectedAmount abi.TokenAmount, tok shared.TipSetToken) (abi.TokenAmount, error) {
	panic("implement me")
}

func (m *mockRPN) GetRetrievalPricingInput(ctx context.Context, pieceCID cid.Cid, storageDeals []abi.DealID) (retrievalmarket.PricingInput, error) {
	panic("implement me")
}

var _ retrievalmarket.RetrievalProviderNode = (*mockRPN)(nil)
