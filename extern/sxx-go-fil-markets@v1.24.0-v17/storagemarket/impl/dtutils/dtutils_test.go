package dtutils_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	bs "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-statemachine/fsm"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/dtutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/requestvalidation"
)

func TestProviderDataTransferSubscriber(t *testing.T) {
	ps := shared_testutil.GeneratePeers(2)
	init := ps[0]
	resp := ps[1]
	tid := datatransfer.TransferID(1)
	expectedProposalCID := shared_testutil.GenerateCids(1)[0]
	tests := map[string]struct {
		code          datatransfer.EventCode
		message       string
		status        datatransfer.Status
		called        bool
		voucher       datatransfer.Voucher
		expectedID    interface{}
		expectedEvent fsm.EventName
		expectedArgs  []interface{}
	}{
		"not a storage voucher": {
			called:  false,
			voucher: nil,
		},
		"open event": {
			code:   datatransfer.Open,
			status: datatransfer.Requested,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferInitiated,
			expectedArgs:  []interface{}{datatransfer.ChannelID{Initiator: init, Responder: resp, ID: tid}},
		},
		"restart event": {
			code:   datatransfer.Restart,
			status: datatransfer.Ongoing,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferRestarted,
			expectedArgs:  []interface{}{datatransfer.ChannelID{Initiator: init, Responder: resp, ID: tid}},
		},
		"disconnected event": {
			code:   datatransfer.Disconnected,
			status: datatransfer.Ongoing,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferStalled,
		},
		"completion status": {
			code:   datatransfer.Complete,
			status: datatransfer.Completed,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferCompleted,
		},
		"data received": {
			code:   datatransfer.DataReceived,
			status: datatransfer.Ongoing,
			called: false,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID: expectedProposalCID,
		},
		"error event": {
			code:    datatransfer.Error,
			message: "something went wrong",
			status:  datatransfer.Failed,
			called:  true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ProviderEventDataTransferFailed,
			expectedArgs:  []interface{}{errors.New("deal data transfer failed: something went wrong")},
		},
		"other event": {
			code:   datatransfer.DataSent,
			status: datatransfer.Ongoing,
			called: false,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
		},
	}
	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ProviderDataTransferSubscriber(fdg)
			subscriber(datatransfer.Event{Code: data.code, Message: data.message}, shared_testutil.NewTestChannel(
				shared_testutil.TestChannelParams{Vouchers: []datatransfer.Voucher{data.voucher}, Status: data.status,
					Sender: init, Recipient: resp, TransferID: tid, IsPull: false},
			))
			if data.called {
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
	ps := shared_testutil.GeneratePeers(2)
	init := ps[0]
	resp := ps[1]
	tid := datatransfer.TransferID(1)

	expectedProposalCID := shared_testutil.GenerateCids(1)[0]
	tests := map[string]struct {
		code          datatransfer.EventCode
		message       string
		status        datatransfer.Status
		called        bool
		voucher       datatransfer.Voucher
		expectedID    interface{}
		expectedEvent fsm.EventName
		expectedArgs  []interface{}
	}{
		"not a storage voucher": {
			called:  false,
			voucher: nil,
		},
		"completion event": {
			code:   datatransfer.Complete,
			status: datatransfer.Completed,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ClientEventDataTransferComplete,
		},
		"restart event": {
			code:   datatransfer.Restart,
			status: datatransfer.Ongoing,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ClientEventDataTransferRestarted,
			expectedArgs:  []interface{}{datatransfer.ChannelID{Initiator: init, Responder: resp, ID: tid}},
		},
		"disconnected event": {
			code:   datatransfer.Disconnected,
			status: datatransfer.Ongoing,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ClientEventDataTransferStalled,
		},
		"accept event": {
			code:   datatransfer.Accept,
			status: datatransfer.Requested,
			called: true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ClientEventDataTransferInitiated,
			expectedArgs:  []interface{}{datatransfer.ChannelID{Initiator: init, Responder: resp, ID: tid}},
		},
		"error event": {
			code:    datatransfer.Error,
			message: "something went wrong",
			status:  datatransfer.Failed,
			called:  true,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			expectedID:    expectedProposalCID,
			expectedEvent: storagemarket.ClientEventDataTransferFailed,
			expectedArgs:  []interface{}{errors.New("deal data transfer failed: something went wrong")},
		},
		"other event": {
			code:   datatransfer.DataReceived,
			status: datatransfer.Ongoing,
			called: false,
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
		},
	}

	for test, data := range tests {
		t.Run(test, func(t *testing.T) {
			fdg := &fakeDealGroup{}
			subscriber := dtutils.ClientDataTransferSubscriber(fdg)
			subscriber(datatransfer.Event{Code: data.code, Message: data.message}, shared_testutil.NewTestChannel(
				shared_testutil.TestChannelParams{Vouchers: []datatransfer.Voucher{data.voucher}, Status: data.status,
					Sender: init, Recipient: resp, TransferID: tid, IsPull: false},
			))
			if data.called {
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

func TestTransportConfigurer(t *testing.T) {
	expectedProposalCID := shared_testutil.GenerateCids(1)[0]
	expectedChannelID := shared_testutil.MakeTestChannelID()

	testCases := map[string]struct {
		voucher          datatransfer.Voucher
		transport        datatransfer.Transport
		returnedStore    bs.Blockstore
		returnedStoreErr error
		getterCalled     bool
		useStoreCalled   bool
	}{
		"non-storage voucher": {
			voucher:      nil,
			getterCalled: false,
		},
		"non-configurable transport": {
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			transport:    &fakeTransport{},
			getterCalled: false,
		},
		"store getter errors": {
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			transport:        &fakeGsTransport{Transport: &fakeTransport{}},
			getterCalled:     true,
			useStoreCalled:   false,
			returnedStore:    nil,
			returnedStoreErr: errors.New("something went wrong"),
		},
		"store getter succeeds": {
			voucher: &requestvalidation.StorageDataTransferVoucher{
				Proposal: expectedProposalCID,
			},
			transport:        &fakeGsTransport{Transport: &fakeTransport{}},
			getterCalled:     true,
			useStoreCalled:   true,
			returnedStore:    bs.NewBlockstore(ds.NewMapDatastore()),
			returnedStoreErr: nil,
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			storeGetter := &fakeStoreGetter{returnedErr: data.returnedStoreErr, returnedStore: data.returnedStore}
			transportConfigurer := dtutils.TransportConfigurer(storeGetter)
			transportConfigurer(expectedChannelID, data.voucher, data.transport)
			if data.getterCalled {
				require.True(t, storeGetter.called)
				require.Equal(t, expectedProposalCID, storeGetter.lastProposalCid)
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

type fakeStoreGetter struct {
	lastProposalCid cid.Cid
	returnedErr     error
	returnedStore   bs.Blockstore
	called          bool
}

func (fsg *fakeStoreGetter) Get(proposalCid cid.Cid) (bs.Blockstore, error) {
	fsg.lastProposalCid = proposalCid
	fsg.called = true
	return fsg.returnedStore, fsg.returnedErr
}

type fakeTransport struct{}

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
