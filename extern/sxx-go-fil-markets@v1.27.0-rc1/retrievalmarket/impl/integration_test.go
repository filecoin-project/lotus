package retrievalimpl_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/network"
	"github.com/ipld/go-car"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	dtimpl "github.com/filecoin-project/go-data-transfer/v2/impl"
	dtgstransport "github.com/filecoin-project/go-data-transfer/v2/transport/graphsync"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	rmtesting "github.com/filecoin-project/go-fil-markets/retrievalmarket/testing"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/stores"
)

func TestClientCanMakeQueryToProvider(t *testing.T) {
	bgCtx := context.Background()
	payChAddr := address.TestAddress

	client, expectedCIDs, missingPiece, expectedQR, retrievalPeer, _, pieceStore := requireSetupTestClientAndProvider(bgCtx, t, payChAddr)

	t.Run("when piece is found, returns piece and price data", func(t *testing.T) {
		expectedQR.Status = retrievalmarket.QueryResponseAvailable
		actualQR, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], retrievalmarket.QueryParams{})

		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when piece is not found, returns unavailable", func(t *testing.T) {
		expectedQR.PieceCIDFound = retrievalmarket.QueryItemUnavailable
		expectedQR.Status = retrievalmarket.QueryResponseUnavailable
		expectedQR.Message = "piece info for cid not found (deal has not been added to a piece yet)"
		expectedQR.Size = 0
		actualQR, err := client.Query(bgCtx, retrievalPeer, missingPiece, retrievalmarket.QueryParams{})
		actualQR.MaxPaymentInterval = expectedQR.MaxPaymentInterval
		actualQR.MinPricePerByte = expectedQR.MinPricePerByte
		actualQR.MaxPaymentIntervalIncrease = expectedQR.MaxPaymentIntervalIncrease
		actualQR.UnsealPrice = expectedQR.UnsealPrice
		assert.NoError(t, err)
		assert.Equal(t, expectedQR, actualQR)
	})

	t.Run("when there is some other error, returns error", func(t *testing.T) {
		pieceStore.ReturnErrorFromGetPieceInfo(fmt.Errorf("someerr"))
		expectedQR.Status = retrievalmarket.QueryResponseError
		expectedQR.PieceCIDFound = retrievalmarket.QueryItemUnavailable
		expectedQR.Size = 0
		expectedQR.Message = "failed to fetch piece to retrieve from: someerr"
		actualQR, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], retrievalmarket.QueryParams{})
		assert.NoError(t, err)
		actualQR.MaxPaymentInterval = expectedQR.MaxPaymentInterval
		actualQR.MinPricePerByte = expectedQR.MinPricePerByte
		actualQR.MaxPaymentIntervalIncrease = expectedQR.MaxPaymentIntervalIncrease
		actualQR.UnsealPrice = expectedQR.UnsealPrice
		assert.Equal(t, expectedQR, actualQR)
	})

}

func TestProvider_Stop(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	bgCtx := context.Background()
	payChAddr := address.TestAddress
	client, expectedCIDs, _, _, retrievalPeer, provider, _ := requireSetupTestClientAndProvider(bgCtx, t, payChAddr)
	require.NoError(t, provider.Stop())
	_, err := client.Query(bgCtx, retrievalPeer, expectedCIDs[0], retrievalmarket.QueryParams{})

	assert.EqualError(t, err, "exhausted 5 attempts but failed to open stream, err: protocols not supported: [/fil/retrieval/qry/1.0.0]")
}

func requireSetupTestClientAndProvider(ctx context.Context, t *testing.T, payChAddr address.Address) (
	retrievalmarket.RetrievalClient,
	[]cid.Cid,
	cid.Cid,
	retrievalmarket.QueryResponse,
	retrievalmarket.RetrievalPeer,
	retrievalmarket.RetrievalProvider,
	*tut.TestPieceStore,
) {
	testData := tut.NewLibp2pTestData(ctx, t)
	nw1 := rmnet.NewFromLibp2pHost(testData.Host1, rmnet.RetryParameters(100*time.Millisecond, 1*time.Second, 5, 5))
	cids := tut.GenerateCids(2)
	rcNode1 := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{
		PayCh:          payChAddr,
		CreatePaychCID: cids[0],
		AddFundsCID:    cids[1],
	})

	gs1 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(testData.Host1), testData.LinkSystem1)
	dtTransport1 := dtgstransport.NewTransport(testData.Host1.ID(), gs1)
	dt1, err := dtimpl.NewDataTransfer(testData.DTStore1, testData.DTNet1, dtTransport1)
	require.NoError(t, err)
	tut.StartAndWaitForReadyDT(ctx, t, dt1)
	require.NoError(t, err)
	clientDs := namespace.Wrap(testData.Ds1, datastore.NewKey("/retrievals/client"))
	ba := tut.NewTestRetrievalBlockstoreAccessor()
	client, err := retrievalimpl.NewClient(nw1, dt1, rcNode1, &tut.TestPeerResolver{}, clientDs, ba)
	require.NoError(t, err)
	tut.StartAndWaitForReady(ctx, t, client)
	nw2 := rmnet.NewFromLibp2pHost(testData.Host2, rmnet.RetryParameters(0, 0, 0, 0))
	providerNode := testnodes.NewTestRetrievalProviderNode()
	sectorAccessor := testnodes.NewTestSectorAccessor()
	pieceStore := tut.NewTestPieceStore()
	expectedCIDs := tut.GenerateCids(3)
	expectedPieceCIDs := tut.GenerateCids(3)
	missingCID := tut.GenerateCids(1)[0]
	expectedQR := tut.MakeTestQueryResponse()
	dagstoreWrapper := tut.NewMockDagStoreWrapper(pieceStore, sectorAccessor)

	pieceStore.ExpectMissingCID(missingCID)
	for i, c := range expectedCIDs {
		pieceStore.ExpectCID(c, piecestore.CIDInfo{
			PieceBlockLocations: []piecestore.PieceBlockLocation{
				{
					PieceCID: expectedPieceCIDs[i],
				},
			},
		})
		dagstoreWrapper.AddBlockToPieceIndex(c, expectedPieceCIDs[i])
	}
	for i, piece := range expectedPieceCIDs {
		pieceStore.ExpectPiece(piece, piecestore.PieceInfo{
			Deals: []piecestore.DealInfo{
				{
					Length: abi.PaddedPieceSize(expectedQR.Size * uint64(i+1)),
				},
			},
		})
	}

	paymentAddress := address.TestAddress2

	gs2 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(testData.Host2), testData.LinkSystem2)
	dtTransport2 := dtgstransport.NewTransport(testData.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(testData.DTStore2, testData.DTNet2, dtTransport2)
	require.NoError(t, err)
	tut.StartAndWaitForReadyDT(ctx, t, dt2)
	require.NoError(t, err)
	providerDs := namespace.Wrap(testData.Ds2, datastore.NewKey("/retrievals/provider"))

	priceFunc := func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		ask := retrievalmarket.Ask{}
		ask.PaymentInterval = expectedQR.MaxPaymentInterval
		ask.PaymentIntervalIncrease = expectedQR.MaxPaymentIntervalIncrease
		ask.PricePerByte = expectedQR.MinPricePerByte
		ask.UnsealPrice = expectedQR.UnsealPrice
		return ask, nil
	}

	provider, err := retrievalimpl.NewProvider(
		paymentAddress, providerNode, sectorAccessor, nw2, pieceStore, dagstoreWrapper, dt2, providerDs,
		priceFunc)
	require.NoError(t, err)

	tut.StartAndWaitForReady(ctx, t, provider)
	retrievalPeer := retrievalmarket.RetrievalPeer{
		Address: paymentAddress,
		ID:      testData.Host2.ID(),
	}
	rcNode1.ExpectKnownAddresses(retrievalPeer, nil)

	expectedQR.Size = uint64(abi.PaddedPieceSize(expectedQR.Size).Unpadded())

	return client, expectedCIDs, missingCID, expectedQR, retrievalPeer, provider, pieceStore
}

func TestClientCanMakeDealWithProvider(t *testing.T) {
	// -------- SET UP PROVIDER

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)

	partialSelector := ssb.ExploreFields(func(specBuilder builder.ExploreFieldsSpecBuilder) {
		specBuilder.Insert("Links", ssb.ExploreIndex(0, ssb.ExploreFields(func(specBuilder builder.ExploreFieldsSpecBuilder) {
			specBuilder.Insert("Hash", ssb.Matcher())
		})))
	}).Node()

	var customDeciderRan bool

	testCases := []struct {
		name                    string
		decider                 retrievalimpl.DealDecider
		filename                string
		filesize                uint64
		voucherAmts             []abi.TokenAmount
		selector                datamodel.Node
		unsealPrice             abi.TokenAmount
		zeroPricePerByte        bool
		paramsV1, addFunds      bool
		skipStores              bool
		failsUnseal             bool
		paymentInterval         uint64
		paymentIntervalIncrease uint64
		channelAvailableFunds   retrievalmarket.ChannelAvailableFunds
		fundsReplenish          abi.TokenAmount
		cancelled               bool
	}{
		{name: "1 block file retrieval succeeds",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			addFunds:    false,
		},
		{name: "1 block file retrieval succeeds with unseal price",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			unsealPrice: abi.NewTokenAmount(100),
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(100), abi.NewTokenAmount(100), abi.NewTokenAmount(410100)},
			selector:    selectorparse.CommonSelector_ExploreAllRecursively,
			paramsV1:    true,
		},
		{name: "1 block file retrieval succeeds with existing payment channel",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			addFunds:    true},
		{name: "1 block file retrieval succeeds, but waits for other payment channel funds to land",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			channelAvailableFunds: retrievalmarket.ChannelAvailableFunds{
				// this is bit contrived, but we're simulating other deals expending the funds by setting the initial confirmed to negative
				// when funds get added on initial create, it will reset to zero
				// which will trigger a later voucher shortfall and then waiting for both
				// the pending and then the queued amounts
				ConfirmedAmt:        abi.NewTokenAmount(-410000),
				PendingAmt:          abi.NewTokenAmount(200000),
				PendingWaitSentinel: &tut.GenerateCids(1)[0],
				QueuedAmt:           abi.NewTokenAmount(210000),
			},
		},
		{name: "1 block file retrieval succeeds, after insufficient funds and restart",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
			channelAvailableFunds: retrievalmarket.ChannelAvailableFunds{
				// this is bit contrived, but we're simulating other deals expending the funds by setting the initial confirmed to negative
				// when funds get added on initial create, it will reset to zero
				// which will trigger a later voucher shortfall
				ConfirmedAmt: abi.NewTokenAmount(-410000),
			},
			fundsReplenish: abi.NewTokenAmount(410000),
		},
		{name: "1 block file retrieval cancelled after insufficient funds",
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{},
			channelAvailableFunds: retrievalmarket.ChannelAvailableFunds{
				// this is bit contrived, but we're simulating other deals expending the funds by setting the initial confirmed to negative
				// when funds get added on initial create, it will reset to zero
				// which will trigger a later voucher shortfall
				ConfirmedAmt: abi.NewTokenAmount(-410000),
			},
			cancelled: true,
		},
		{name: "multi-block file retrieval succeeds",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10174000), abi.NewTokenAmount(19958000)},
		},
		{name: "multi-block file retrieval with zero price per byte succeeds",
			filename:         "lorem.txt",
			filesize:         19000,
			zeroPricePerByte: true,
		},
		{name: "multi-block file retrieval succeeds with V1 params and AllSelector",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10174000), abi.NewTokenAmount(19958000)},
			paramsV1:    true,
			selector:    selectorparse.CommonSelector_ExploreAllRecursively},
		{name: "partial file retrieval succeeds with V1 params and selector recursion depth 1",
			filename:    "lorem.txt",
			filesize:    1024,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(1982000)},
			paramsV1:    true,
			selector:    partialSelector},
		{name: "succeeds when using a custom decider function",
			decider: func(ctx context.Context, state retrievalmarket.ProviderDealState) (bool, string, error) {
				customDeciderRan = true
				return true, "", nil
			},
			filename:    "lorem_under_1_block.txt",
			filesize:    410,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(410000)},
		},
		{name: "succeeds for regular blockstore",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(10174000), abi.NewTokenAmount(19958000)},
			skipStores:  true,
		},
		{
			name:        "failed unseal",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{},
			failsUnseal: true,
		},

		{name: "multi-block file retrieval succeeds, final block exceeds payment interval",
			filename:                "lorem.txt",
			filesize:                19000,
			voucherAmts:             []abi.TokenAmount{abi.NewTokenAmount(9150000), abi.NewTokenAmount(19390000), abi.NewTokenAmount(19958000)},
			paymentInterval:         9000,
			paymentIntervalIncrease: 1250,
		},

		{name: "multi-block file retrieval succeeds, final block lands on payment interval",
			filename:    "lorem.txt",
			filesize:    19000,
			voucherAmts: []abi.TokenAmount{abi.NewTokenAmount(9150000), abi.NewTokenAmount(19958000)},
			// Total bytes: 19,920
			// intervals: 9,000 | 9,000 + (9,000 + 1920)
			paymentInterval:         9000,
			paymentIntervalIncrease: 1920,
		},
	}

	for i, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bgCtx := context.Background()
			clientPaymentChannel, err := address.NewIDAddress(uint64(i * 10))
			require.NoError(t, err)

			testData := tut.NewLibp2pTestData(bgCtx, t)

			// Create a CARv2 file from a fixture
			fpath := filepath.Join(tut.ThisDir(t), "./fixtures/"+testCase.filename)
			pieceLink, path := testData.LoadUnixFSFileToStore(t, fpath)
			c, ok := pieceLink.(cidlink.Link)
			require.True(t, ok)
			payloadCID := c.Cid

			// Get the CARv1 payload of the UnixFS DAG that the (Filestore backed by the CARv2) contains.
			carFile, err := os.CreateTemp(t.TempDir(), "rand")
			require.NoError(t, err)

			fs, err := stores.ReadOnlyFilestore(path)
			require.NoError(t, err)

			sc := car.NewSelectiveCar(bgCtx, fs, []car.Dag{{Root: payloadCID, Selector: selectorparse.CommonSelector_ExploreAllRecursively}})
			prepared, err := sc.Prepare()
			require.NoError(t, err)
			carBuf := new(bytes.Buffer)
			require.NoError(t, prepared.Dump(context.TODO(), carBuf))
			carDataBuf := new(bytes.Buffer)
			tr := io.TeeReader(carBuf, carDataBuf)
			require.NoError(t, fs.Close())
			_, err = io.Copy(carFile, tr)
			require.NoError(t, err)
			require.NoError(t, carFile.Close())
			carData := carDataBuf.Bytes()

			// Set up retrieval parameters
			providerPaymentAddr, err := address.NewIDAddress(uint64(i * 99))
			require.NoError(t, err)
			paymentInterval := testCase.paymentInterval
			if paymentInterval == 0 {
				paymentInterval = uint64(10000)
			}
			paymentIntervalIncrease := testCase.paymentIntervalIncrease
			if paymentIntervalIncrease == 0 {
				paymentIntervalIncrease = uint64(1000)
			}
			pricePerByte := abi.NewTokenAmount(1000)
			if testCase.zeroPricePerByte {
				pricePerByte = abi.NewTokenAmount(0)
			}
			unsealPrice := testCase.unsealPrice
			if unsealPrice.Int == nil {
				unsealPrice = big.Zero()
			}

			expectedQR := retrievalmarket.QueryResponse{
				Size:                       1024,
				PaymentAddress:             providerPaymentAddr,
				MinPricePerByte:            pricePerByte,
				MaxPaymentInterval:         paymentInterval,
				MaxPaymentIntervalIncrease: paymentIntervalIncrease,
				UnsealPrice:                unsealPrice,
			}

			// Set up the piece info that will be retrieved by the provider
			// when the retrieval request is made
			sectorID := abi.SectorNumber(100000)
			offset := abi.PaddedPieceSize(1000)
			pieceInfo := piecestore.PieceInfo{
				PieceCID: tut.GenerateCids(1)[0],
				Deals: []piecestore.DealInfo{
					{
						DealID:   abi.DealID(100),
						SectorID: sectorID,
						Offset:   offset,
						Length:   abi.UnpaddedPieceSize(len(carData)).Padded(),
					},
				},
			}
			providerNode := testnodes.NewTestRetrievalProviderNode()
			providerNode.ExpectPricingParams(pieceInfo.PieceCID, []abi.DealID{100})

			sectorAccessor := testnodes.NewTestSectorAccessor()
			if testCase.failsUnseal {
				sectorAccessor.ExpectFailedUnseal(sectorID, offset.Unpadded(), abi.UnpaddedPieceSize(len(carData)))
			} else {
				sectorAccessor.ExpectUnseal(sectorID, offset.Unpadded(), abi.UnpaddedPieceSize(len(carData)), carData)
			}

			decider := rmtesting.TrivialTestDecider
			if testCase.decider != nil {
				decider = testCase.decider
			}

			// ------- SET UP CLIENT
			ctx, cancel := context.WithTimeout(bgCtx, 60*time.Second)
			defer cancel()

			provider := setupProvider(bgCtx, t, testData, payloadCID, pieceInfo, carFile.Name(), expectedQR,
				providerPaymentAddr, providerNode, sectorAccessor, decider)
			tut.StartAndWaitForReady(ctx, t, provider)

			retrievalPeer := retrievalmarket.RetrievalPeer{Address: providerPaymentAddr, ID: testData.Host2.ID()}

			expectedVoucher := tut.MakeTestSignedVoucher()

			// just make sure there is enough to cover the transfer
			expectedTotal := big.Mul(pricePerByte, abi.NewTokenAmount(int64(len(carData))))

			// voucherAmts are pulled from the actual answer so the expected keys in the test node match up.
			// later we compare the voucher values.  The last voucherAmt is a remainder
			proof := []byte("")
			for _, voucherAmt := range testCase.voucherAmts {
				require.NoError(t, providerNode.ExpectVoucher(clientPaymentChannel, expectedVoucher, proof, voucherAmt, voucherAmt, nil))
			}

			nw1 := rmnet.NewFromLibp2pHost(testData.Host1, rmnet.RetryParameters(0, 0, 0, 0))
			createdChan, newLaneAddr, createdVoucher, clientNode, client, ba, err := setupClient(bgCtx, t, clientPaymentChannel, expectedVoucher, nw1, testData, testCase.addFunds, testCase.channelAvailableFunds)
			require.NoError(t, err)
			tut.StartAndWaitForReady(ctx, t, client)

			clientNode.ExpectKnownAddresses(retrievalPeer, nil)

			clientDealStateChan := make(chan retrievalmarket.ClientDealState)
			client.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
				switch state.Status {
				case retrievalmarket.DealStatusCompleted, retrievalmarket.DealStatusCancelled, retrievalmarket.DealStatusErrored:
					clientDealStateChan <- state
					return
				}
				if state.Status == retrievalmarket.DealStatusInsufficientFunds {
					if !testCase.fundsReplenish.Nil() {
						clientNode.ResetChannelAvailableFunds(retrievalmarket.ChannelAvailableFunds{
							ConfirmedAmt: testCase.fundsReplenish,
						})
						client.TryRestartInsufficientFunds(state.PaymentInfo.PayCh)
					}
					if testCase.cancelled {
						client.CancelDeal(state.ID)
					}
				}
				msg := `
Client:
Event:           %s
Status:          %s
TotalReceived:   %d
BytesPaidFor:    %d
CurrentInterval: %d
TotalFunds:      %s
Message:         %s
`
				t.Logf(msg, retrievalmarket.ClientEvents[event], retrievalmarket.DealStatuses[state.Status], state.TotalReceived, state.BytesPaidFor, state.CurrentInterval,
					state.TotalFunds.String(), state.Message)
			})

			providerDealStateChan := make(chan retrievalmarket.ProviderDealState)
			provider.SubscribeToEvents(func(event retrievalmarket.ProviderEvent, state retrievalmarket.ProviderDealState) {
				switch state.Status {
				case retrievalmarket.DealStatusCompleted, retrievalmarket.DealStatusCancelled, retrievalmarket.DealStatusErrored:
					providerDealStateChan <- state
					return
				}
				msg := `
Provider:
Event:           %s
Status:          %s
FundsReceived:   %s
Message:		 %s
`
				t.Logf(msg, retrievalmarket.ProviderEvents[event], retrievalmarket.DealStatuses[state.Status], state.FundsReceived.String(), state.Message)

			})
			// **** Send the query for the Piece
			// set up retrieval params
			resp, err := client.Query(bgCtx, retrievalPeer, payloadCID, retrievalmarket.QueryParams{})
			require.NoError(t, err)
			require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

			var rmParams retrievalmarket.Params
			if testCase.paramsV1 {
				rmParams, err = retrievalmarket.NewParamsV1(pricePerByte, paymentInterval, paymentIntervalIncrease, testCase.selector, nil, unsealPrice)
				require.NoError(t, err)
			} else {
				rmParams = retrievalmarket.NewParamsV0(pricePerByte, paymentInterval, paymentIntervalIncrease)
			}

			// *** Retrieve the piece
			_, err = client.Retrieve(bgCtx, 0, payloadCID, rmParams, expectedTotal, retrievalPeer, clientPaymentChannel, retrievalPeer.Address)
			require.NoError(t, err)

			// verify that client subscribers will be notified of state changes
			var clientDealState retrievalmarket.ClientDealState
			select {
			case <-ctx.Done():
				t.Error("deal never completed")
				t.FailNow()
			case clientDealState = <-clientDealStateChan:
			}
			if testCase.failsUnseal || testCase.cancelled {
				assert.Equal(t, retrievalmarket.DealStatusCancelled, clientDealState.Status)
			} else {
				if !testCase.zeroPricePerByte {
					assert.Equal(t, clientDealState.PaymentInfo.Lane, expectedVoucher.Lane)
					require.NotNil(t, createdChan)
					require.Equal(t, expectedTotal, createdChan.amt)
					require.Equal(t, clientPaymentChannel, *newLaneAddr)

					// verify that the voucher was saved/seen by the client with correct values
					require.NotNil(t, createdVoucher)
					tut.TestVoucherEquality(t, createdVoucher, expectedVoucher)
				}
				assert.Equal(t, retrievalmarket.DealStatusCompleted, clientDealState.Status)
			}

			ctxProv, cancelProv := context.WithTimeout(bgCtx, 10*time.Second)
			defer cancelProv()
			var providerDealState retrievalmarket.ProviderDealState
			select {
			case <-ctxProv.Done():
				t.Error("provider never saw completed deal")
				t.FailNow()
			case providerDealState = <-providerDealStateChan:
			}

			if testCase.failsUnseal {
				tut.AssertRetrievalDealState(t, retrievalmarket.DealStatusErrored, providerDealState.Status)
			} else if testCase.cancelled {
				tut.AssertRetrievalDealState(t, retrievalmarket.DealStatusCancelled, providerDealState.Status)
			} else {
				tut.AssertRetrievalDealState(t, retrievalmarket.DealStatusCompleted, providerDealState.Status)
			}
			// TODO this is terrible, but it's temporary until the test harness refactor
			// in the resuming retrieval deals branch is done
			// https://github.com/filecoin-project/go-fil-markets/issues/65
			if testCase.decider != nil {
				assert.True(t, customDeciderRan)
			}
			// verify that the nodes we interacted with behaved as expected
			clientNode.VerifyExpectations(t)
			providerNode.VerifyExpectations(t)
			sectorAccessor.VerifyExpectations(t)
			if !testCase.failsUnseal && !testCase.cancelled {
				testData.VerifyFileTransferredIntoStore(t, pieceLink, ba.Blockstore, testCase.filesize)
			}
		})
	}
}

func setupClient(
	ctx context.Context,
	t *testing.T,
	clientPaymentChannel address.Address,
	expectedVoucher *paych.SignedVoucher,
	nw1 rmnet.RetrievalMarketNetwork,
	testData *tut.Libp2pTestData,
	addFunds bool,
	channelAvailableFunds retrievalmarket.ChannelAvailableFunds,
) (
	*pmtChan,
	*address.Address,
	*paych.SignedVoucher,
	*testnodes.TestRetrievalClientNode,
	retrievalmarket.RetrievalClient,
	*tut.TestRetrievalBlockstoreAccessor,
	error) {
	var createdChan pmtChan
	paymentChannelRecorder := func(client, miner address.Address, amt abi.TokenAmount) {
		createdChan = pmtChan{client, miner, amt}
	}

	var newLaneAddr address.Address
	laneRecorder := func(paymentChannel address.Address) {
		newLaneAddr = paymentChannel
	}

	var createdVoucher paych.SignedVoucher
	paymentVoucherRecorder := func(v *paych.SignedVoucher) {
		createdVoucher = *v
	}
	cids := tut.GenerateCids(2)
	clientNode := testnodes.NewTestRetrievalClientNode(testnodes.TestRetrievalClientNodeParams{
		AddFundsOnly:           addFunds,
		PayCh:                  clientPaymentChannel,
		Lane:                   expectedVoucher.Lane,
		Voucher:                expectedVoucher,
		PaymentChannelRecorder: paymentChannelRecorder,
		AllocateLaneRecorder:   laneRecorder,
		PaymentVoucherRecorder: paymentVoucherRecorder,
		CreatePaychCID:         cids[0],
		AddFundsCID:            cids[1],
		IntegrationTest:        true,
		ChannelAvailableFunds:  channelAvailableFunds,
	})

	gs1 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(testData.Host1), testData.LinkSystem1)
	dtTransport1 := dtgstransport.NewTransport(testData.Host1.ID(), gs1)
	dt1, err := dtimpl.NewDataTransfer(testData.DTStore1, testData.DTNet1, dtTransport1)
	require.NoError(t, err)
	tut.StartAndWaitForReadyDT(ctx, t, dt1)
	require.NoError(t, err)
	clientDs := namespace.Wrap(testData.Ds1, datastore.NewKey("/retrievals/client"))
	ba := tut.NewTestRetrievalBlockstoreAccessor()

	client, err := retrievalimpl.NewClient(nw1, dt1, clientNode, &tut.TestPeerResolver{}, clientDs, ba)
	return &createdChan, &newLaneAddr, &createdVoucher, clientNode, client, ba, err
}

func setupProvider(
	ctx context.Context,
	t *testing.T,
	testData *tut.Libp2pTestData,
	payloadCID cid.Cid,
	pieceInfo piecestore.PieceInfo,
	carFilePath string,
	expectedQR retrievalmarket.QueryResponse,
	providerPaymentAddr address.Address,
	providerNode retrievalmarket.RetrievalProviderNode,
	sectorAccessor retrievalmarket.SectorAccessor,
	decider retrievalimpl.DealDecider,
) retrievalmarket.RetrievalProvider {
	nw2 := rmnet.NewFromLibp2pHost(testData.Host2, rmnet.RetryParameters(0, 0, 0, 0))
	pieceStore := tut.NewTestPieceStore()
	expectedPiece := pieceInfo.PieceCID
	cidInfo := piecestore.CIDInfo{
		PieceBlockLocations: []piecestore.PieceBlockLocation{
			{
				PieceCID: expectedPiece,
			},
		},
	}
	pieceStore.ExpectCID(payloadCID, cidInfo)
	pieceStore.ExpectPiece(expectedPiece, pieceInfo)

	gs2 := graphsyncimpl.New(ctx, network.NewFromLibp2pHost(testData.Host2), testData.LinkSystem2)
	dtTransport2 := dtgstransport.NewTransport(testData.Host2.ID(), gs2)
	dt2, err := dtimpl.NewDataTransfer(testData.DTStore2, testData.DTNet2, dtTransport2)
	require.NoError(t, err)
	tut.StartAndWaitForReadyDT(ctx, t, dt2)
	require.NoError(t, err)
	providerDs := namespace.Wrap(testData.Ds2, datastore.NewKey("/retrievals/provider"))

	opts := []retrievalimpl.RetrievalProviderOption{retrievalimpl.DealDeciderOpt(decider)}

	priceFunc := func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		ask := retrievalmarket.Ask{}
		ask.PaymentInterval = expectedQR.MaxPaymentInterval
		ask.PaymentIntervalIncrease = expectedQR.MaxPaymentIntervalIncrease
		ask.PricePerByte = expectedQR.MinPricePerByte
		ask.UnsealPrice = expectedQR.UnsealPrice
		return ask, nil
	}

	// Create a DAG store wrapper
	dagstoreWrapper := tut.NewMockDagStoreWrapper(pieceStore, sectorAccessor)
	dagstoreWrapper.AddBlockToPieceIndex(payloadCID, pieceInfo.PieceCID)

	// Register the piece with the DAG store wrapper
	err = stores.RegisterShardSync(ctx, dagstoreWrapper, pieceInfo.PieceCID, carFilePath, true)
	require.NoError(t, err)

	// Remove the CAR file so that the provider is forced to unseal the data
	// (instead of using the cached CAR file)
	_ = os.Remove(carFilePath)

	provider, err := retrievalimpl.NewProvider(providerPaymentAddr, providerNode, sectorAccessor,
		nw2, pieceStore, dagstoreWrapper, dt2, providerDs, priceFunc, opts...)
	require.NoError(t, err)

	return provider
}

type pmtChan struct {
	client, miner address.Address
	amt           abi.TokenAmount
}
