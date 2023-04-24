package dtutils_test

import (
	"errors"
	"math/rand"
	"testing"

	ds "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestProviderDataTransferSubscriber(t *testing.T) {
	dealProposal := shared_testutil.MakeTestDealProposal()
	node := rm.BindnodeRegistry.TypeToNode(dealProposal)
	dealProposalVoucher := datatransfer.TypedVoucher{Voucher: node, Type: rm.DealProposalType}
	testPeers := shared_testutil.GeneratePeers(2)
	transferID := datatransfer.TransferID(rand.Uint64())
	tests := map[string]struct {
		code          datatransfer.EventCode
		message       string
		status        datatransfer.Status
		ignored       bool
		expectedEvent fsm.EventName
		expectedArgs  []interface{}
	}{
		"not a retrieval voucher": {
			ignored: true,
		},
		"accept": {
			code:          datatransfer.Accept,
			status:        datatransfer.Ongoing,
			expectedEvent: rm.ProviderEventDealAccepted,
			expectedArgs:  []interface{}{datatransfer.ChannelID{ID: transferID, Initiator: testPeers[1], Responder: testPeers[0]}},
		},
		"error": {
			code:          datatransfer.Error,
			message:       "something went wrong",
			status:        datatransfer.Ongoing,
			expectedEvent: rm.ProviderEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("deal data transfer failed: something went wrong")},
		},
		"disconnected": {
			code:          datatransfer.Disconnected,
			message:       "something went wrong",
			status:        datatransfer.Ongoing,
			expectedEvent: rm.ProviderEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("deal data transfer stalled (peer hungup)")},
		},
		"completed": {
			code:          datatransfer.ResumeResponder,
			status:        datatransfer.Completed,
			expectedEvent: rm.ProviderEventComplete,
		},
		"cancel": {
			code:          datatransfer.Cancel,
			status:        datatransfer.Cancelling,
			expectedEvent: rm.ProviderEventClientCancelled,
		},
		"data limit exceeded": {
			code:          datatransfer.DataLimitExceeded,
			status:        datatransfer.Ongoing,
			expectedEvent: rm.ProviderEventPaymentRequested,
		},
		"begin finalizing": {
			code:          datatransfer.BeginFinalizing,
			status:        datatransfer.Finalizing,
			expectedEvent: rm.ProviderEventLastPaymentRequested,
		},
		"new voucher": {
			code:          datatransfer.NewVoucher,
			status:        datatransfer.Ongoing,
			expectedEvent: rm.ProviderEventProcessPayment,
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ProviderDataTransferSubscriber(fdg)
			if !data.ignored {
				subscriber(datatransfer.Event{Code: data.code, Message: data.message}, shared_testutil.NewTestChannel(shared_testutil.TestChannelParams{
					IsPull:     true,
					TransferID: transferID,
					Sender:     testPeers[0],
					Recipient:  testPeers[1],
					Vouchers:   []datatransfer.TypedVoucher{dealProposalVoucher},
					Status:     data.status}))
				require.True(t, fdg.called)
				require.Equal(t, fdg.lastID, rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]})
				require.Equal(t, fdg.lastEvent, data.expectedEvent)
				require.Equal(t, fdg.lastArgs, data.expectedArgs)
			} else {
				subscriber(datatransfer.Event{Code: data.code, Message: data.message}, shared_testutil.NewTestChannel(shared_testutil.TestChannelParams{}))
				require.False(t, fdg.called)
			}
		})
	}

}
func TestClientDataTransferSubscriber(t *testing.T) {
	dealProposal := shared_testutil.MakeTestDealProposal()
	node := rm.BindnodeRegistry.TypeToNode(dealProposal)
	dealProposalVoucher := datatransfer.TypedVoucher{Voucher: node, Type: retrievalmarket.DealProposalType}
	dealResponseVoucher := func(dealResponse retrievalmarket.DealResponse) datatransfer.TypedVoucher {
		node := rm.BindnodeRegistry.TypeToNode(&dealResponse)
		return datatransfer.TypedVoucher{Voucher: node, Type: retrievalmarket.DealResponseType}
	}
	paymentOwed := shared_testutil.MakeTestTokenAmount()
	tests := map[string]struct {
		code          datatransfer.EventCode
		message       string
		state         shared_testutil.TestChannelParams
		ignored       bool
		expectedID    interface{}
		expectedEvent fsm.EventName
		expectedArgs  []interface{}
	}{
		"not a retrieval voucher": {
			ignored: true,
		},
		"progress": {
			code: datatransfer.DataReceivedProgress,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				Status:   datatransfer.Ongoing,
				Received: 1000},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventBlocksReceived,
			expectedArgs:  []interface{}{uint64(1000)},
		},
		"finish transfer": {
			code: datatransfer.FinishTransfer,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				Status:   datatransfer.TransferFinished},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventAllBlocksReceived,
		},
		"cancel": {
			code: datatransfer.Cancel,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				Status:   datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventProviderCancelled,
		},
		"new voucher result - rejected": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				VoucherResults: []datatransfer.TypedVoucher{dealResponseVoucher(retrievalmarket.DealResponse{
					Status:  retrievalmarket.DealStatusRejected,
					ID:      dealProposal.ID,
					Message: "something went wrong",
				})},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealRejected,
			expectedArgs:  []interface{}{"something went wrong"},
		},
		"new voucher result - not found": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				VoucherResults: []datatransfer.TypedVoucher{dealResponseVoucher(retrievalmarket.DealResponse{
					Status:  retrievalmarket.DealStatusDealNotFound,
					ID:      dealProposal.ID,
					Message: "something went wrong",
				})},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealNotFound,
			expectedArgs:  []interface{}{"something went wrong"},
		},
		"new voucher result - accepted": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				VoucherResults: []datatransfer.TypedVoucher{dealResponseVoucher(retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusAccepted,
					ID:     dealProposal.ID,
				})},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealAccepted,
		},
		"new voucher result - funds needed last payment": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				VoucherResults: []datatransfer.TypedVoucher{dealResponseVoucher(retrievalmarket.DealResponse{
					Status:      retrievalmarket.DealStatusFundsNeededLastPayment,
					ID:          dealProposal.ID,
					PaymentOwed: paymentOwed,
				})},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventLastPaymentRequested,
			expectedArgs:  []interface{}{paymentOwed},
		},
		"new voucher result - completed": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				VoucherResults: []datatransfer.TypedVoucher{dealResponseVoucher(retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusCompleted,
					ID:     dealProposal.ID,
				})},
				Status: datatransfer.ResponderCompleted},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventComplete,
		},
		"new voucher result - funds needed": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				VoucherResults: []datatransfer.TypedVoucher{dealResponseVoucher(retrievalmarket.DealResponse{
					Status:      retrievalmarket.DealStatusFundsNeeded,
					ID:          dealProposal.ID,
					PaymentOwed: paymentOwed,
				})},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventPaymentRequested,
			expectedArgs:  []interface{}{paymentOwed},
		},
		"new voucher result - unexpected response": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				VoucherResults: []datatransfer.TypedVoucher{dealResponseVoucher(retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusPaymentChannelAddingFunds,
					ID:     dealProposal.ID,
				})},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventUnknownResponseReceived,
			expectedArgs:  []interface{}{retrievalmarket.DealStatusPaymentChannelAddingFunds},
		},
		"error": {
			code:    datatransfer.Error,
			message: "something went wrong",
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				Status:   datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("deal data transfer failed: something went wrong")},
		},
		"disconnected": {
			code:    datatransfer.Disconnected,
			message: "something went wrong",
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				Status:   datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("deal data transfer stalled (peer hungup)")},
		},
		"error, response rejected": {
			code:    datatransfer.Error,
			message: datatransfer.ErrRejected.Error(),
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.TypedVoucher{dealProposalVoucher},
				Status:   datatransfer.Ongoing,
				Message:  datatransfer.ErrRejected.Error()},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealRejected,
			expectedArgs:  []interface{}{"rejected for unknown reasons"},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ClientDataTransferSubscriber(fdg)
			subscriber(datatransfer.Event{Code: data.code, Message: data.message}, shared_testutil.NewTestChannel(data.state))
			if !data.ignored {
				require.True(t, fdg.called)
				require.Equal(t, fdg.lastID, data.expectedID)
				require.Equal(t, fdg.lastEvent, data.expectedEvent)
				require.Equal(t, fdg.lastArgs, data.expectedArgs)
			} else {
				require.False(t, fdg.called)
			}
		})
	}
}

type fakeDealGroup struct {
	returnedErr error
	called      bool
	lastID      interface{}
	lastEvent   fsm.EventName
	lastArgs    []interface{}
}

func (fdg *fakeDealGroup) Send(id interface{}, name fsm.EventName, args ...interface{}) (err error) {
	fdg.lastID = id
	fdg.lastEvent = name
	fdg.lastArgs = args
	fdg.called = true
	return fdg.returnedErr
}

func TestTransportConfigurer(t *testing.T) {
	payloadCID := shared_testutil.GenerateCids(1)[0]
	expectedChannelID := shared_testutil.MakeTestChannelID()
	expectedDealID := rm.DealID(rand.Uint64())
	thisPeer := expectedChannelID.Initiator
	expectedPeer := expectedChannelID.Responder
	dealProposalVoucher := func(proposal rm.DealProposal) datatransfer.TypedVoucher {
		node := rm.BindnodeRegistry.TypeToNode(&proposal)
		return datatransfer.TypedVoucher{Voucher: node, Type: rm.DealProposalType}
	}

	testCases := map[string]struct {
		voucher          datatransfer.TypedVoucher
		returnedStore    bstore.Blockstore
		returnedStoreErr error
		getterCalled     bool
		useStoreCalled   bool
	}{
		"non-storage voucher": {
			voucher:      datatransfer.TypedVoucher{},
			getterCalled: false,
		},
		"store getter errors": {
			voucher: dealProposalVoucher(rm.DealProposal{
				PayloadCID: payloadCID,
				ID:         expectedDealID,
			}),
			getterCalled:     true,
			useStoreCalled:   false,
			returnedStore:    nil,
			returnedStoreErr: errors.New("something went wrong"),
		},
		"store getter succeeds": {
			voucher: dealProposalVoucher(rm.DealProposal{
				PayloadCID: payloadCID,
				ID:         expectedDealID,
			}),
			getterCalled:     true,
			useStoreCalled:   true,
			returnedStore:    bstore.NewBlockstore(ds.NewMapDatastore()),
			returnedStoreErr: nil,
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			storeGetter := &fakeStoreGetter{returnedErr: data.returnedStoreErr, returnedStore: data.returnedStore}
			transportConfigurer := dtutils.TransportConfigurer(thisPeer, storeGetter)
			options := transportConfigurer(expectedChannelID, data.voucher)
			if data.getterCalled {
				require.True(t, storeGetter.called)
				require.Equal(t, expectedDealID, storeGetter.lastDealID)
				require.Equal(t, expectedPeer, storeGetter.lastOtherPeer)
				if data.useStoreCalled {
					require.Len(t, options, 1)
				} else {
					require.Empty(t, options)
				}
			} else {
				require.False(t, storeGetter.called)
				require.Empty(t, options)
			}
		})
	}
}

type fakeStoreGetter struct {
	lastDealID    rm.DealID
	lastOtherPeer peer.ID
	returnedErr   error
	returnedStore bstore.Blockstore
	called        bool
}

func (fsg *fakeStoreGetter) Get(otherPeer peer.ID, dealID rm.DealID) (bstore.Blockstore, error) {
	fsg.lastDealID = dealID
	fsg.lastOtherPeer = otherPeer
	fsg.called = true
	return fsg.returnedStore, fsg.returnedErr
}
