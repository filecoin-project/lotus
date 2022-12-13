package retrievalmarket_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipld/go-car"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
	testnodes2 "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmnet "github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/clientutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testharness/dependencies"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestStorageRetrieval(t *testing.T) {
	bgCtx := context.Background()

	tcs := map[string]struct {
		unSealPrice             abi.TokenAmount
		pricePerByte            abi.TokenAmount
		paymentInterval         uint64
		paymentIntervalIncrease uint64
		voucherAmts             []abi.TokenAmount
	}{

		"non-zero unseal, zero price per byte": {
			unSealPrice:  abi.NewTokenAmount(1000),
			pricePerByte: big.Zero(),
			voucherAmts:  []abi.TokenAmount{abi.NewTokenAmount(1000)},
		},

		"zero unseal, non-zero price per byte": {
			unSealPrice:             big.Zero(),
			pricePerByte:            abi.NewTokenAmount(1000),
			paymentInterval:         uint64(10000),
			paymentIntervalIncrease: uint64(1000),
			voucherAmts:             []abi.TokenAmount{abi.NewTokenAmount(10174000), abi.NewTokenAmount(19958000)},
		},

		"zero unseal, zero price per byte": {
			unSealPrice:             big.Zero(),
			pricePerByte:            big.Zero(),
			paymentInterval:         uint64(0),
			paymentIntervalIncrease: uint64(0),
			voucherAmts:             nil,
		},

		"non-zero unseal, non zero prices per byte": {
			unSealPrice:             abi.NewTokenAmount(1000),
			pricePerByte:            abi.NewTokenAmount(1000),
			paymentInterval:         uint64(10000),
			paymentIntervalIncrease: uint64(1000),
			voucherAmts:             []abi.TokenAmount{abi.NewTokenAmount(1000), abi.NewTokenAmount(10175000), abi.NewTokenAmount(19959000)},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			providerNode := testnodes2.NewTestRetrievalProviderNode()
			sa := testnodes2.NewTestSectorAccessor()
			pieceStore := tut.NewTestPieceStore()
			deps := setupDepsWithDagStore(bgCtx, t, sa, pieceStore)
			dagStore := deps.DagStore.(*tut.MockDagStoreWrapper)
			sh := testharness.NewHarnessWithTestData(t, deps.TestData, deps, true, false)

			storageProviderSeenDeal := doStorage(t, bgCtx, sh)
			ctxTimeout, canc := context.WithTimeout(bgCtx, 25*time.Second)
			defer canc()

			params := retrievalmarket.Params{
				UnsealPrice:             tc.unSealPrice,
				PricePerByte:            tc.pricePerByte,
				PaymentInterval:         tc.paymentInterval,
				PaymentIntervalIncrease: tc.paymentIntervalIncrease,
			}
			rh := newRetrievalHarnessWithDeps(ctxTimeout, t, sh, storageProviderSeenDeal, providerNode, sa, pieceStore, dagStore, params)

			checkRetrieve(t, bgCtx, rh, sh, tc.voucherAmts)
		})
	}
}

func TestOfflineStorageRetrieval(t *testing.T) {
	bgCtx := context.Background()

	tcs := map[string]struct {
		unSealPrice             abi.TokenAmount
		pricePerByte            abi.TokenAmount
		paymentInterval         uint64
		paymentIntervalIncrease uint64
		voucherAmts             []abi.TokenAmount
	}{

		"non-zero unseal, zero price per byte": {
			unSealPrice:  abi.NewTokenAmount(1000),
			pricePerByte: big.Zero(),
			voucherAmts:  []abi.TokenAmount{abi.NewTokenAmount(1000)},
		},

		"zero unseal, non-zero price per byte": {
			unSealPrice:             big.Zero(),
			pricePerByte:            abi.NewTokenAmount(1000),
			paymentInterval:         uint64(10000),
			paymentIntervalIncrease: uint64(1000),
			voucherAmts:             []abi.TokenAmount{abi.NewTokenAmount(10174000), abi.NewTokenAmount(19958000)},
		},

		"zero unseal, zero price per byte": {
			unSealPrice:             big.Zero(),
			pricePerByte:            big.Zero(),
			paymentInterval:         uint64(0),
			paymentIntervalIncrease: uint64(0),
			voucherAmts:             nil,
		},

		"non-zero unseal, non zero prices per byte": {
			unSealPrice:             abi.NewTokenAmount(1000),
			pricePerByte:            abi.NewTokenAmount(1000),
			paymentInterval:         uint64(10000),
			paymentIntervalIncrease: uint64(1000),
			voucherAmts:             []abi.TokenAmount{abi.NewTokenAmount(1000), abi.NewTokenAmount(10175000), abi.NewTokenAmount(19959000)},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			// offline storage
			providerNode := testnodes2.NewTestRetrievalProviderNode()
			sa := testnodes2.NewTestSectorAccessor()
			pieceStore := tut.NewTestPieceStore()
			deps := setupDepsWithDagStore(bgCtx, t, sa, pieceStore)
			dagStore := deps.DagStore.(*tut.MockDagStoreWrapper)
			sh := testharness.NewHarnessWithTestData(t, deps.TestData, deps, true, false)

			// start and wait for client/provider
			ctx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
			defer cancel()
			shared_testutil.StartAndWaitForReady(ctx, t, sh.Provider)
			shared_testutil.StartAndWaitForReady(ctx, t, sh.Client)

			// Do a Selective CARv1 traversal on the CARv2 file to get a deterministic CARv1 that we can import on the miner side.
			sc := car.NewSelectiveCar(ctx, sh.Data, []car.Dag{{Root: sh.PayloadCid, Selector: selectorparse.CommonSelector_ExploreAllRecursively}})
			prepared, err := sc.Prepare()
			require.NoError(t, err)
			carBuf := new(bytes.Buffer)
			require.NoError(t, prepared.Write(carBuf))

			commP, size, err := clientutils.CommP(ctx, sh.Data, &storagemarket.DataRef{
				// hacky but need it for now because if it's manual, we wont get a CommP.
				TransferType: storagemarket.TTGraphsync,
				Root:         sh.PayloadCid,
			}, 2<<29)
			require.NoError(t, err)

			// propose deal
			dataRef := &storagemarket.DataRef{
				TransferType: storagemarket.TTManual,
				Root:         sh.PayloadCid,
				PieceCid:     &commP,
				PieceSize:    size,
			}
			result := sh.ProposeStorageDeal(t, dataRef, false, false)
			proposalCid := result.ProposalCid

			wg := sync.WaitGroup{}
			sh.WaitForClientEvent(&wg, storagemarket.ClientEventDataTransferComplete)
			sh.WaitForProviderEvent(&wg, storagemarket.ProviderEventDataRequested)
			waitGroupWait(ctx, &wg)

			cd, err := sh.Client.GetLocalDeal(ctx, proposalCid)
			assert.NoError(t, err)
			require.Eventually(t, func() bool {
				cd, _ = sh.Client.GetLocalDeal(ctx, proposalCid)
				return cd.State == storagemarket.StorageDealCheckForAcceptance
			}, 1*time.Second, 100*time.Millisecond, "actual deal status is %s", storagemarket.DealStates[cd.State])

			providerDeals, err := sh.Provider.ListLocalDeals()
			assert.NoError(t, err)
			pd := providerDeals[0]
			assert.True(t, pd.ProposalCid.Equals(proposalCid))
			shared_testutil.AssertDealState(t, storagemarket.StorageDealWaitingForData, pd.State)

			// provider imports deal
			err = sh.Provider.ImportDataForDeal(ctx, pd.ProposalCid, carBuf)
			require.NoError(t, err)

			// wait for event signalling deal completion.
			sh.WaitForClientEvent(&wg, storagemarket.ClientEventDealExpired)
			sh.WaitForProviderEvent(&wg, storagemarket.ProviderEventDealExpired)
			waitGroupWait(ctx, &wg)

			// client asserts expiry
			require.Eventually(t, func() bool {
				cd, _ = sh.Client.GetLocalDeal(ctx, proposalCid)
				return cd.State == storagemarket.StorageDealExpired
			}, 5*time.Second, 100*time.Millisecond)

			// provider asserts expiry
			require.Eventually(t, func() bool {
				providerDeals, _ = sh.Provider.ListLocalDeals()
				pd = providerDeals[0]
				return pd.State == storagemarket.StorageDealExpired
			}, 5*time.Second, 100*time.Millisecond)

			t.Log("Offline storage complete")

			// Retrieve
			ctxTimeout, canc := context.WithTimeout(bgCtx, 25*time.Second)
			defer canc()
			params := retrievalmarket.Params{
				UnsealPrice:             tc.unSealPrice,
				PricePerByte:            tc.pricePerByte,
				PaymentInterval:         tc.paymentInterval,
				PaymentIntervalIncrease: tc.paymentIntervalIncrease,
			}
			rh := newRetrievalHarnessWithDeps(ctxTimeout, t, sh, cd, providerNode, sa, pieceStore, dagStore, params)

			checkRetrieve(t, bgCtx, rh, sh, tc.voucherAmts)
		})
	}
}

func checkRetrieve(t *testing.T, bgCtx context.Context, rh *retrievalHarness, sh *testharness.StorageHarness, vAmts []abi.TokenAmount) {
	clientDealStateChan := make(chan retrievalmarket.ClientDealState)

	rh.Client.SubscribeToEvents(func(event retrievalmarket.ClientEvent, state retrievalmarket.ClientDealState) {
		switch state.Status {
		case retrievalmarket.DealStatusCompleted:
			clientDealStateChan <- state
		default:
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
		}
	})

	providerDealStateChan := make(chan retrievalmarket.ProviderDealState)
	rh.Provider.SubscribeToEvents(func(event retrievalmarket.ProviderEvent, state retrievalmarket.ProviderDealState) {
		switch state.Status {
		case retrievalmarket.DealStatusCompleted:
			providerDealStateChan <- state
		default:
			msg := `
			Provider:
			Event:           %s
			Status:          %s
			TotalSent:       %d
			FundsReceived:   %s
			Message:		 %s
			CurrentInterval: %d
			`
			t.Logf(msg, retrievalmarket.ProviderEvents[event], retrievalmarket.DealStatuses[state.Status], state.TotalSent, state.FundsReceived.String(), state.Message,
				state.CurrentInterval)
		}
	})

	fsize := doRetrieve(t, bgCtx, rh, sh, vAmts)

	ctxTimeout, cancel := context.WithTimeout(bgCtx, 10*time.Second)
	defer cancel()

	// verify that client subscribers will be notified of state changes
	var clientDealState retrievalmarket.ClientDealState
	select {
	case <-ctxTimeout.Done():
		t.Error("deal never completed")
		t.FailNow()
	case clientDealState = <-clientDealStateChan:
	}

	ctxTimeout, cancel = context.WithTimeout(bgCtx, 10*time.Second)
	defer cancel()
	var providerDealState retrievalmarket.ProviderDealState
	select {
	case <-ctxTimeout.Done():
		t.Error("provider never saw completed deal")
		t.FailNow()
	case providerDealState = <-providerDealStateChan:
	}

	require.Equal(t, retrievalmarket.DealStatusCompleted, providerDealState.Status)
	require.Equal(t, retrievalmarket.DealStatusCompleted, clientDealState.Status)

	rh.ClientNode.VerifyExpectations(t)

	sh.TestData.VerifyFileTransferredIntoStore(t, cidlink.Link{Cid: sh.PayloadCid}, rh.BlockstoreAccessor.Blockstore, uint64(fsize))
}

// waitGroupWait calls wg.Wait while respecting context cancellation
func waitGroupWait(ctx context.Context, wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

var _ datatransfer.RequestValidator = (*fakeDTValidator)(nil)

type retrievalHarness struct {
	Ctx                         context.Context
	Epoch                       abi.ChainEpoch
	Client                      retrievalmarket.RetrievalClient
	ClientNode                  *testnodes2.TestRetrievalClientNode
	Provider                    retrievalmarket.RetrievalProvider
	ProviderNode                *testnodes2.TestRetrievalProviderNode
	BlockstoreAccessor          *tut.TestRetrievalBlockstoreAccessor
	SectorAccessor              *testnodes2.TestSectorAccessor
	PieceStore                  piecestore.PieceStore
	ExpPaych, NewLaneAddr       *address.Address
	ExpPaychAmt, ActualPaychAmt *abi.TokenAmount
	ExpVoucher, ActualVoucher   *paych.SignedVoucher
	RetrievalParams             retrievalmarket.Params

	TestDataNet *shared_testutil.Libp2pTestData
}

func setupDepsWithDagStore(ctx context.Context, t *testing.T, sa *testnodes2.TestSectorAccessor, pieceStore *tut.TestPieceStore) *dependencies.StorageDependencies {
	smState := testnodes.NewStorageMarketState()
	td := shared_testutil.NewLibp2pTestData(ctx, t)
	deps := dependencies.NewDependenciesWithTestData(t, ctx, td, smState, "", testnodes.DelayFakeCommonNode{}, testnodes.DelayFakeCommonNode{})
	deps.DagStore = tut.NewMockDagStoreWrapper(pieceStore, sa)
	return deps
}

func newRetrievalHarnessWithDeps(
	ctx context.Context,
	t *testing.T,
	sh *testharness.StorageHarness,
	deal storagemarket.ClientDeal,
	providerNode *testnodes2.TestRetrievalProviderNode,
	sa *testnodes2.TestSectorAccessor,
	pieceStore *tut.TestPieceStore,
	dagStore *tut.MockDagStoreWrapper,
	params ...retrievalmarket.Params,
) *retrievalHarness {
	var newPaychAmt abi.TokenAmount
	paymentChannelRecorder := func(client, miner address.Address, amt abi.TokenAmount) {
		newPaychAmt = amt
	}

	var newLaneAddr address.Address
	laneRecorder := func(paymentChannel address.Address) {
		newLaneAddr = paymentChannel
	}

	var newVoucher paych.SignedVoucher
	paymentVoucherRecorder := func(v *paych.SignedVoucher) {
		newVoucher = *v
	}

	cids := tut.GenerateCids(2)
	clientPaymentChannel, err := address.NewActorAddress([]byte("a"))

	expectedVoucher := tut.MakeTestSignedVoucher()
	require.NoError(t, err)
	clientNode := testnodes2.NewTestRetrievalClientNode(testnodes2.TestRetrievalClientNodeParams{
		Lane:                   expectedVoucher.Lane,
		PayCh:                  clientPaymentChannel,
		Voucher:                expectedVoucher,
		PaymentChannelRecorder: paymentChannelRecorder,
		AllocateLaneRecorder:   laneRecorder,
		PaymentVoucherRecorder: paymentVoucherRecorder,
		CreatePaychCID:         cids[0],
		AddFundsCID:            cids[1],
		IntegrationTest:        true,
	})

	nw1 := rmnet.NewFromLibp2pHost(sh.TestData.Host1, rmnet.RetryParameters(0, 0, 0, 0))
	clientDs := namespace.Wrap(sh.TestData.Ds1, datastore.NewKey("/retrievals/client"))
	ba := tut.NewTestRetrievalBlockstoreAccessor()
	client, err := retrievalimpl.NewClient(nw1, sh.DTClient, clientNode, sh.PeerResolver, clientDs, ba)
	require.NoError(t, err)
	tut.StartAndWaitForReady(ctx, t, client)
	payloadCID := deal.DataRef.Root
	providerPaymentAddr := deal.MinerWorker

	// Get the data passed to the sealing code when the last deal completed.
	// This is the padded CAR file.
	carData := sh.ProviderNode.LastOnDealCompleteBytes
	expectedPiece := deal.Proposal.PieceCID
	sectorID := abi.SectorNumber(100000)
	offset := abi.PaddedPieceSize(1000)
	pieceInfo := piecestore.PieceInfo{
		PieceCID: expectedPiece,
		Deals: []piecestore.DealInfo{
			{
				SectorID: sectorID,
				Offset:   offset,
				Length:   abi.UnpaddedPieceSize(uint64(len(carData))).Padded(),
			},
		},
	}
	sa.ExpectUnseal(sectorID, offset.Unpadded(), abi.UnpaddedPieceSize(uint64(len(carData))), carData)

	// clear out provider blockstore
	allCids, err := sh.TestData.Bs2.AllKeysChan(sh.Ctx)
	require.NoError(t, err)
	for c := range allCids {
		err = sh.TestData.Bs2.DeleteBlock(ctx, c)
		require.NoError(t, err)
	}

	nw2 := rmnet.NewFromLibp2pHost(sh.TestData.Host2, rmnet.RetryParameters(0, 0, 0, 0))
	cidInfo := piecestore.CIDInfo{
		PieceBlockLocations: []piecestore.PieceBlockLocation{
			{
				PieceCID: expectedPiece,
			},
		},
	}
	pieceStore.ExpectCID(payloadCID, cidInfo)
	pieceStore.ExpectPiece(expectedPiece, pieceInfo)
	dagStore.AddBlockToPieceIndex(payloadCID, expectedPiece)
	providerDs := namespace.Wrap(sh.TestData.Ds2, datastore.NewKey("/retrievals/provider"))

	var p retrievalmarket.Params
	if len(params) == 0 {
		p = retrievalmarket.Params{
			PricePerByte:            abi.NewTokenAmount(1000),
			PaymentInterval:         uint64(10000),
			PaymentIntervalIncrease: uint64(1000),
			UnsealPrice:             big.Zero(),
		}
	} else {
		p = params[0]
	}

	priceFunc := func(ctx context.Context, dealPricingParams retrievalmarket.PricingInput) (retrievalmarket.Ask, error) {
		ask := retrievalmarket.Ask{}
		ask.PaymentInterval = p.PaymentInterval
		ask.PaymentIntervalIncrease = p.PaymentIntervalIncrease
		ask.PricePerByte = p.PricePerByte
		ask.UnsealPrice = p.UnsealPrice

		return ask, nil
	}

	provider, err := retrievalimpl.NewProvider(
		providerPaymentAddr, providerNode, sa, nw2, pieceStore,
		sh.DagStore, sh.DTProvider, providerDs, priceFunc)
	require.NoError(t, err)
	tut.StartAndWaitForReady(ctx, t, provider)

	return &retrievalHarness{
		Ctx:                ctx,
		Client:             client,
		ClientNode:         clientNode,
		Epoch:              sh.Epoch,
		ExpPaych:           &clientPaymentChannel,
		NewLaneAddr:        &newLaneAddr,
		ActualPaychAmt:     &newPaychAmt,
		ExpVoucher:         expectedVoucher,
		ActualVoucher:      &newVoucher,
		Provider:           provider,
		ProviderNode:       providerNode,
		SectorAccessor:     sa,
		BlockstoreAccessor: ba,
		PieceStore:         sh.PieceStore,
		RetrievalParams:    p,
		TestDataNet:        sh.TestData,
	}
}

type fakeDTValidator struct{}

func (v *fakeDTValidator) ValidatePush(isRestart bool, _ datatransfer.ChannelID, sender peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.VoucherResult, error) {
	return nil, nil
}

func (v *fakeDTValidator) ValidatePull(isRestart bool, _ datatransfer.ChannelID, receiver peer.ID, voucher datatransfer.Voucher, baseCid cid.Cid, selector ipld.Node) (datatransfer.VoucherResult, error) {
	return nil, nil
}

func doStorage(t *testing.T, ctx context.Context, sh *testharness.StorageHarness) storagemarket.ClientDeal {
	shared_testutil.StartAndWaitForReady(ctx, t, sh.Client)
	shared_testutil.StartAndWaitForReady(ctx, t, sh.Provider)

	// set up a subscriber
	providerDealChan := make(chan storagemarket.MinerDeal)
	subscriber := func(event storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		providerDealChan <- deal
	}
	_ = sh.Provider.SubscribeToEvents(subscriber)

	clientDealChan := make(chan storagemarket.ClientDeal)
	clientSubscriber := func(event storagemarket.ClientEvent, deal storagemarket.ClientDeal) {
		clientDealChan <- deal
	}
	_ = sh.Client.SubscribeToEvents(clientSubscriber)

	// set ask price where we'll accept any price
	err := sh.Provider.SetAsk(big.NewInt(0), big.NewInt(0), 50000)
	assert.NoError(t, err)

	result := sh.ProposeStorageDeal(t, &storagemarket.DataRef{TransferType: storagemarket.TTGraphsync, Root: sh.PayloadCid}, false, false)
	require.False(t, result.ProposalCid.Equals(cid.Undef))

	time.Sleep(time.Millisecond * 200)

	ctxTimeout, canc := context.WithTimeout(ctx, 25*time.Second)
	defer canc()

	var storageProviderSeenDeal storagemarket.MinerDeal
	var storageClientSeenDeal storagemarket.ClientDeal
	for storageProviderSeenDeal.State != storagemarket.StorageDealExpired ||
		storageClientSeenDeal.State != storagemarket.StorageDealExpired {
		select {
		case storageProviderSeenDeal = <-providerDealChan:
		case storageClientSeenDeal = <-clientDealChan:
		case <-ctxTimeout.Done():
			t.Fatalf("never saw completed deal, client deal state: %s (%d), provider deal state: %s (%d)",
				storagemarket.DealStates[storageClientSeenDeal.State],
				storageClientSeenDeal.State,
				storagemarket.DealStates[storageProviderSeenDeal.State],
				storageProviderSeenDeal.State,
			)
		}
	}
	// ---------------
	fmt.Println("\n Storage is complete")

	return storageClientSeenDeal
}

func doRetrieve(t *testing.T, ctx context.Context, rh *retrievalHarness, sh *testharness.StorageHarness, voucherAmts []abi.TokenAmount) int {

	proof := []byte("")
	for _, voucherAmt := range voucherAmts {
		require.NoError(t, rh.ProviderNode.ExpectVoucher(*rh.ExpPaych, rh.ExpVoucher, proof, voucherAmt, voucherAmt, nil))
	}

	peers := rh.Client.FindProviders(sh.PayloadCid)
	require.Len(t, peers, 1)
	retrievalPeer := peers[0]
	require.NotNil(t, retrievalPeer.PieceCID)

	rh.ClientNode.ExpectKnownAddresses(retrievalPeer, nil)

	resp, err := rh.Client.Query(ctx, retrievalPeer, sh.PayloadCid, retrievalmarket.QueryParams{})
	require.NoError(t, err)
	require.Equal(t, retrievalmarket.QueryResponseAvailable, resp.Status)

	// testing V1 only
	rmParams, err := retrievalmarket.NewParamsV1(rh.RetrievalParams.PricePerByte, rh.RetrievalParams.PaymentInterval, rh.RetrievalParams.PaymentIntervalIncrease,
		selectorparse.CommonSelector_ExploreAllRecursively, nil,
		rh.RetrievalParams.UnsealPrice)
	require.NoError(t, err)

	// just make sure there is enough to cover the transfer
	fsize := 19000 // this is the known file size of the test file lorem.txt
	expectedTotal := big.Add(big.Mul(rh.RetrievalParams.PricePerByte, abi.NewTokenAmount(int64(fsize*2))), rh.RetrievalParams.UnsealPrice)

	// *** Retrieve the piece
	_, err = rh.Client.Retrieve(ctx, 0, sh.PayloadCid, rmParams, expectedTotal, retrievalPeer, *rh.ExpPaych, retrievalPeer.Address)
	require.NoError(t, err)

	return fsize
}
