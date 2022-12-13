package providerstates_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-statemachine/fsm"
	fsmtest "github.com/filecoin-project/go-statemachine/fsm/testutil"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	rmtesting "github.com/filecoin-project/go-fil-markets/retrievalmarket/testing"
	testnet "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestUnsealData(t *testing.T) {
	ctx := context.Background()
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	runUnsealData := func(t *testing.T,
		node *testnodes.TestRetrievalProviderNode,
		setupEnv func(e *rmtesting.TestProviderDealEnvironment),
		dealState *rm.ProviderDealState) {
		environment := rmtesting.NewTestProviderDealEnvironment(node)
		setupEnv(environment)
		fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
		err := providerstates.UnsealData(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		node.VerifyExpectations(t)
		fsmCtx.ReplayEvents(t, dealState)
	}

	expectedPiece := testnet.GenerateCids(1)[0]
	proposal := rm.DealProposal{
		ID:         rm.DealID(10),
		PayloadCID: expectedPiece,
		Params:     rm.NewParamsV0(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease),
	}

	pieceCid := testnet.GenerateCids(1)[0]

	sectorID := abi.SectorNumber(rand.Uint64())
	offset := abi.PaddedPieceSize(rand.Uint64())
	length := abi.PaddedPieceSize(rand.Uint64())

	sectorID2 := abi.SectorNumber(rand.Uint64())
	offset2 := abi.PaddedPieceSize(rand.Uint64())
	length2 := abi.PaddedPieceSize(rand.Uint64())

	makeDeals := func() *rm.ProviderDealState {
		return &rm.ProviderDealState{
			DealProposal: proposal,
			Status:       rm.DealStatusUnsealing,
			PieceInfo: &piecestore.PieceInfo{
				PieceCID: pieceCid,
				Deals: []piecestore.DealInfo{
					{
						DealID:   abi.DealID(rand.Uint64()),
						SectorID: sectorID,
						Offset:   offset,
						Length:   length,
					},
					{
						DealID:   abi.DealID(rand.Uint64()),
						SectorID: sectorID2,
						Offset:   offset2,
						Length:   length2,
					},
				},
			},
			TotalSent:     0,
			FundsReceived: abi.NewTokenAmount(0),
		}
	}

	t.Run("unseals successfully", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		dealState := makeDeals()
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {}
		runUnsealData(t, node, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusUnsealed)
	})

	t.Run("PrepareBlockstore error", func(t *testing.T) {
		node := testnodes.NewTestRetrievalProviderNode()
		dealState := makeDeals()
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.PrepareBlockstoreError = errors.New("Something went wrong")
		}
		runUnsealData(t, node, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusFailing)
		require.Equal(t, dealState.Message, "Something went wrong")
	})
}

func TestUnpauseDeal(t *testing.T) {
	ctx := context.Background()
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	runUnpauseDeal := func(t *testing.T,
		setupEnv func(e *rmtesting.TestProviderDealEnvironment),
		dealState *rm.ProviderDealState) {
		node := testnodes.NewTestRetrievalProviderNode()
		environment := rmtesting.NewTestProviderDealEnvironment(node)
		setupEnv(environment)
		fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
		dealState.ChannelID = &datatransfer.ChannelID{
			Initiator: "initiator",
			Responder: dealState.Receiver,
			ID:        1,
		}
		err := providerstates.UnpauseDeal(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		node.VerifyExpectations(t)
		fsmCtx.ReplayEvents(t, dealState)
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusUnsealed)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {}
		runUnpauseDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusUnsealed)
	})
	t.Run("error tracking channel", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusUnsealed)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.TrackTransferError = errors.New("something went wrong tracking")
		}
		runUnpauseDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong tracking")
	})
	t.Run("error resuming channel", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusUnsealed)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.ResumeDataTransferError = errors.New("something went wrong resuming")
		}
		runUnpauseDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong resuming")
	})
}

func TestCancelDeal(t *testing.T) {
	ctx := context.Background()
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	runCancelDeal := func(t *testing.T,
		setupEnv func(e *rmtesting.TestProviderDealEnvironment),
		dealState *rm.ProviderDealState) {
		node := testnodes.NewTestRetrievalProviderNode()
		environment := rmtesting.NewTestProviderDealEnvironment(node)
		setupEnv(environment)
		fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
		dealState.ChannelID = &datatransfer.ChannelID{
			Initiator: "initiator",
			Responder: dealState.Receiver,
			ID:        1,
		}
		err := providerstates.CancelDeal(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		node.VerifyExpectations(t)
		fsmCtx.ReplayEvents(t, dealState)
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusFailing)
		dealState.Message = "Existing error"
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {}
		runCancelDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "Existing error")
	})
	t.Run("error untracking channel", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusFailing)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.UntrackTransferError = errors.New("something went wrong untracking")
		}
		runCancelDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong untracking")
	})
	t.Run("error deleting store", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusFailing)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.DeleteStoreError = errors.New("something went wrong deleting store")
		}
		runCancelDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong deleting store")
	})
	t.Run("error closing channel", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusFailing)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.CloseDataTransferError = errors.New("something went wrong closing")
		}
		runCancelDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong closing")
	})
}

func TestCleanupDeal(t *testing.T) {
	ctx := context.Background()
	eventMachine, err := fsm.NewEventProcessor(rm.ProviderDealState{}, "Status", providerstates.ProviderEvents)
	require.NoError(t, err)
	runCleanupDeal := func(t *testing.T,
		setupEnv func(e *rmtesting.TestProviderDealEnvironment),
		dealState *rm.ProviderDealState) {
		node := testnodes.NewTestRetrievalProviderNode()
		environment := rmtesting.NewTestProviderDealEnvironment(node)
		setupEnv(environment)
		fsmCtx := fsmtest.NewTestContext(ctx, eventMachine)
		err := providerstates.CleanupDeal(fsmCtx, environment, *dealState)
		require.NoError(t, err)
		node.VerifyExpectations(t)
		fsmCtx.ReplayEvents(t, dealState)
	}

	t.Run("it works", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusCompleting)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {}
		runCleanupDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusCompleted)
	})
	t.Run("error untracking channel", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusCompleting)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.UntrackTransferError = errors.New("something went wrong untracking")
		}
		runCleanupDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong untracking")
	})
	t.Run("error deleting store", func(t *testing.T) {
		dealState := makeDealState(rm.DealStatusCompleting)
		setupEnv := func(fe *rmtesting.TestProviderDealEnvironment) {
			fe.DeleteStoreError = errors.New("something went wrong deleting store")
		}
		runCleanupDeal(t, setupEnv, dealState)
		require.Equal(t, dealState.Status, rm.DealStatusErrored)
		require.Equal(t, dealState.Message, "something went wrong deleting store")
	})

}

var dealID = rm.DealID(10)
var defaultCurrentInterval = uint64(1000)
var defaultIntervalIncrease = uint64(500)
var defaultPricePerByte = abi.NewTokenAmount(500)
var defaultPaymentPerInterval = big.Mul(defaultPricePerByte, abi.NewTokenAmount(int64(defaultCurrentInterval)))
var defaultTotalSent = uint64(5000)
var defaultFundsReceived = abi.NewTokenAmount(2500000)

func makeDealState(status rm.DealStatus) *rm.ProviderDealState {
	return &rm.ProviderDealState{
		Status:          status,
		TotalSent:       defaultTotalSent,
		CurrentInterval: defaultCurrentInterval,
		FundsReceived:   defaultFundsReceived,
		DealProposal: rm.DealProposal{
			ID:     dealID,
			Params: rm.NewParamsV0(defaultPricePerByte, defaultCurrentInterval, defaultIntervalIncrease),
		},
	}
}
