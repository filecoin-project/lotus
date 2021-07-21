package dagstore

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

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
		expectErr   bool
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
	}, {
		// Expect the API to unseal the data if there are no unsealed deals
		name:        "unseal if necessary",
		deals:       []abi.SectorNumber{sealedSectorID},
		fetchedData: sealedSectorData,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ps := getPieceStore(t)
			rpn := &mockRPN{
				sectors: mockData,
			}
			api := NewLotusAccessor(ps, rpn)

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
		})
	}
}

func TestLotusAccessorGetUnpaddedCARSize(t *testing.T) {
	ctx := context.Background()
	cid1, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	ps := getPieceStore(t)
	rpn := &mockRPN{}
	api := NewLotusAccessor(ps, rpn)

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

func getPieceStore(t *testing.T) piecestore.PieceStore {
	ps, err := piecestoreimpl.NewPieceStore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	require.NoError(t, err)

	err = ps.Start(context.Background())
	require.NoError(t, err)
	//
	//ready := make(chan error)
	//ps.OnReady(func(err error) {
	//	ready <- err
	//})
	//err = <-ready
	//require.NoError(t, err)
	//
	return ps
}

type mockRPN struct {
	sectors map[abi.SectorNumber]string
}

func (m *mockRPN) UnsealSector(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (io.ReadCloser, error) {
	data, ok := m.sectors[sectorID]
	if !ok {
		panic("sector not found")
	}
	return io.NopCloser(bytes.NewBuffer([]byte(data))), nil
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
