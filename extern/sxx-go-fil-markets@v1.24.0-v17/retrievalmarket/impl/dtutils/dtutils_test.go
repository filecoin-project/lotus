package dtutils_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	ds "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	rm "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestProviderDataTransferSubscriber(t *testing.T) {
	dealProposal := shared_testutil.MakeTestDealProposal()
	legacyProposal := migrations.DealProposal0{
		PayloadCID: dealProposal.PayloadCID,
		ID:         dealProposal.ID,
		Params0: migrations.Params0{
			Selector:                dealProposal.Selector,
			PieceCID:                dealProposal.PieceCID,
			PricePerByte:            dealProposal.PricePerByte,
			PaymentInterval:         dealProposal.PaymentInterval,
			PaymentIntervalIncrease: dealProposal.PaymentIntervalIncrease,
			UnsealPrice:             dealProposal.UnsealPrice,
		},
	}
	testPeers := shared_testutil.GeneratePeers(2)
	transferID := datatransfer.TransferID(rand.Uint64())
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
		"accept": {
			code: datatransfer.Accept,
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&dealProposal},
				Status:     datatransfer.Ongoing},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventDealAccepted,
			expectedArgs:  []interface{}{datatransfer.ChannelID{ID: transferID, Initiator: testPeers[1], Responder: testPeers[0]}},
		},
		"accept, legacy": {
			code: datatransfer.Accept,
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&legacyProposal},
				Status:     datatransfer.Ongoing},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventDealAccepted,
			expectedArgs:  []interface{}{datatransfer.ChannelID{ID: transferID, Initiator: testPeers[1], Responder: testPeers[0]}},
		},
		"error": {
			code:    datatransfer.Error,
			message: "something went wrong",
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&dealProposal},
				Status:     datatransfer.Ongoing},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("deal data transfer failed: something went wrong")},
		},
		"disconnected": {
			code:    datatransfer.Disconnected,
			message: "something went wrong",
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&dealProposal},
				Status:     datatransfer.Ongoing},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("deal data transfer stalled (peer hungup)")},
		},
		"completed": {
			code: datatransfer.ResumeResponder,
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&dealProposal},
				Status:     datatransfer.Completed},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventComplete,
		},
		"cancel": {
			code: datatransfer.Cancel,
			state: shared_testutil.TestChannelParams{
				IsPull:     true,
				TransferID: transferID,
				Sender:     testPeers[0],
				Recipient:  testPeers[1],
				Vouchers:   []datatransfer.Voucher{&dealProposal},
				Status:     datatransfer.Completed},
			expectedID:    rm.ProviderDealIdentifier{DealID: dealProposal.ID, Receiver: testPeers[1]},
			expectedEvent: rm.ProviderEventClientCancelled,
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ProviderDataTransferSubscriber(fdg)
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
func TestClientDataTransferSubscriber(t *testing.T) {
	dealProposal := shared_testutil.MakeTestDealProposal()
	legacyProposal := migrations.DealProposal0{
		PayloadCID: dealProposal.PayloadCID,
		ID:         dealProposal.ID,
		Params0: migrations.Params0{
			Selector:                dealProposal.Selector,
			PieceCID:                dealProposal.PieceCID,
			PricePerByte:            dealProposal.PricePerByte,
			PaymentInterval:         dealProposal.PaymentInterval,
			PaymentIntervalIncrease: dealProposal.PaymentIntervalIncrease,
			UnsealPrice:             dealProposal.UnsealPrice,
		},
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
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.Ongoing,
				Received: 1000},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventBlocksReceived,
			expectedArgs:  []interface{}{uint64(1000)},
		},
		"finish transfer": {
			code: datatransfer.FinishTransfer,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.TransferFinished},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventAllBlocksReceived,
		},
		"cancel": {
			code: datatransfer.Cancel,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventProviderCancelled,
		},
		"new voucher result - rejected": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status:  retrievalmarket.DealStatusRejected,
					ID:      dealProposal.ID,
					Message: "something went wrong",
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealRejected,
			expectedArgs:  []interface{}{"something went wrong"},
		},
		"new voucher result - not found": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status:  retrievalmarket.DealStatusDealNotFound,
					ID:      dealProposal.ID,
					Message: "something went wrong",
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealNotFound,
			expectedArgs:  []interface{}{"something went wrong"},
		},
		"new voucher result - accepted": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusAccepted,
					ID:     dealProposal.ID,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealAccepted,
		},
		"new voucher result - accepted, legacy": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&legacyProposal},
				VoucherResults: []datatransfer.VoucherResult{&migrations.DealResponse0{
					Status: retrievalmarket.DealStatusAccepted,
					ID:     dealProposal.ID,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDealAccepted,
		},
		"new voucher result - funds needed last payment": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status:      retrievalmarket.DealStatusFundsNeededLastPayment,
					ID:          dealProposal.ID,
					PaymentOwed: paymentOwed,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventLastPaymentRequested,
			expectedArgs:  []interface{}{paymentOwed},
		},
		"new voucher result - completed": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusCompleted,
					ID:     dealProposal.ID,
				}},
				Status: datatransfer.ResponderCompleted},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventComplete,
		},
		"new voucher result - funds needed": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status:      retrievalmarket.DealStatusFundsNeeded,
					ID:          dealProposal.ID,
					PaymentOwed: paymentOwed,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventPaymentRequested,
			expectedArgs:  []interface{}{paymentOwed},
		},
		"new voucher result - unexpected response": {
			code: datatransfer.NewVoucherResult,
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				VoucherResults: []datatransfer.VoucherResult{&retrievalmarket.DealResponse{
					Status: retrievalmarket.DealStatusPaymentChannelAddingFunds,
					ID:     dealProposal.ID,
				}},
				Status: datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventUnknownResponseReceived,
			expectedArgs:  []interface{}{retrievalmarket.DealStatusPaymentChannelAddingFunds},
		},
		"error": {
			code:    datatransfer.Error,
			message: "something went wrong",
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("deal data transfer failed: something went wrong")},
		},
		"disconnected": {
			code:    datatransfer.Disconnected,
			message: "something went wrong",
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
				Status:   datatransfer.Ongoing},
			expectedID:    dealProposal.ID,
			expectedEvent: rm.ClientEventDataTransferError,
			expectedArgs:  []interface{}{errors.New("deal data transfer stalled (peer hungup)")},
		},
		"error, response rejected": {
			code:    datatransfer.Error,
			message: datatransfer.ErrRejected.Error(),
			state: shared_testutil.TestChannelParams{
				Vouchers: []datatransfer.Voucher{&dealProposal},
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

	testCases := map[string]struct {
		voucher          datatransfer.Voucher
		transport        datatransfer.Transport
		returnedStore    bstore.Blockstore
		returnedStoreErr error
		getterCalled     bool
		useStoreCalled   bool
	}{
		"non-storage voucher": {
			voucher:      nil,
			getterCalled: false,
		},
		"non-configurable transport": {
			voucher: &rm.DealProposal{
				PayloadCID: payloadCID,
				ID:         expectedDealID,
			},
			transport:    &fakeTransport{},
			getterCalled: false,
		},
		"store getter errors": {
			voucher: &rm.DealProposal{
				PayloadCID: payloadCID,
				ID:         expectedDealID,
			},
			transport:        &fakeGsTransport{Transport: &fakeTransport{}},
			getterCalled:     true,
			useStoreCalled:   false,
			returnedStore:    nil,
			returnedStoreErr: errors.New("something went wrong"),
		},
		"store getter succeeds": {
			voucher: &rm.DealProposal{
				PayloadCID: payloadCID,
				ID:         expectedDealID,
			},
			transport:        &fakeGsTransport{Transport: &fakeTransport{}},
			getterCalled:     true,
			useStoreCalled:   true,
			returnedStore:    bstore.NewBlockstore(ds.NewMapDatastore()),
			returnedStoreErr: nil,
		},
		"store getter succeeds, legacy": {
			voucher: &migrations.DealProposal0{
				PayloadCID: payloadCID,
				ID:         expectedDealID,
			},
			transport:        &fakeGsTransport{Transport: &fakeTransport{}},
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
			transportConfigurer(expectedChannelID, data.voucher, data.transport)
			if data.getterCalled {
				require.True(t, storeGetter.called)
				require.Equal(t, expectedDealID, storeGetter.lastDealID)
				require.Equal(t, expectedPeer, storeGetter.lastOtherPeer)
				fgt, ok := data.transport.(*fakeGsTransport)
				require.True(t, ok)
				if data.useStoreCalled {
					require.True(t, fgt.called)
					require.Equal(t, expectedChannelID, fgt.lastChannelID)
				} else {
					require.False(t, fgt.called)
				}
			} else {
				require.False(t, storeGetter.called)
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

type fakeTransport struct{}

var _ datatransfer.Transport = (*fakeTransport)(nil)

func (ft *fakeTransport) OpenChannel(ctx context.Context, dataSender peer.ID, channelID datatransfer.ChannelID, root ipld.Link, stor ipld.Node, channel datatransfer.ChannelState, msg datatransfer.Message) error {
	return nil
}

func (ft *fakeTransport) CloseChannel(ctx context.Context, chid datatransfer.ChannelID) error {
	return nil
}

func (ft *fakeTransport) SetEventHandler(events datatransfer.EventsHandler) error {
	return nil
}

func (ft *fakeTransport) CleanupChannel(chid datatransfer.ChannelID) {
}

func (ft *fakeTransport) Shutdown(context.Context) error {
	return nil
}

type fakeGsTransport struct {
	datatransfer.Transport
	lastChannelID  datatransfer.ChannelID
	lastLinkSystem ipld.LinkSystem
	called         bool
}

func (fgt *fakeGsTransport) UseStore(channelID datatransfer.ChannelID, lsys ipld.LinkSystem) error {
	fgt.lastChannelID = channelID
	fgt.lastLinkSystem = lsys
	fgt.called = true
	return nil
}
