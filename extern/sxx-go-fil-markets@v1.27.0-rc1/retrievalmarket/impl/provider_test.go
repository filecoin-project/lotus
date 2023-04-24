package retrievalimpl_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dss "github.com/ipfs/go-datastore/sync"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/requestvalidation"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations/maptypes"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestDynamicPricing(t *testing.T) {
	ctx := context.Background()
	expectedAddress := address.TestAddress2

	payloadCID := tut.GenerateCids(1)[0]
	peer1 := peer.ID("peer1")
	peer2 := peer.ID("peer2")

	// differential price per byte
	expectedppbUnVerified := abi.NewTokenAmount(4321)
	expectedppbVerified := abi.NewTokenAmount(2)

	// differential sealing/unsealing price
	expectedUnsealPrice := abi.NewTokenAmount(100)
	expectedUnsealDiscount := abi.NewTokenAmount(1)

	// differential payment interval
	expectedpiPeer1 := uint64(4567)
	expectedpiPeer2 := uint64(20)

	expectedPaymentIntervalIncrease := uint64(100)

	// multiple pieces have the same payload
	expectedPieceCID1 := tut.GenerateCids(1)[0]
	expectedPieceCID2 := tut.GenerateCids(1)[0]

	// sizes
	piece1SizePadded := uint64(1234)
	piece1Size := uint64(abi.PaddedPieceSize(piece1SizePadded).Unpadded())

	piece2SizePadded := uint64(2234)
	piece2Size := uint64(abi.PaddedPieceSize(piece2SizePadded).Unpadded())

	piece1 := piecestore.PieceInfo{
		PieceCID: expectedPieceCID1,
		Deals: []piecestore.DealInfo{
			{
				DealID: abi.DealID(1),
				Length: abi.PaddedPieceSize(piece1SizePadded),
			},
			{
				DealID: abi.DealID(11),
				Length: abi.PaddedPieceSize(piece1SizePadded),
			},
		},
	}

	piece2 := piecestore.PieceInfo{
		PieceCID: expectedPieceCID2,
		Deals: []piecestore.DealInfo{
			{
				DealID: abi.DealID(2),
				Length: abi.PaddedPieceSize(piece2SizePadded),
			},
			{
				DealID: abi.DealID(22),
				Length: abi.PaddedPieceSize(piece2SizePadded),
			},
			{
				DealID: abi.DealID(222),
				Length: abi.PaddedPieceSize(piece2SizePadded),
			},
		},
	}

	dPriceFunc := func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		ask := retrievalmarket.Ask{}

		if dealPricingParams.VerifiedDeal {
			ask.PricePerByte = expectedppbVerified
		} else {
			ask.PricePerByte = expectedppbUnVerified
		}

		if dealPricingParams.Unsealed {
			ask.UnsealPrice = expectedUnsealDiscount
		} else {
			ask.UnsealPrice = expectedUnsealPrice
		}

		fmt.Println("\n client is", dealPricingParams.Client.String())
		if dealPricingParams.Client == peer2 {
			ask.PaymentInterval = expectedpiPeer2
		} else {
			ask.PaymentInterval = expectedpiPeer1
		}
		ask.PaymentIntervalIncrease = expectedPaymentIntervalIncrease

		return ask, nil
	}

	buildProvider := func(
		t *testing.T,
		node *testnodes.TestRetrievalProviderNode,
		sa retrievalmarket.SectorAccessor,
		qs network.RetrievalQueryStream,
		pieceStore piecestore.PieceStore,
		dagStore *tut.MockDagStoreWrapper,
		net *tut.TestRetrievalMarketNetwork,
		pFnc retrievalimpl.RetrievalPricingFunc,
	) retrievalmarket.RetrievalProvider {
		ds := dss.MutexWrap(datastore.NewMapDatastore())
		dt := tut.NewTestDataTransfer()
		c, err := retrievalimpl.NewProvider(expectedAddress, node, sa, net, pieceStore, dagStore, dt, ds, pFnc)
		require.NoError(t, err)
		tut.StartAndWaitForReady(ctx, t, c)
		return c
	}

	readWriteQueryStream := func() *tut.TestRetrievalQueryStream {
		qRead, qWrite := tut.QueryReadWriter()
		qrRead, qrWrite := tut.QueryResponseReadWriter()
		qs := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
			Reader:     qRead,
			Writer:     qWrite,
			RespReader: qrRead,
			RespWriter: qrWrite,
		})
		return qs
	}

	tcs := map[string]struct {
		query              retrievalmarket.Query
		expFunc            func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper)
		nodeFunc           func(n *testnodes.TestRetrievalProviderNode)
		sectorAccessorFunc func(sa *testnodes.TestSectorAccessor)
		peerIdFnc          func(stream *tut.TestRetrievalQueryStream)
		providerFnc        func(provider retrievalmarket.RetrievalProvider)

		pricingFnc retrievalimpl.RetrievalPricingFunc

		expectedPricePerByte            abi.TokenAmount
		expectedPaymentInterval         uint64
		expectedPaymentIntervalIncrease uint64
		expectedUnsealPrice             abi.TokenAmount
		expectedSize                    uint64
	}{
		// Retrieval request for a payloadCid without a pieceCid
		"pieceCid no-op: quote correct price for sealed, unverified, peer1": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.ExpectPricingParams(expectedPieceCID1, []abi.DealID{1, 11, 2, 22, 222})
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbUnVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealPrice,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece1Size,
		},

		"pieceCid no-op: quote correct price for sealed, unverified, peer2": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer2)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.ExpectPricingParams(expectedPieceCID1, []abi.DealID{1, 11, 2, 22, 222})
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbUnVerified,
			expectedPaymentInterval:         expectedpiPeer2,
			expectedUnsealPrice:             expectedUnsealPrice,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece1Size,
		},

		"pieceCid no-op: quote correct price for sealed, verified, peer1": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.MarkVerified()
				n.ExpectPricingParams(expectedPieceCID1, []abi.DealID{1, 11, 2, 22, 222})
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealPrice,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece1Size,
		},

		"pieceCid no-op: quote correct price for unsealed, unverified, peer1": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.ExpectPricingParams(expectedPieceCID2, []abi.DealID{1, 11, 2, 22, 222})
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece2.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				// pieceStore.ExpectCID(payloadCID, expectedCIDInfo)
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbUnVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealDiscount,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece2Size,
		},

		"pieceCid no-op: quote correct price for unsealed, verified, peer1": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.MarkVerified()
				n.ExpectPricingParams(expectedPieceCID2, []abi.DealID{1, 11, 2, 22, 222})
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece2.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealDiscount,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece2Size,
		},

		"pieceCid no-op: quote correct price for unsealed, verified, peer1 using default pricing policy if data transfer fee set to zero for verified deals": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.MarkVerified()
				n.ExpectPricingParams(expectedPieceCID2, []abi.DealID{1, 11, 2, 22, 222})
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece2.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},

			providerFnc: func(provider retrievalmarket.RetrievalProvider) {
				ask := provider.GetAsk()
				ask.PaymentInterval = expectedpiPeer1
				ask.PaymentIntervalIncrease = expectedPaymentIntervalIncrease
				provider.SetAsk(ask)
			},

			pricingFnc: retrievalimpl.DefaultPricingFunc(true),

			expectedPricePerByte:            big.Zero(),
			expectedUnsealPrice:             big.Zero(),
			expectedPaymentInterval:         expectedpiPeer1,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece2Size,
		},

		"pieceCid no-op: quote correct price for unsealed, verified, peer1 using default pricing policy if data transfer fee not set to zero for verified deals": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.MarkVerified()
				n.ExpectPricingParams(expectedPieceCID2, []abi.DealID{1, 11, 2, 22, 222})
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece2.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},

			providerFnc: func(provider retrievalmarket.RetrievalProvider) {
				ask := provider.GetAsk()
				ask.PricePerByte = expectedppbVerified
				ask.PaymentInterval = expectedpiPeer1
				ask.PaymentIntervalIncrease = expectedPaymentIntervalIncrease
				provider.SetAsk(ask)
			},

			pricingFnc: retrievalimpl.DefaultPricingFunc(false),

			expectedPricePerByte:            expectedppbVerified,
			expectedUnsealPrice:             big.Zero(),
			expectedPaymentInterval:         expectedpiPeer1,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece2Size,
		},

		"pieceCid no-op: quote correct price for sealed, verified, peer1 using default pricing policy": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.MarkVerified()
				n.ExpectPricingParams(expectedPieceCID1, []abi.DealID{1, 11, 2, 22, 222})
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {
				ask := provider.GetAsk()
				ask.PricePerByte = expectedppbVerified
				ask.PaymentInterval = expectedpiPeer1
				ask.PaymentIntervalIncrease = expectedPaymentIntervalIncrease
				ask.UnsealPrice = expectedUnsealPrice
				provider.SetAsk(ask)
			},
			pricingFnc: retrievalimpl.DefaultPricingFunc(false),

			expectedPricePerByte:            expectedppbVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealPrice,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece1Size,
		},

		// Retrieval requests for a payloadCid inside a specific piece Cid
		"specific sealed piece Cid, first piece Cid matches: quote correct price for sealed, unverified, peer1": {
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID1},
			},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.ExpectPricingParams(expectedPieceCID1, []abi.DealID{1, 11})
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece2.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbUnVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealPrice,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece1Size,
		},

		"specific sealed piece Cid, second piece Cid matches: quote correct price for sealed, unverified, peer1": {
			// TODO: FIX
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID2},
			},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.ExpectPricingParams(expectedPieceCID2, []abi.DealID{2, 22, 222})
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece1.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID1, piece2)
				// even though we won't need this, all matching pieces for the payloadCID will be queried
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbUnVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealPrice,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece2Size,
		},

		"specific sealed piece Cid, first piece Cid matches: quote correct price for sealed, verified, peer1": {
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID1},
			},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.ExpectPricingParams(expectedPieceCID1, []abi.DealID{1, 11})
				n.MarkVerified()
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece2.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealPrice,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece1Size,
		},

		"specific sealed piece Cid, first piece Cid matches: quote correct price for unsealed, verified, peer1": {
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID1},
			},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.MarkVerified()
				n.ExpectPricingParams(expectedPieceCID1, []abi.DealID{1, 11})
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece1.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbVerified,
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             expectedUnsealDiscount,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece1Size,
		},

		"specific sealed piece Cid, first piece Cid matches: quote correct price for unsealed, verified, peer2": {
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID2},
			},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer2)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.MarkVerified()
				n.ExpectPricingParams(expectedPieceCID2, []abi.DealID{2, 22, 222})
			},
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := piece2.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {},
			pricingFnc:  dPriceFunc,

			expectedPricePerByte:            expectedppbVerified,
			expectedPaymentInterval:         expectedpiPeer2,
			expectedUnsealPrice:             expectedUnsealDiscount,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece2Size,
		},

		"pieceCid no-op: quote correct price for sealed, unverified, peer1 based on a pre-existing ask": {
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			peerIdFnc: func(qs *tut.TestRetrievalQueryStream) {
				qs.SetRemotePeer(peer1)
			},
			nodeFunc: func(n *testnodes.TestRetrievalProviderNode) {
				n.ExpectPricingParams(expectedPieceCID1, []abi.DealID{1, 11, 2, 22, 222})
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID1, piece1)
				pieceStore.ExpectPiece(expectedPieceCID2, piece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID1)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			providerFnc: func(provider retrievalmarket.RetrievalProvider) {
				ask := provider.GetAsk()
				ask.PricePerByte = expectedppbUnVerified
				ask.UnsealPrice = expectedUnsealPrice
				provider.SetAsk(ask)
			},
			pricingFnc: func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
				ask, _ := dPriceFunc(ctx, dealPricingParams)
				ppb := big.Add(ask.PricePerByte, dealPricingParams.CurrentAsk.PricePerByte)
				unseal := big.Add(ask.UnsealPrice, dealPricingParams.CurrentAsk.UnsealPrice)
				ask.PricePerByte = ppb
				ask.UnsealPrice = unseal
				return ask, nil
			},

			expectedPricePerByte:            big.Mul(expectedppbUnVerified, big.NewInt(2)),
			expectedPaymentInterval:         expectedpiPeer1,
			expectedUnsealPrice:             big.Mul(expectedUnsealPrice, big.NewInt(2)),
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedSize:                    piece1Size,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			node := testnodes.NewTestRetrievalProviderNode()
			sectorAccessor := testnodes.NewTestSectorAccessor()
			qs := readWriteQueryStream()
			tc.peerIdFnc(qs)

			err := qs.WriteQuery(tc.query)
			require.NoError(t, err)
			pieceStore := tut.NewTestPieceStore()
			dagStore := tut.NewMockDagStoreWrapper(pieceStore, sectorAccessor)
			tc.nodeFunc(node)
			if tc.sectorAccessorFunc != nil {
				tc.sectorAccessorFunc(sectorAccessor)
			}
			tc.expFunc(t, pieceStore, dagStore)

			net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{})
			p := buildProvider(t, node, sectorAccessor, qs, pieceStore, dagStore, net, tc.pricingFnc)
			tc.providerFnc(p)
			net.ReceiveQueryStream(qs)

			actualResp, err := qs.ReadQueryResponse()
			require.NoError(t, err)
			pieceStore.VerifyExpectations(t)
			node.VerifyExpectations(t)
			sectorAccessor.VerifyExpectations(t)

			require.Equal(t, expectedAddress, actualResp.PaymentAddress)
			require.Equal(t, tc.expectedPricePerByte, actualResp.MinPricePerByte)
			require.Equal(t, tc.expectedUnsealPrice, actualResp.UnsealPrice)
			require.Equal(t, tc.expectedPaymentInterval, actualResp.MaxPaymentInterval)
			require.Equal(t, tc.expectedPaymentIntervalIncrease, actualResp.MaxPaymentIntervalIncrease)
			require.Equal(t, tc.expectedSize, actualResp.Size)
		})
	}
}

func TestHandleQueryStream(t *testing.T) {
	ctx := context.Background()

	payloadCID := tut.GenerateCids(1)[0]
	payloadCID2 := tut.GenerateCids(1)[0]
	expectedPeer := peer.ID("somepeer")
	paddedSize := uint64(1234)
	expectedSize := uint64(abi.PaddedPieceSize(paddedSize).Unpadded())

	paddedSize2 := uint64(2234)
	expectedSize2 := uint64(abi.PaddedPieceSize(paddedSize2).Unpadded())

	identityCidWith1, err := tut.MakeIdentityCidWith([]cid.Cid{payloadCID}, multicodec.DagPb)
	require.NoError(t, err)
	identityCidWithBoth, err := tut.MakeIdentityCidWith([]cid.Cid{payloadCID, payloadCID2}, multicodec.DagCbor, []byte("and some padding"))
	require.NoError(t, err)
	identityCidWithBothNested, err := tut.MakeIdentityCidWith([]cid.Cid{identityCidWith1, payloadCID2}, multicodec.DagPb)
	require.NoError(t, err)
	identityCidWithBogus, err := tut.MakeIdentityCidWith([]cid.Cid{payloadCID, tut.GenerateCids(1)[0]}, multicodec.DagPb)
	require.NoError(t, err)
	identityCidTooBig, err := tut.MakeIdentityCidWith([]cid.Cid{payloadCID}, multicodec.DagCbor, tut.RandomBytes(2048))
	require.NoError(t, err)
	identityCidTooManyLinks, err := tut.MakeIdentityCidWith(tut.GenerateCids(33), multicodec.DagPb)
	require.NoError(t, err)

	expectedPieceCID := tut.GenerateCids(1)[0]
	expectedPieceCID2 := tut.GenerateCids(1)[0]

	expectedPiece := piecestore.PieceInfo{
		PieceCID: expectedPieceCID,
		Deals: []piecestore.DealInfo{
			{
				Length: abi.PaddedPieceSize(paddedSize),
			},
		},
	}

	expectedPiece2 := piecestore.PieceInfo{
		PieceCID: expectedPieceCID2,
		Deals: []piecestore.DealInfo{
			{
				Length: abi.PaddedPieceSize(paddedSize2),
			},
		},
	}

	expectedAddress := address.TestAddress2
	expectedPricePerByte := abi.NewTokenAmount(4321)
	expectedPaymentInterval := uint64(4567)
	expectedPaymentIntervalIncrease := uint64(100)
	expectedUnsealPrice := abi.NewTokenAmount(100)

	// differential pricing
	expectedUnsealDiscount := abi.NewTokenAmount(1)

	readWriteQueryStream := func() network.RetrievalQueryStream {
		qRead, qWrite := tut.QueryReadWriter()
		qrRead, qrWrite := tut.QueryResponseReadWriter()
		qs := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
			PeerID:     expectedPeer,
			Reader:     qRead,
			Writer:     qWrite,
			RespReader: qrRead,
			RespWriter: qrWrite,
		})
		return qs
	}

	receiveStreamOnProvider := func(
		t *testing.T,
		node *testnodes.TestRetrievalProviderNode,
		sa *testnodes.TestSectorAccessor,
		qs network.RetrievalQueryStream,
		pieceStore piecestore.PieceStore,
		dagStore *tut.MockDagStoreWrapper,
	) {
		ds := dss.MutexWrap(datastore.NewMapDatastore())
		dt := tut.NewTestDataTransfer()
		net := tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{})

		priceFunc := func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
			ask := retrievalmarket.Ask{}
			ask.PricePerByte = expectedPricePerByte
			ask.PaymentInterval = expectedPaymentInterval
			ask.PaymentIntervalIncrease = expectedPaymentIntervalIncrease

			if dealPricingParams.Unsealed {
				ask.UnsealPrice = expectedUnsealDiscount
			} else {
				ask.UnsealPrice = expectedUnsealPrice
			}
			return ask, nil
		}

		c, err := retrievalimpl.NewProvider(expectedAddress, node, sa, net, pieceStore, dagStore, dt, ds, priceFunc)
		require.NoError(t, err)

		tut.StartAndWaitForReady(ctx, t, c)

		net.ReceiveQueryStream(qs)
	}

	testCases := []struct {
		name               string
		query              retrievalmarket.Query
		expResp            retrievalmarket.QueryResponse
		expErr             string
		expFunc            func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper)
		sectorAccessorFunc func(sa *testnodes.TestSectorAccessor)

		expectedPricePerByte            abi.TokenAmount
		expectedPaymentInterval         uint64
		expectedPaymentIntervalIncrease uint64
		expectedUnsealPrice             abi.TokenAmount
	}{
		{name: "When PieceCID is not provided and PayloadCID is found",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID, expectedPiece)
				pieceStore.ExpectPiece(expectedPieceCID2, expectedPiece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseAvailable,
				PieceCIDFound: retrievalmarket.QueryItemAvailable,
				Size:          expectedSize,
			},
			expectedPricePerByte:            expectedPricePerByte,
			expectedPaymentInterval:         expectedPaymentInterval,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedUnsealPrice:             expectedUnsealPrice,
		},

		{name: "When PieceCID is not provided, prefer a piece for which an unsealed sector already exists and price it accordingly",
			sectorAccessorFunc: func(sa *testnodes.TestSectorAccessor) {
				p := expectedPiece2.Deals[0]
				sa.MarkUnsealed(context.TODO(), p.SectorID, p.Offset.Unpadded(), p.Length.Unpadded())
			},
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID, expectedPiece)
				pieceStore.ExpectPiece(expectedPieceCID2, expectedPiece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			query: retrievalmarket.Query{PayloadCID: payloadCID},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseAvailable,
				PieceCIDFound: retrievalmarket.QueryItemAvailable,
				Size:          expectedSize2,
			},
			expectedPricePerByte:            expectedPricePerByte,
			expectedPaymentInterval:         expectedPaymentInterval,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedUnsealPrice:             expectedUnsealDiscount,
		},

		{name: "When PieceCID is provided and both PieceCID and PayloadCID are found",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				loadPieceCIDS(t, pieceStore, payloadCID, expectedPieceCID)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)
			},
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID},
			},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseAvailable,
				PieceCIDFound: retrievalmarket.QueryItemAvailable,
				Size:          expectedSize,
			},
			expectedPricePerByte:            expectedPricePerByte,
			expectedPaymentInterval:         expectedPaymentInterval,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedUnsealPrice:             expectedUnsealPrice,
		},

		{name: "When QueryParams has PieceCID and is missing",
			expFunc: func(t *testing.T, ps *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				loadPieceCIDS(t, ps, payloadCID, cid.Undef)
				ps.ExpectMissingPiece(expectedPieceCID)
				ps.ExpectMissingPiece(expectedPieceCID2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID},
			},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseUnavailable,
				PieceCIDFound: retrievalmarket.QueryItemUnavailable,
				Message:       "piece info for cid not found (deal has not been added to a piece yet)",
			},
			expectedPricePerByte:            big.Zero(),
			expectedPaymentInterval:         0,
			expectedPaymentIntervalIncrease: 0,
			expectedUnsealPrice:             big.Zero(),
		},

		{name: "When payload CID not found",
			expFunc: func(t *testing.T, ps *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {},
			query: retrievalmarket.Query{
				PayloadCID:  payloadCID,
				QueryParams: retrievalmarket.QueryParams{PieceCID: &expectedPieceCID},
			},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseUnavailable,
				PieceCIDFound: retrievalmarket.QueryItemUnavailable,
				Message:       "piece info for cid not found (deal has not been added to a piece yet)",
			},
			expectedPricePerByte:            big.Zero(),
			expectedPaymentInterval:         0,
			expectedPaymentIntervalIncrease: 0,
			expectedUnsealPrice:             big.Zero(),
		},

		{name: "When PieceCID is not provided and PayloadCID is an identity CID with one link that is found",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID, expectedPiece)
				pieceStore.ExpectPiece(expectedPieceCID2, expectedPiece2)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
			},
			query: retrievalmarket.Query{PayloadCID: identityCidWith1},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseAvailable,
				PieceCIDFound: retrievalmarket.QueryItemAvailable,
				Size:          expectedSize,
			},
			expectedPricePerByte:            expectedPricePerByte,
			expectedPaymentInterval:         expectedPaymentInterval,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedUnsealPrice:             expectedUnsealPrice,
		},

		{name: "When PieceCID is not provided and PayloadCID is an identity CID with two links that are found",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID, expectedPiece) // both only appear in this piece, expect just this call
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
				dagStore.AddBlockToPieceIndex(payloadCID2, expectedPieceCID)
			},
			query: retrievalmarket.Query{PayloadCID: identityCidWithBoth},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseAvailable,
				PieceCIDFound: retrievalmarket.QueryItemAvailable,
				Size:          expectedSize,
			},
			expectedPricePerByte:            expectedPricePerByte,
			expectedPaymentInterval:         expectedPaymentInterval,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedUnsealPrice:             expectedUnsealPrice,
		},

		{name: "When PieceCID is not provided and PayloadCID is an identity CID with two links (one nested) that are found",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {
				pieceStore.ExpectPiece(expectedPieceCID, expectedPiece) // both only appear in this piece, expect just this call
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)
				dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID2)
				dagStore.AddBlockToPieceIndex(payloadCID2, expectedPieceCID)
			},
			query: retrievalmarket.Query{PayloadCID: identityCidWithBothNested},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseAvailable,
				PieceCIDFound: retrievalmarket.QueryItemAvailable,
				Size:          expectedSize,
			},
			expectedPricePerByte:            expectedPricePerByte,
			expectedPaymentInterval:         expectedPaymentInterval,
			expectedPaymentIntervalIncrease: expectedPaymentIntervalIncrease,
			expectedUnsealPrice:             expectedUnsealPrice,
		},

		{name: "When PieceCID is not provided and PayloadCID is an identity CID with a link that is not found",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {},
			query:   retrievalmarket.Query{PayloadCID: identityCidWithBogus},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseUnavailable,
				PieceCIDFound: retrievalmarket.QueryItemUnavailable,
				Message:       "piece info for cid not found (deal has not been added to a piece yet)",
			},
			expectedPricePerByte:            big.Zero(),
			expectedPaymentInterval:         0,
			expectedPaymentIntervalIncrease: 0,
			expectedUnsealPrice:             big.Zero(),
		},

		{name: "When PieceCID is not provided and PayloadCID is an identity CID that is too large",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {},
			query:   retrievalmarket.Query{PayloadCID: identityCidTooBig},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseError,
				PieceCIDFound: retrievalmarket.QueryItemUnavailable,
				Message:       "failed to fetch piece to retrieve from: refusing to decode too-long identity CID (2094 bytes)",
			},
			expectedPricePerByte:            big.Zero(),
			expectedPaymentInterval:         0,
			expectedPaymentIntervalIncrease: 0,
			expectedUnsealPrice:             big.Zero(),
		},

		{name: "When PieceCID is not provided and PayloadCID is an identity CID with too many links",
			expFunc: func(t *testing.T, pieceStore *tut.TestPieceStore, dagStore *tut.MockDagStoreWrapper) {},
			query:   retrievalmarket.Query{PayloadCID: identityCidTooManyLinks},
			expResp: retrievalmarket.QueryResponse{
				Status:        retrievalmarket.QueryResponseError,
				PieceCIDFound: retrievalmarket.QueryItemUnavailable,
				Message:       "failed to fetch piece to retrieve from: refusing to process identity CID with too many links (33)",
			},
			expectedPricePerByte:            big.Zero(),
			expectedPaymentInterval:         0,
			expectedPaymentIntervalIncrease: 0,
			expectedUnsealPrice:             big.Zero(),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node := testnodes.NewTestRetrievalProviderNode()
			sa := testnodes.NewTestSectorAccessor()
			qs := readWriteQueryStream()
			err := qs.WriteQuery(tc.query)
			require.NoError(t, err)
			pieceStore := tut.NewTestPieceStore()
			dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)
			if tc.sectorAccessorFunc != nil {
				tc.sectorAccessorFunc(sa)
			}

			tc.expFunc(t, pieceStore, dagStore)

			receiveStreamOnProvider(t, node, sa, qs, pieceStore, dagStore)

			actualResp, err := qs.ReadQueryResponse()
			pieceStore.VerifyExpectations(t)
			if tc.expErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expErr)
			}

			tc.expResp.PaymentAddress = expectedAddress
			tc.expResp.MinPricePerByte = tc.expectedPricePerByte
			tc.expResp.MaxPaymentInterval = tc.expectedPaymentInterval
			tc.expResp.MaxPaymentIntervalIncrease = tc.expectedPaymentIntervalIncrease
			tc.expResp.UnsealPrice = tc.expectedUnsealPrice
			assert.Equal(t, tc.expResp, actualResp)
		})
	}

	t.Run("error reading piece", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		sa := testnodes.NewTestSectorAccessor()

		qs := readWriteQueryStream()
		err := qs.WriteQuery(retrievalmarket.Query{
			PayloadCID: payloadCID,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()
		dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)
		dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)

		receiveStreamOnProvider(t, node, sa, qs, pieceStore, dagStore)

		response, err := qs.ReadQueryResponse()
		require.NoError(t, err)
		require.Equal(t, response.Status, retrievalmarket.QueryResponseError)
		require.NotEmpty(t, response.Message)
	})

	t.Run("when ReadDealStatusRequest fails", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		sa := testnodes.NewTestSectorAccessor()
		qs := readWriteQueryStream()
		pieceStore := tut.NewTestPieceStore()
		dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)

		receiveStreamOnProvider(t, node, sa, qs, pieceStore, dagStore)

		response, err := qs.ReadQueryResponse()
		require.NotNil(t, err)
		require.Equal(t, response, retrievalmarket.QueryResponseUndefined)
	})

	t.Run("when WriteDealStatusResponse fails", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		sa := testnodes.NewTestSectorAccessor()
		qRead, qWrite := tut.QueryReadWriter()
		qs := tut.NewTestRetrievalQueryStream(tut.TestQueryStreamParams{
			PeerID:     expectedPeer,
			Reader:     qRead,
			Writer:     qWrite,
			RespWriter: tut.FailResponseWriter,
		})
		err := qs.WriteQuery(retrievalmarket.Query{
			PayloadCID: payloadCID,
		})
		require.NoError(t, err)
		pieceStore := tut.NewTestPieceStore()
		pieceStore.ExpectPiece(expectedPieceCID, expectedPiece)
		dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)
		dagStore.AddBlockToPieceIndex(payloadCID, expectedPieceCID)

		receiveStreamOnProvider(t, node, sa, qs, pieceStore, dagStore)

		pieceStore.VerifyExpectations(t)
	})

}

func TestProvider_Construct(t *testing.T) {
	ds := datastore.NewMapDatastore()
	pieceStore := tut.NewTestPieceStore()
	node := testnodes.NewTestRetrievalProviderNode()
	sa := testnodes.NewTestSectorAccessor()
	dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)
	dt := tut.NewTestDataTransfer()

	priceFunc := func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		ask := retrievalmarket.Ask{}
		return ask, nil
	}

	_, err := retrievalimpl.NewProvider(
		tut.NewIDAddr(t, 2344),
		node,
		sa,
		tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{}),
		pieceStore,
		dagStore,
		dt,
		ds,
		priceFunc,
	)
	require.NoError(t, err)
	require.Len(t, dt.Subscribers, 1)
	require.Len(t, dt.RegisteredVoucherTypes, 2)
	require.Equal(t, dt.RegisteredVoucherTypes[0].VoucherType, retrievalmarket.DealProposalType)
	_, ok := dt.RegisteredVoucherTypes[0].Validator.(*requestvalidation.ProviderRequestValidator)
	require.True(t, ok)
	require.Equal(t, dt.RegisteredVoucherTypes[1].VoucherType, retrievalmarket.DealPaymentType)
	_, ok = dt.RegisteredVoucherTypes[1].Validator.(*requestvalidation.ProviderRequestValidator)
	require.True(t, ok)
	require.Len(t, dt.RegisteredTransportConfigurers, 1)
	require.Equal(t, dt.RegisteredTransportConfigurers[0].VoucherType, retrievalmarket.DealProposalType)

}

func TestProviderConfigOpts(t *testing.T) {
	var sawOpt int
	opt1 := func(p *retrievalimpl.Provider) { sawOpt++ }
	opt2 := func(p *retrievalimpl.Provider) { sawOpt += 2 }
	ds := datastore.NewMapDatastore()
	pieceStore := tut.NewTestPieceStore()
	node := testnodes.NewTestRetrievalProviderNode()
	sa := testnodes.NewTestSectorAccessor()
	dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)

	priceFunc := func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		ask := retrievalmarket.Ask{}
		return ask, nil
	}

	p, err := retrievalimpl.NewProvider(
		tut.NewIDAddr(t, 2344),
		node,
		sa,
		tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{}),
		pieceStore,
		dagStore,
		tut.NewTestDataTransfer(),
		ds, priceFunc, opt1, opt2,
	)
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, 3, sawOpt)

	// just test that we can create a DealDeciderOpt function and that it runs
	// successfully in the constructor
	ddOpt := retrievalimpl.DealDeciderOpt(
		func(_ context.Context, state retrievalmarket.ProviderDealState) (bool, string, error) {
			return true, "yes", nil
		})

	p, err = retrievalimpl.NewProvider(
		tut.NewIDAddr(t, 2344),
		testnodes.NewTestRetrievalProviderNode(),
		testnodes.NewTestSectorAccessor(),
		tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{}),
		tut.NewTestPieceStore(),
		dagStore,
		tut.NewTestDataTransfer(),
		ds, priceFunc, ddOpt)
	require.NoError(t, err)
	require.NotNil(t, p)
}

// loadPieceCIDS sets expectations to receive expectedPieceCID and 3 other random PieceCIDs to
// disinguish the case of a PayloadCID is found but the PieceCID is not
func loadPieceCIDS(t *testing.T, pieceStore *tut.TestPieceStore, expPayloadCID, expectedPieceCID cid.Cid) {

	otherPieceCIDs := tut.GenerateCids(3)
	expectedSize := uint64(1234)

	blockLocs := make([]piecestore.PieceBlockLocation, 4)
	expectedPieceInfo := piecestore.PieceInfo{
		PieceCID: expectedPieceCID,
		Deals: []piecestore.DealInfo{
			{
				Length: abi.PaddedPieceSize(expectedSize),
			},
		},
	}

	blockLocs[0] = piecestore.PieceBlockLocation{PieceCID: expectedPieceCID}
	for i, pieceCID := range otherPieceCIDs {
		blockLocs[i+1] = piecestore.PieceBlockLocation{PieceCID: pieceCID}
		pi := expectedPieceInfo
		pi.PieceCID = pieceCID
	}
	if expectedPieceCID != cid.Undef {
		pieceStore.ExpectPiece(expectedPieceCID, expectedPieceInfo)
	}
}

func TestProviderMigrations(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	pieceStore := tut.NewTestPieceStore()
	node := testnodes.NewTestRetrievalProviderNode()
	sa := testnodes.NewTestSectorAccessor()
	dagStore := tut.NewMockDagStoreWrapper(pieceStore, sa)
	dt := tut.NewTestDataTransfer()

	providerDs := namespace.Wrap(ds, datastore.NewKey("/retrievals/provider"))

	numDeals := 5
	payloadCIDs := make([]cid.Cid, numDeals)
	iDs := make([]retrievalmarket.DealID, numDeals)
	pieceCIDs := make([]*cid.Cid, numDeals)
	pricePerBytes := make([]abi.TokenAmount, numDeals)
	paymentIntervals := make([]uint64, numDeals)
	paymentIntervalIncreases := make([]uint64, numDeals)
	unsealPrices := make([]abi.TokenAmount, numDeals)
	storeIDs := make([]uint64, numDeals)
	channelIDs := make([]datatransfer.ChannelID, numDeals)
	receivers := make([]peer.ID, numDeals)
	messages := make([]string, numDeals)
	fundsReceiveds := make([]abi.TokenAmount, numDeals)
	selfPeer := tut.GeneratePeers(1)[0]
	dealIDs := make([]abi.DealID, numDeals)
	sectorIDs := make([]abi.SectorNumber, numDeals)
	offsets := make([]abi.PaddedPieceSize, numDeals)
	lengths := make([]abi.PaddedPieceSize, numDeals)

	for i := 0; i < numDeals; i++ {
		payloadCIDs[i] = tut.GenerateCids(1)[0]
		iDs[i] = retrievalmarket.DealID(rand.Uint64())
		pieceCID := tut.GenerateCids(1)[0]
		pieceCIDs[i] = &pieceCID
		pricePerBytes[i] = big.NewInt(rand.Int63())
		paymentIntervals[i] = rand.Uint64()
		paymentIntervalIncreases[i] = rand.Uint64()
		unsealPrices[i] = big.NewInt(rand.Int63())
		storeIDs[i] = rand.Uint64()
		receivers[i] = tut.GeneratePeers(1)[0]
		channelIDs[i] = datatransfer.ChannelID{
			Responder: selfPeer,
			Initiator: receivers[i],
			ID:        datatransfer.TransferID(rand.Uint64()),
		}
		messages[i] = string(tut.RandomBytes(20))
		fundsReceiveds[i] = big.NewInt(rand.Int63())
		dealIDs[i] = abi.DealID(rand.Uint64())
		sectorIDs[i] = abi.SectorNumber(rand.Uint64())
		offsets[i] = abi.PaddedPieceSize(rand.Uint64())
		lengths[i] = abi.PaddedPieceSize(rand.Uint64())
		deal := maptypes.ProviderDealState1{
			DealProposal: retrievalmarket.DealProposal{
				PayloadCID: payloadCIDs[i],
				ID:         iDs[i],
				Params: retrievalmarket.Params{
					Selector: retrievalmarket.CborGenCompatibleNode{
						Node: selectorparse.CommonSelector_ExploreAllRecursively,
					},
					PieceCID:                pieceCIDs[i],
					PricePerByte:            pricePerBytes[i],
					PaymentInterval:         paymentIntervals[i],
					PaymentIntervalIncrease: paymentIntervalIncreases[i],
					UnsealPrice:             unsealPrices[i],
				},
			},
			StoreID:   storeIDs[i],
			ChannelID: channelIDs[i],
			PieceInfo: &piecestore.PieceInfo{
				PieceCID: pieceCID,
				Deals: []piecestore.DealInfo{
					{
						DealID:   dealIDs[i],
						SectorID: sectorIDs[i],
						Offset:   offsets[i],
						Length:   lengths[i],
					},
				},
			},
			Status:        retrievalmarket.DealStatusCompleted,
			Receiver:      receivers[i],
			Message:       messages[i],
			FundsReceived: fundsReceiveds[i],
		}
		buf := new(bytes.Buffer)
		err := deal.MarshalCBOR(buf)
		require.NoError(t, err)
		err = providerDs.Put(ctx, datastore.NewKey(fmt.Sprint(deal.ID)), buf.Bytes())
		require.NoError(t, err)
	}

	priceFunc := func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		ask := retrievalmarket.Ask{}
		return ask, nil
	}

	retrievalProvider, err := retrievalimpl.NewProvider(
		tut.NewIDAddr(t, 2344),
		node,
		sa,
		tut.NewTestRetrievalMarketNetwork(tut.TestNetworkParams{}),
		pieceStore,
		dagStore,
		dt,
		providerDs,
		priceFunc,
	)
	require.NoError(t, err)
	tut.StartAndWaitForReady(ctx, t, retrievalProvider)
	deals := retrievalProvider.ListDeals()
	require.NoError(t, err)
	for i := 0; i < numDeals; i++ {
		deal, ok := deals[retrievalmarket.ProviderDealIdentifier{Receiver: receivers[i], DealID: iDs[i]}]
		require.True(t, ok)
		expectedDeal := retrievalmarket.ProviderDealState{
			DealProposal: retrievalmarket.DealProposal{
				PayloadCID: payloadCIDs[i],
				ID:         iDs[i],
				Params: retrievalmarket.Params{
					Selector: retrievalmarket.CborGenCompatibleNode{
						Node: selectorparse.CommonSelector_ExploreAllRecursively,
					},
					PieceCID:                pieceCIDs[i],
					PricePerByte:            pricePerBytes[i],
					PaymentInterval:         paymentIntervals[i],
					PaymentIntervalIncrease: paymentIntervalIncreases[i],
					UnsealPrice:             unsealPrices[i],
				},
			},
			StoreID:   storeIDs[i],
			ChannelID: &channelIDs[i],
			PieceInfo: &piecestore.PieceInfo{
				PieceCID: *pieceCIDs[i],
				Deals: []piecestore.DealInfo{
					{
						DealID:   dealIDs[i],
						SectorID: sectorIDs[i],
						Offset:   offsets[i],
						Length:   lengths[i],
					},
				},
			},
			Status:        retrievalmarket.DealStatusCompleted,
			Receiver:      receivers[i],
			Message:       messages[i],
			FundsReceived: fundsReceiveds[i],
		}
		require.Equal(t, expectedDeal, deal)
	}
}
