package graphsyncimpl_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-graphsync"
	gsimpl "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/ipldbridge"
	gsmsg "github.com/ipfs/go-graphsync/message"
	gsnet "github.com/ipfs/go-graphsync/network"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/datatransfer"
	. "github.com/filecoin-project/lotus/datatransfer/impl/graphsync"
	"github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/network"
	"github.com/filecoin-project/lotus/datatransfer/testutil"
)

type receivedMessage struct {
	message message.DataTransferMessage
	sender  peer.ID
}

// Receiver is an interface for receiving messages from the GraphSyncNetwork.
type receiver struct {
	messageReceived chan receivedMessage
}

func (r *receiver) ReceiveRequest(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferRequest) {

	select {
	case <-ctx.Done():
	case r.messageReceived <- receivedMessage{incoming, sender}:
	}
}

func (r *receiver) ReceiveResponse(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferResponse) {

	select {
	case <-ctx.Done():
	case r.messageReceived <- receivedMessage{incoming, sender}:
	}
}

func (r *receiver) ReceiveError(err error) {
}

type fakeDTType struct {
	data string
}

func (ft *fakeDTType) ToBytes() ([]byte, error) {
	return []byte(ft.data), nil
}

func (ft *fakeDTType) FromBytes(data []byte) error {
	ft.data = string(data)
	return nil
}

func (ft *fakeDTType) Type() string {
	return "FakeDTType"
}

func TestDataTransferOneWay(t *testing.T) {
	// create network
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	gsData := newGraphsyncTestingData(t, ctx)
	host1 := gsData.host1
	host2 := gsData.host2
	// setup receiving peer to just record message coming in
	dtnet2 := network.NewFromLibp2pHost(host2)
	r := &receiver{
		messageReceived: make(chan receivedMessage),
	}
	dtnet2.SetDelegate(r)

	gs := gsData.setupGraphsyncHost1()
	dt := NewGraphSyncDataTransfer(ctx, host1, gs)

	t.Run("OpenPushDataTransfer", func(t *testing.T) {
		ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

		// this is the selector for "get the whole DAG"
		// TODO: support storage deals with custom payload selectors
		stor := ssb.ExploreRecursive(selector.RecursionLimitNone(),
			ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

		voucher := fakeDTType{"applesauce"}
		baseCid := testutil.GenerateCids(1)[0]
		channelID, err := dt.OpenPushDataChannel(ctx, host2.ID(), &voucher, baseCid, stor)
		require.NoError(t, err)
		require.NotNil(t, channelID)
		require.Equal(t, channelID.To, host2.ID())
		require.NoError(t, err)

		var messageReceived receivedMessage
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case messageReceived = <-r.messageReceived:
		}

		sender := messageReceived.sender
		require.Equal(t, sender, host1.ID())

		received := messageReceived.message
		require.True(t, received.IsRequest())
		receivedRequest, ok := received.(message.DataTransferRequest)
		require.True(t, ok)

		require.Equal(t, receivedRequest.TransferID(), channelID.ID)
		require.Equal(t, receivedRequest.BaseCid(), baseCid)
		require.False(t, receivedRequest.IsCancel())
		require.False(t, receivedRequest.IsPull())
		reader := bytes.NewReader(receivedRequest.Selector())
		receivedSelector, err := dagcbor.Decoder(ipldfree.NodeBuilder(), reader)
		require.NoError(t, err)
		require.Equal(t, receivedSelector, stor)
		receivedVoucher := new(fakeDTType)
		err = receivedVoucher.FromBytes(receivedRequest.Voucher())
		require.NoError(t, err)
		require.Equal(t, *receivedVoucher, voucher)
		require.Equal(t, receivedRequest.VoucherType(), voucher.Type())
	})

	// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/16
	t.Run("OpenPullDataTransfer", func(t *testing.T) {
		ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

		stor := ssb.ExploreRecursive(selector.RecursionLimitNone(),
			ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

		voucher := fakeDTType{"applesauce"}
		baseCid := testutil.GenerateCids(1)[0]
		channelID, err := dt.OpenPullDataChannel(ctx, host2.ID(), &voucher, baseCid, stor)
		require.NoError(t, err)
		require.NotNil(t, channelID)
		require.Equal(t, channelID.To, host2.ID())
		require.NoError(t, err)

		var messageReceived receivedMessage
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case messageReceived = <-r.messageReceived:
		}

		sender := messageReceived.sender
		require.Equal(t, sender, host1.ID())

		received := messageReceived.message
		require.True(t, received.IsRequest())
		receivedRequest, ok := received.(message.DataTransferRequest)
		require.True(t, ok)

		require.Equal(t, receivedRequest.TransferID(), channelID.ID)
		require.Equal(t, receivedRequest.BaseCid(), baseCid)
		require.False(t, receivedRequest.IsCancel())
		require.True(t, receivedRequest.IsPull())
		reader := bytes.NewReader(receivedRequest.Selector())
		receivedSelector, err := dagcbor.Decoder(ipldfree.NodeBuilder(), reader)
		require.NoError(t, err)
		require.Equal(t, receivedSelector, stor)
		receivedVoucher := new(fakeDTType)
		err = receivedVoucher.FromBytes(receivedRequest.Voucher())
		require.NoError(t, err)
		require.Equal(t, *receivedVoucher, voucher)
		require.Equal(t, receivedRequest.VoucherType(), voucher.Type())
	})
}

type receivedValidation struct {
	isPull   bool
	other    peer.ID
	voucher  datatransfer.Voucher
	baseCid  cid.Cid
	selector ipld.Node
}

type fakeValidator struct {
	ctx                 context.Context
	validationsReceived chan receivedValidation
}

func (fv *fakeValidator) ValidatePush(
	sender peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	selector ipld.Node) error {

	select {
	case <-fv.ctx.Done():
	case fv.validationsReceived <- receivedValidation{false, sender, voucher, baseCid, selector}:
	}
	return nil
}

func (fv *fakeValidator) ValidatePull(
	receiver peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	selector ipld.Node) error {

	select {
	case <-fv.ctx.Done():
	case fv.validationsReceived <- receivedValidation{true, receiver, voucher, baseCid, selector}:
	}
	return nil
}

func TestDataTransferValidation(t *testing.T) {
	// create network
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	gsData := newGraphsyncTestingData(t, ctx)
	host1 := gsData.host1
	host2 := gsData.host2

	gs1 := &fakeGraphSync{
		receivedRequests: make(chan receivedGraphSyncRequest, 1),
	}
	dt1 := NewGraphSyncDataTransfer(ctx, host1, gs1)

	gs2 := &fakeGraphSync{
		receivedRequests: make(chan receivedGraphSyncRequest, 1),
	}
	dt2 := NewGraphSyncDataTransfer(ctx, host2, gs2)

	fv := &fakeValidator{ctx, make(chan receivedValidation)}

	err := dt2.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), fv)
	require.NoError(t, err)

	t.Run("ValidatePush", func(t *testing.T) {

		voucher := fakeDTType{"applesauce"}
		baseCid := testutil.GenerateCids(1)[0]
		dt1.OpenPushDataChannel(ctx, host2.ID(), &voucher, baseCid, gsData.allSelector)
		var validation receivedValidation
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case validation = <-fv.validationsReceived:
			assert.False(t, validation.isPull)
		}

		assert.False(t, validation.isPull)
		assert.Equal(t, host1.ID(), validation.other)
		assert.Equal(t, &voucher, validation.voucher)
		assert.Equal(t, baseCid, validation.baseCid)
		assert.Equal(t, gsData.allSelector, validation.selector)
	})

	t.Run("ValidatePull", func(t *testing.T) {

		voucher := fakeDTType{"applesauce"}
		baseCid := testutil.GenerateCids(1)[0]
		channelID, err := dt1.OpenPullDataChannel(ctx, host2.ID(), &voucher, baseCid, gsData.allSelector)
		require.NoError(t, err)

		assert.Equal(t, channelID.To, host2.ID())

		var validation receivedValidation
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case validation = <-fv.validationsReceived:
		}

		assert.True(t, validation.isPull)
		assert.Equal(t, validation.other, host1.ID())
		assert.Equal(t, &voucher, validation.voucher)
		assert.Equal(t, baseCid, validation.baseCid)
		assert.Equal(t, gsData.allSelector, validation.selector)
	})
}

type stubbedValidator struct {
	didPush    bool
	didPull    bool
	expectPush bool
	expectPull bool
	pushError  error
	pullError  error
}

func newSV() *stubbedValidator {
	return &stubbedValidator{false, false, false, false, nil, nil}
}

func (sv *stubbedValidator) ValidatePush(
	sender peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	selector ipld.Node) error {
	sv.didPush = true
	return sv.pushError
}

func (sv *stubbedValidator) ValidatePull(
	receiver peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	selector ipld.Node) error {
	sv.didPull = true
	return sv.pullError
}

func (sv *stubbedValidator) stubErrorPush() {
	sv.pushError = errors.New("something went wrong")
}

func (sv *stubbedValidator) stubSuccessPush() {
	sv.pullError = nil
}

func (sv *stubbedValidator) expectSuccessPush() {
	sv.expectPush = true
	sv.stubSuccessPush()
}

func (sv *stubbedValidator) expectErrorPush() {
	sv.expectPush = true
	sv.stubErrorPush()
}

func (sv *stubbedValidator) stubErrorPull() {
	sv.pullError = errors.New("something went wrong")
}

func (sv *stubbedValidator) stubSuccessPull() {
	sv.pullError = nil
}

func (sv *stubbedValidator) expectSuccessPull() {
	sv.expectPull = true
	sv.stubSuccessPull()
}

func (sv *stubbedValidator) expectErrorPull() {
	sv.expectPull = true
	sv.stubErrorPull()
}

func (sv *stubbedValidator) verifyExpectations(t *testing.T) {
	if sv.expectPush {
		require.True(t, sv.didPush)
	}
	if sv.expectPull {
		require.True(t, sv.didPull)
	}
}

func TestSendResponseToIncomingRequest(t *testing.T) {
	// create network
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	gsData := newGraphsyncTestingData(t, ctx)
	host1 := gsData.host1
	host2 := gsData.host2

	// setup receiving peer to just record message coming in
	dtnet1 := network.NewFromLibp2pHost(host1)
	r := &receiver{
		messageReceived: make(chan receivedMessage),
	}
	dtnet1.SetDelegate(r)

	gs2 := &fakeGraphSync{
		receivedRequests: make(chan receivedGraphSyncRequest, 1),
	}

	voucher := fakeDTType{"applesauce"}
	baseCid := testutil.GenerateCids(1)[0]
	id := datatransfer.TransferID(rand.Int31())
	var buffer bytes.Buffer
	err := dagcbor.Encoder(gsData.allSelector, &buffer)
	require.NoError(t, err)

	t.Run("Response to push with successful validation", func(t *testing.T) {
		sv := newSV()
		sv.expectSuccessPush()

		dt := NewGraphSyncDataTransfer(ctx, host2, gs2)
		require.NoError(t, dt.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv))

		isPull := false
		voucherBytes, err := voucher.ToBytes()
		require.NoError(t, err)
		_ = message.NewRequest(id, isPull, voucher.Type(), voucherBytes, baseCid, buffer.Bytes())
		request := message.NewRequest(id, isPull, voucher.Type(), voucherBytes, baseCid, buffer.Bytes())
		require.NoError(t, dtnet1.SendMessage(ctx, host2.ID(), request))
		var messageReceived receivedMessage
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case messageReceived = <-r.messageReceived:
		}

		sv.verifyExpectations(t)

		sender := messageReceived.sender
		require.Equal(t, sender, host2.ID())

		received := messageReceived.message
		require.False(t, received.IsRequest())
		receivedResponse, ok := received.(message.DataTransferResponse)
		require.True(t, ok)

		assert.Equal(t, receivedResponse.TransferID(), id)
		require.True(t, receivedResponse.Accepted())

	})

	t.Run("Response to push with error validation", func(t *testing.T) {
		sv := newSV()
		sv.expectErrorPush()
		dt := NewGraphSyncDataTransfer(ctx, host2, gs2)
		err = dt.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv)
		require.NoError(t, err)

		isPull := false

		voucherBytes, err := voucher.ToBytes()
		require.NoError(t, err)
		request := message.NewRequest(id, isPull, voucher.Type(), voucherBytes, baseCid, buffer.Bytes())
		require.NoError(t, dtnet1.SendMessage(ctx, host2.ID(), request))

		var messageReceived receivedMessage
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case messageReceived = <-r.messageReceived:
		}

		sv.verifyExpectations(t)

		sender := messageReceived.sender
		require.Equal(t, sender, host2.ID())

		received := messageReceived.message
		require.False(t, received.IsRequest())
		receivedResponse, ok := received.(message.DataTransferResponse)
		require.True(t, ok)

		require.Equal(t, receivedResponse.TransferID(), id)
		require.False(t, receivedResponse.Accepted())
	})

	t.Run("Response to pull with successful validation", func(t *testing.T) {
		sv := newSV()
		sv.expectSuccessPull()

		dt := NewGraphSyncDataTransfer(ctx, host2, gs2)
		err = dt.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv)
		require.NoError(t, err)

		isPull := true

		voucherBytes, err := voucher.ToBytes()
		require.NoError(t, err)
		request := message.NewRequest(id, isPull, voucher.Type(), voucherBytes, baseCid, buffer.Bytes())

		require.NoError(t, dtnet1.SendMessage(ctx, host2.ID(), request))
		var messageReceived receivedMessage
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case messageReceived = <-r.messageReceived:
		}

		sv.verifyExpectations(t)

		sender := messageReceived.sender
		require.Equal(t, sender, host2.ID())

		received := messageReceived.message
		require.False(t, received.IsRequest())
		receivedResponse, ok := received.(message.DataTransferResponse)
		require.True(t, ok)

		require.Equal(t, receivedResponse.TransferID(), id)
		require.True(t, receivedResponse.Accepted())
	})

	t.Run("Response to push with error validation", func(t *testing.T) {
		sv := newSV()
		sv.expectErrorPull()

		dt := NewGraphSyncDataTransfer(ctx, host2, gs2)
		err = dt.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv)
		require.NoError(t, err)

		isPull := true
		voucherBytes, err := voucher.ToBytes()
		require.NoError(t, err)
		request := message.NewRequest(id, isPull, voucher.Type(), voucherBytes, baseCid, buffer.Bytes())
		require.NoError(t, dtnet1.SendMessage(ctx, host2.ID(), request))

		var messageReceived receivedMessage
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case messageReceived = <-r.messageReceived:
		}

		sv.verifyExpectations(t)

		sender := messageReceived.sender
		require.Equal(t, sender, host2.ID())

		received := messageReceived.message
		require.False(t, received.IsRequest())
		receivedResponse, ok := received.(message.DataTransferResponse)
		require.True(t, ok)

		require.Equal(t, receivedResponse.TransferID(), id)
		require.False(t, receivedResponse.Accepted())
	})
}

func TestGraphsyncImpl_RegisterVoucherType(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	mn := mocknet.New(ctx)
	// setup network
	host1, err := mn.GenPeer()
	require.NoError(t, err)

	gs1 := &fakeGraphSync{
		receivedRequests: make(chan receivedGraphSyncRequest, 1),
	}
	dt := NewGraphSyncDataTransfer(ctx, host1, gs1)
	fv := &fakeValidator{ctx, make(chan receivedValidation)}

	// a voucher type can be registered
	assert.NoError(t, dt.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), fv))

	// it cannot be re-registered
	assert.EqualError(t, dt.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), fv), "voucher type already registered: *graphsyncimpl_test.fakeDTType")

	// it must be registered as a pointer
	assert.EqualError(t, dt.RegisterVoucherType(reflect.TypeOf(fakeDTType{}), fv),
		"voucherType must be a reflect.Ptr Kind")
}

func TestDataTransferSubscribing(t *testing.T) {
	// create network
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	gsData := newGraphsyncTestingData(t, ctx)
	host1 := gsData.host1
	host2 := gsData.host2

	gs1 := &fakeGraphSync{
		receivedRequests: make(chan receivedGraphSyncRequest, 1),
	}
	gs2 := &fakeGraphSync{
		receivedRequests: make(chan receivedGraphSyncRequest, 1),
	}
	sv := newSV()
	sv.stubErrorPull()
	sv.stubErrorPush()
	dt2 := NewGraphSyncDataTransfer(ctx, host2, gs2)
	require.NoError(t, dt2.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv))
	voucher := fakeDTType{"applesauce"}
	baseCid := testutil.GenerateCids(1)[0]

	dt1 := NewGraphSyncDataTransfer(ctx, host1, gs1)

	subscribe1Calls := make(chan struct{}, 1)
	subscribe1 := func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		if event == datatransfer.Error {
			subscribe1Calls <- struct{}{}
		}
	}
	subscribe2Calls := make(chan struct{}, 1)
	subscribe2 := func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		if event == datatransfer.Error {
			subscribe2Calls <- struct{}{}
		}
	}
	unsub1 := dt1.SubscribeToEvents(subscribe1)
	unsub2 := dt1.SubscribeToEvents(subscribe2)
	_, err := dt1.OpenPushDataChannel(ctx, host2.ID(), &voucher, baseCid, gsData.allSelector)
	require.NoError(t, err)
	select {
	case <-ctx.Done():
		t.Fatal("subscribed events not received")
	case <-subscribe1Calls:
	}
	select {
	case <-ctx.Done():
		t.Fatal("subscribed events not received")
	case <-subscribe2Calls:
	}
	unsub1()
	unsub2()

	subscribe3Calls := make(chan struct{}, 1)
	subscribe3 := func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		if event == datatransfer.Error {
			subscribe3Calls <- struct{}{}
		}
	}
	subscribe4Calls := make(chan struct{}, 1)
	subscribe4 := func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		if event == datatransfer.Error {
			subscribe4Calls <- struct{}{}
		}
	}
	unsub3 := dt1.SubscribeToEvents(subscribe3)
	unsub4 := dt1.SubscribeToEvents(subscribe4)
	_, err = dt1.OpenPullDataChannel(ctx, host2.ID(), &voucher, baseCid, gsData.allSelector)
	require.NoError(t, err)
	select {
	case <-ctx.Done():
		t.Fatal("subscribed events not received")
	case <-subscribe1Calls:
		t.Fatal("received channel that should have been unsubscribed")
	case <-subscribe2Calls:
		t.Fatal("received channel that should have been unsubscribed")
	case <-subscribe3Calls:
	}
	select {
	case <-ctx.Done():
		t.Fatal("subscribed events not received")
	case <-subscribe1Calls:
		t.Fatal("received channel that should have been unsubscribed")
	case <-subscribe2Calls:
		t.Fatal("received channel that should have been unsubscribed")
	case <-subscribe4Calls:
	}
	unsub3()
	unsub4()
}

func TestDataTransferInitiatingPushGraphsyncRequests(t *testing.T) {
	// create network
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	gsData := newGraphsyncTestingData(t, ctx)
	host1 := gsData.host1
	host2 := gsData.host2

	gs2 := &fakeGraphSync{
		receivedRequests: make(chan receivedGraphSyncRequest, 1),
	}

	// setup receiving peer to just record message coming in
	dtnet1 := network.NewFromLibp2pHost(host1)
	r := &receiver{
		messageReceived: make(chan receivedMessage),
	}
	dtnet1.SetDelegate(r)

	voucher := fakeDTType{"applesauce"}
	baseCid := testutil.GenerateCids(1)[0]
	id := datatransfer.TransferID(rand.Int31())
	var buffer bytes.Buffer

	err := dagcbor.Encoder(gsData.allSelector, &buffer)
	require.NoError(t, err)

	isPull := false
	voucherBytes, err := voucher.ToBytes()
	require.NoError(t, err)
	request := message.NewRequest(id, isPull, voucher.Type(), voucherBytes, baseCid, buffer.Bytes())

	t.Run("with successful validation", func(t *testing.T) {
		sv := newSV()
		sv.expectSuccessPush()

		dt2 := NewGraphSyncDataTransfer(ctx, host2, gs2)
		require.NoError(t, dt2.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv))

		require.NoError(t, dtnet1.SendMessage(ctx, host2.ID(), request))
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case <-r.messageReceived:
		}
		sv.verifyExpectations(t)

		var requestReceived receivedGraphSyncRequest
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case requestReceived = <-gs2.receivedRequests:
		}

		sv.verifyExpectations(t)

		receiver := requestReceived.p
		require.Equal(t, receiver, host1.ID())

		cl, ok := requestReceived.root.(cidlink.Link)
		require.True(t, ok)
		require.Equal(t, baseCid, cl.Cid)

		require.Equal(t, gsData.allSelector, requestReceived.selector)

	})

	t.Run("with error validation", func(t *testing.T) {
		sv := newSV()
		sv.expectErrorPush()

		dt2 := NewGraphSyncDataTransfer(ctx, host2, gs2)
		require.NoError(t, dt2.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv))

		require.NoError(t, dtnet1.SendMessage(ctx, host2.ID(), request))
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case <-r.messageReceived:
		}
		sv.verifyExpectations(t)

		// no graphsync request should be scheduled
		require.Empty(t, gs2.receivedRequests)

	})
}

// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/21
func TestDataTransferInitiatingPullGraphsyncRequests(t *testing.T) {
	//ctx := context.Background()
	//ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	//defer cancel()
	//gsData := newGraphsyncTestingData(t, ctx)
	//host1 := gsData.host1
	//host2 := gsData.host2
	//
	//gs2 := &fakeGraphSync{
	//	receivedRequests: make(chan receivedGraphSyncRequest, 1),
	//}
	//voucher := fakeDTType{"applesauce"}
	//baseCid := testutil.GenerateCids(1)[0]
	//
	//gs1 := &fakeGraphSync{
	//	receivedRequests: make(chan receivedGraphSyncRequest, 1),
	//}
	//dt1 := NewGraphSyncDataTransfer(ctx, host1, gs1)
	//
	//t.Run("with successful validation", func(t *testing.T) {
	//	sv := newSV()
	//	sv.expectSuccessPull()
	//
	//	dt2 := NewGraphSyncDataTransfer(ctx, host2, gs2)
	//	err := dt2.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv)
	//	require.NoError(t, err)
	//
	//	_, err = dt1.OpenPullDataChannel(ctx, host2.ID(), &voucher, baseCid, gsData.allSelector)
	//	require.NoError(t, err)
	//
	//	var requestReceived receivedGraphSyncRequest
	//	select {
	//	case <-ctx.Done():
	//		t.Fatal("did not receive message sent")
	//	case requestReceived = <-gs1.receivedRequests:
	//	}
	//
	//	sv.verifyExpectations(t)
	//
	//	receiver := requestReceived.p
	//	require.Equal(t, receiver, host2.ID())
	//
	//	cl, ok := requestReceived.root.(cidlink.Link)
	//	require.True(t, ok)
	//	require.Equal(t, baseCid, cl.Cid)
	//
	//	require.Equal(t, gsData.allSelector, requestReceived.selector)
	//})
	//
	//t.Run("with error validation", func(t *testing.T) {
	//	sv := newSV()
	//	sv.expectErrorPull()
	//
	//	dt2 := NewGraphSyncDataTransfer(ctx, host2, gs2)
	//	err := dt2.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv)
	//	require.NoError(t, err)
	//
	//	subscribeCalls := make(chan struct{}, 1)
	//	subscribe := func(event datatransfer.Event, channelState datatransfer.ChannelState) {
	//		if event == datatransfer.Error {
	//			subscribeCalls <- struct{}{}
	//		}
	//	}
	//	unsub := dt1.SubscribeToEvents(subscribe)
	//	_, err = dt1.OpenPullDataChannel(ctx, host2.ID(), &voucher, baseCid, gsData.allSelector)
	//	require.NoError(t, err)
	//
	//	select {
	//	case <-ctx.Done():
	//		t.Fatal("subscribed events not received")
	//	case <-subscribeCalls:
	//	}
	//
	//	sv.verifyExpectations(t)
	//
	//	// no graphsync request should be scheduled
	//	require.Empty(t, gs1.receivedRequests)
	//	unsub()
	//})
}

type receivedGraphSyncMessage struct {
	message gsmsg.GraphSyncMessage
	p       peer.ID
}

type fakeGraphSyncReceiver struct {
	receivedMessages chan receivedGraphSyncMessage
}

func (fgsr *fakeGraphSyncReceiver) ReceiveMessage(ctx context.Context, sender peer.ID, incoming gsmsg.GraphSyncMessage) {
	select {
	case <-ctx.Done():
	case fgsr.receivedMessages <- receivedGraphSyncMessage{incoming, sender}:
	}
}

func (fgsr *fakeGraphSyncReceiver) ReceiveError(_ error) {
}
func (fgsr *fakeGraphSyncReceiver) Connected(p peer.ID) {
}
func (fgsr *fakeGraphSyncReceiver) Disconnected(p peer.ID) {
}

// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/22
func TestRespondingToPushGraphsyncRequests(t *testing.T) {
	// create network
	//ctx := context.Background()
	//ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	//defer cancel()
	//gsData := newGraphsyncTestingData(t, ctx)
	//host1 := gsData.host1
	//host2 := gsData.host2
	//voucher := fakeDTType{"applesauce"}
	//baseBlock1 := blocks.NewBlock(testutil.RandomBytes(100))
	//err := gsData.bs1.Put(baseBlock1)
	//require.NoError(t, err)
	//
	//// setup receiving peer to just record message coming in
	//dtnet2 := network.NewFromLibp2pHost(host2)
	//r := &receiver{
	//	messageReceived: make(chan receivedMessage),
	//}
	//dtnet2.SetDelegate(r)
	//
	//gsr := &fakeGraphSyncReceiver{
	//	receivedMessages: make(chan receivedGraphSyncMessage),
	//}
	//gsData.gsNet2.SetDelegate(gsr)
	//
	//gs1 := gsData.setupGraphsyncHost1()
	//dt1 := NewGraphSyncDataTransfer(ctx, host1, gs1)
	//t.Run("when request is initiated", func(t *testing.T) {
	//	_, err = dt1.OpenPushDataChannel(ctx, host2.ID(), &voucher, baseBlock1.Cid(), gsData.allSelector)
	//	require.NoError(t, err)
	//	var messageReceived receivedMessage
	//	select {
	//	case <-ctx.Done():
	//		t.Fatal("did not receive message sent")
	//	case messageReceived = <-r.messageReceived:
	//	}
	//	requestReceived := messageReceived.message.(message.DataTransferRequest)
	//	var buf bytes.Buffer
	//	extStruct := &ExtensionDataTransferData{TransferID: uint64(requestReceived.TransferID())}
	//	err = extStruct.MarshalCBOR(&buf)
	//	require.NoError(t, err)
	//	extData := buf.Bytes()
	//	err := dagcbor.Encoder(gsData.allSelector, &buf)
	//	require.NoError(t, err)
	//	selectorBytes := buf.Bytes()
	//	request := gsmsg.NewRequest(graphsync.RequestID(rand.Int31()), baseBlock1.Cid(), selectorBytes, graphsync.Priority(rand.Int31()), graphsync.ExtensionData{
	//		Name: ExtensionDataTransfer,
	//		Data: extData,
	//	})
	//	gsmessage := gsmsg.New()
	//	gsmessage.AddRequest(request)
	//	gsData.gsNet2.SendMessage(ctx, host1.ID(), gsmessage)
	//	var gsMessageReceived receivedGraphSyncMessage
	//	select {
	//	case <-ctx.Done():
	//		t.Fatal("did not receive message sent")
	//	case gsMessageReceived = <-gsr.receivedMessages:
	//	}
	//	response := gsMessageReceived.message.Responses()[0]
	//	require.False(t, gsmsg.IsTerminalFailureCode(response.Status()))
	//})
	//
	//t.Run("when no request is initiated", func(t *testing.T) {
	//	var buf bytes.Buffer
	//	extStruct := &ExtensionDataTransferData{TransferID: rand.Uint64()}
	//	err = extStruct.MarshalCBOR(&buf)
	//	require.NoError(t, err)
	//	extData := buf.Bytes()
	//	err := dagcbor.Encoder(gsData.allSelector, &buf)
	//	require.NoError(t, err)
	//	selectorBytes := buf.Bytes()
	//	request := gsmsg.NewRequest(graphsync.RequestID(rand.Int31()), baseBlock1.Cid(), selectorBytes, graphsync.Priority(rand.Int31()), graphsync.ExtensionData{
	//		Name: ExtensionDataTransfer,
	//		Data: extData,
	//	})
	//	gsmessage := gsmsg.New()
	//	gsmessage.AddRequest(request)
	//	gsData.gsNet2.SendMessage(ctx, host1.ID(), gsmessage)
	//	var gsMessageReceived receivedGraphSyncMessage
	//	select {
	//	case <-ctx.Done():
	//		t.Fatal("did not receive message sent")
	//	case gsMessageReceived = <-gsr.receivedMessages:
	//	}
	//	response := gsMessageReceived.message.Responses()[0]
	//	require.True(t, gsmsg.IsTerminalFailureCode(response.Status()))
	//})
}

// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/23
func TestRespondingToPullGraphsyncRequests(t *testing.T) {
	// create network
	//ctx := context.Background()
	//ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	//defer cancel()
	//
	//gsData := newGraphsyncTestingData(t, ctx)
	//host1 := gsData.host1
	//host2 := gsData.host2
	//
	//// setup receiving peer to just record message coming in
	//dtnet1 := network.NewFromLibp2pHost(host1)
	//r := &receiver{
	//	messageReceived: make(chan receivedMessage),
	//}
	//dtnet1.SetDelegate(r)
	//
	//gsr := &fakeGraphSyncReceiver{
	//	receivedMessages: make(chan receivedGraphSyncMessage),
	//}
	//gsData.gsNet1.SetDelegate(gsr)
	//
	//gs2 := gsData.setupGraphsyncHost2()
	//
	//voucher := fakeDTType{"applesauce"}
	//baseBlock1 := blocks.NewBlock(testutil.RandomBytes(100))
	//err := gsData.bs2.Put(baseBlock1)
	//require.NoError(t, err)
	//id := datatransfer.TransferID(rand.Int31())
	//var buf bytes.Buffer
	//err = dagcbor.Encoder(gsData.allSelector, &buf)
	//require.NoError(t, err)
	//selectorBytes := buf.Bytes()
	//
	//t.Run("When request is initated and validated", func(t *testing.T) {
	//	sv := newSV()
	//	sv.expectSuccessPull()
	//
	//	dt := NewGraphSyncDataTransfer(ctx, host2, gs2)
	//	require.NoError(t, dt.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv))
	//
	//	isPull := true
	//	voucherBytes, err := voucher.ToBytes()
	//	require.NoError(t, err)
	//	request := message.NewRequest(id, isPull, voucher.Type(), voucherBytes, baseBlock1.Cid(), selectorBytes)
	//	require.NoError(t, dtnet1.SendMessage(ctx, host2.ID(), request))
	//	var messageReceived receivedMessage
	//	select {
	//	case <-ctx.Done():
	//		t.Fatal("did not receive message sent")
	//	case messageReceived = <-r.messageReceived:
	//	}
	//	sv.verifyExpectations(t)
	//	receivedResponse, ok := messageReceived.message.(message.DataTransferResponse)
	//	require.True(t, ok)
	//	require.True(t, receivedResponse.Accepted())
	//	extStruct := &ExtensionDataTransferData{TransferID: uint64(receivedResponse.TransferID())}
	//	err = extStruct.MarshalCBOR(&buf)
	//	require.NoError(t, err)
	//	extData := buf.Bytes()
	//	gsRequest := gsmsg.NewRequest(graphsync.RequestID(rand.Int31()), baseBlock1.Cid(), selectorBytes, graphsync.Priority(rand.Int31()), graphsync.ExtensionData{
	//		Name: ExtensionDataTransfer,
	//		Data: extData,
	//	})
	//	gsmessage := gsmsg.New()
	//	gsmessage.AddRequest(gsRequest)
	//	gsData.gsNet1.SendMessage(ctx, host1.ID(), gsmessage)
	//	var gsMessageReceived receivedGraphSyncMessage
	//	select {
	//	case <-ctx.Done():
	//		t.Fatal("did not receive message sent")
	//	case gsMessageReceived = <-gsr.receivedMessages:
	//	}
	//	response := gsMessageReceived.message.Responses()[0]
	//	require.False(t, gsmsg.IsTerminalFailureCode(response.Status()))
	//})
	//
	//// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/14
	//t.Run("When request is not initiated", func(t *testing.T) {
	//	_ = NewGraphSyncDataTransfer(ctx, host2, gs2)
	//	extStruct := &ExtensionDataTransferData{TransferID: rand.Uint64()}
	//	err = extStruct.MarshalCBOR(&buf)
	//	require.NoError(t, err)
	//	extData := buf.Bytes()
	//	request := gsmsg.NewRequest(graphsync.RequestID(rand.Int31()), baseBlock1.Cid(), selectorBytes, graphsync.Priority(rand.Int31()), graphsync.ExtensionData{
	//		Name: ExtensionDataTransfer,
	//		Data: extData,
	//	})
	//	gsmessage := gsmsg.New()
	//	gsmessage.AddRequest(request)
	//	gsData.gsNet1.SendMessage(ctx, host1.ID(), gsmessage)
	//	var gsMessageReceived receivedGraphSyncMessage
	//	select {
	//	case <-ctx.Done():
	//		t.Fatal("did not receive message sent")
	//	case gsMessageReceived = <-gsr.receivedMessages:
	//	}
	//	response := gsMessageReceived.message.Responses()[0]
	//	require.True(t, gsmsg.IsTerminalFailureCode(response.Status()))
	//})
}

// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/24
func TestDataTransferPushRoundTrip(t *testing.T) {
	//ctx := context.Background()
	//ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	//defer cancel()
	//
	//gsData := newGraphsyncTestingData(t, ctx)
	//host1 := gsData.host1
	//host2 := gsData.host2
	//
	//root := gsData.loadUnixFSFile(t, false)
	//rootCid := root.(cidlink.Link).Cid
	//gs1 := gsData.setupGraphsyncHost1()
	//gs2 := gsData.setupGraphsyncHost2()
	//
	//dt1 := NewGraphSyncDataTransfer(ctx, host1, gs1)
	//dt2 := NewGraphSyncDataTransfer(ctx, host2, gs2)
	//
	//finished := make(chan struct{}, 1)
	//var subscriber datatransfer.Subscriber = func(event datatransfer.Event, channelState datatransfer.ChannelState) {
	//	if event == datatransfer.Complete {
	//		finished <- struct{}{}
	//	}
	//}
	//dt2.SubscribeToEvents(subscriber)
	//voucher := fakeDTType{"applesauce"}
	//sv := newSV()
	//sv.expectSuccessPull()
	//require.NoError(t, dt2.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv))
	//
	//dt1.OpenPushDataChannel(ctx, host2.ID(), &voucher, rootCid, gsData.allSelector)
	//select {
	//case <-ctx.Done():
	//	t.Fatal("Did not complete succcessful data transfer")
	//case <-finished:
	//	gsData.verifyFileTransferred(t, root, true)
	//}
}

// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/24
func TestDataTransferPullRoundTrip(t *testing.T) {
	//ctx := context.Background()
	//ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	//defer cancel()
	//
	//gsData := newGraphsyncTestingData(t, ctx)
	//host1 := gsData.host1
	//host2 := gsData.host2
	//
	//root := gsData.loadUnixFSFile(t, false)
	//rootCid := root.(cidlink.Link).Cid
	//gs1 := gsData.setupGraphsyncHost1()
	//gs2 := gsData.setupGraphsyncHost2()
	//
	//dt1 := NewGraphSyncDataTransfer(ctx, host1, gs1)
	//dt2 := NewGraphSyncDataTransfer(ctx, host2, gs2)
	//
	//finished := make(chan struct{}, 1)
	//var subscriber datatransfer.Subscriber = func(event datatransfer.Event, channelState datatransfer.ChannelState) {
	//	if event == datatransfer.Complete {
	//		finished <- struct{}{}
	//	}
	//}
	//dt2.SubscribeToEvents(subscriber)
	//voucher := fakeDTType{"applesauce"}
	//sv := newSV()
	//sv.expectSuccessPull()
	//require.NoError(t, dt1.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), sv))
	//
	//dt2.OpenPullDataChannel(ctx, host1.ID(), &voucher, rootCid, gsData.allSelector)
	//select {
	//case <-ctx.Done():
	//	t.Fatal("Did not complete succcessful data transfer")
	//case <-finished:
	//	gsData.verifyFileTransferred(t, root, true)
	//}
}

const unixfsChunkSize uint64 = 1 << 10
const unixfsLinksPerLevel = 1024

type graphsyncTestingData struct {
	ctx         context.Context
	bs1         bstore.Blockstore
	bs2         bstore.Blockstore
	dagService1 ipldformat.DAGService
	dagService2 ipldformat.DAGService
	loader1     ipld.Loader
	loader2     ipld.Loader
	storer1     ipld.Storer
	storer2     ipld.Storer
	host1       host.Host
	host2       host.Host
	gsNet1      gsnet.GraphSyncNetwork
	gsNet2      gsnet.GraphSyncNetwork
	bridge1     ipldbridge.IPLDBridge
	bridge2     ipldbridge.IPLDBridge
	allSelector ipld.Node
	origBytes   []byte
}

func newGraphsyncTestingData(t *testing.T, ctx context.Context) *graphsyncTestingData {

	gsData := &graphsyncTestingData{}
	gsData.ctx = ctx
	makeLoader := func(bs bstore.Blockstore) ipld.Loader {
		return func(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error) {
			c, ok := lnk.(cidlink.Link)
			if !ok {
				return nil, errors.New("Incorrect Link Type")
			}
			// read block from one store
			block, err := bs.Get(c.Cid)
			if err != nil {
				return nil, err
			}
			return bytes.NewReader(block.RawData()), nil
		}
	}

	makeStorer := func(bs bstore.Blockstore) ipld.Storer {
		return func(lnkCtx ipld.LinkContext) (io.Writer, ipld.StoreCommitter, error) {
			var buf bytes.Buffer
			var committer ipld.StoreCommitter = func(lnk ipld.Link) error {
				c, ok := lnk.(cidlink.Link)
				if !ok {
					return errors.New("Incorrect Link Type")
				}
				block, err := blocks.NewBlockWithCid(buf.Bytes(), c.Cid)
				if err != nil {
					return err
				}
				return bs.Put(block)
			}
			return &buf, committer, nil
		}
	}
	// make a blockstore and dag service
	gsData.bs1 = bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
	gsData.bs2 = bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))

	gsData.dagService1 = merkledag.NewDAGService(blockservice.New(gsData.bs1, offline.Exchange(gsData.bs1)))
	gsData.dagService2 = merkledag.NewDAGService(blockservice.New(gsData.bs2, offline.Exchange(gsData.bs2)))

	// setup an IPLD loader/storer for blockstore 1
	gsData.loader1 = makeLoader(gsData.bs1)
	gsData.storer1 = makeStorer(gsData.bs1)

	// setup an IPLD loader/storer for blockstore 2
	gsData.loader2 = makeLoader(gsData.bs2)
	gsData.storer2 = makeStorer(gsData.bs2)

	mn := mocknet.New(ctx)

	// setup network
	var err error
	gsData.host1, err = mn.GenPeer()
	require.NoError(t, err)

	gsData.host2, err = mn.GenPeer()
	require.NoError(t, err)

	err = mn.LinkAll()
	require.NoError(t, err)

	gsData.gsNet1 = gsnet.NewFromLibp2pHost(gsData.host1)
	gsData.gsNet2 = gsnet.NewFromLibp2pHost(gsData.host2)

	gsData.bridge1 = ipldbridge.NewIPLDBridge()
	gsData.bridge2 = ipldbridge.NewIPLDBridge()

	// create a selector for the whole UnixFS dag
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	gsData.allSelector = ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	return gsData
}

func (gsData *graphsyncTestingData) setupGraphsyncHost1() graphsync.GraphExchange {
	// setup graphsync
	return gsimpl.New(gsData.ctx, gsData.gsNet1, gsData.bridge1, gsData.loader1, gsData.storer1)
}

func (gsData *graphsyncTestingData) setupGraphsyncHost2() graphsync.GraphExchange {
	// setup graphsync
	return gsimpl.New(gsData.ctx, gsData.gsNet2, gsData.bridge2, gsData.loader2, gsData.storer2)
}

func (gsData *graphsyncTestingData) loadUnixFSFile(t *testing.T, useSecondNode bool) ipld.Link {

	// read in a fixture file
	path, err := filepath.Abs(filepath.Join("fixtures", "lorem.txt"))
	require.NoError(t, err)

	f, err := os.Open(path)
	require.NoError(t, err)

	var buf bytes.Buffer
	tr := io.TeeReader(f, &buf)
	file := files.NewReaderFile(tr)

	// import to UnixFS
	var dagService ipldformat.DAGService
	if useSecondNode {
		dagService = gsData.dagService2
	} else {
		dagService = gsData.dagService1
	}
	bufferedDS := ipldformat.NewBufferedDAG(gsData.ctx, dagService)

	params := ihelper.DagBuilderParams{
		Maxlinks:   unixfsLinksPerLevel,
		RawLeaves:  true,
		CidBuilder: nil,
		Dagserv:    bufferedDS,
	}

	db, err := params.New(chunker.NewSizeSplitter(file, int64(unixfsChunkSize)))
	require.NoError(t, err)

	nd, err := balanced.Layout(db)
	require.NoError(t, err)

	err = bufferedDS.Commit()
	require.NoError(t, err)

	// save the original files bytes
	gsData.origBytes = buf.Bytes()

	return cidlink.Link{Cid: nd.Cid()}
}

func (gsData *graphsyncTestingData) verifyFileTransferred(t *testing.T, link ipld.Link, useSecondNode bool) {
	var dagService ipldformat.DAGService
	if useSecondNode {
		dagService = gsData.dagService2
	} else {
		dagService = gsData.dagService1
	}

	c := link.(cidlink.Link).Cid

	// load the root of the UnixFS DAG from the new blockstore
	otherNode, err := dagService.Get(gsData.ctx, c)
	require.NoError(t, err)

	// Setup a UnixFS file reader
	n, err := unixfile.NewUnixfsFile(gsData.ctx, dagService, otherNode)
	require.NoError(t, err)

	fn, ok := n.(files.File)
	require.True(t, ok)

	// Read the bytes for the UnixFS File
	finalBytes, err := ioutil.ReadAll(fn)
	require.NoError(t, err)

	// verify original bytes match final bytes!
	require.EqualValues(t, gsData.origBytes, finalBytes)
}

type receivedGraphSyncRequest struct {
	p          peer.ID
	root       ipld.Link
	selector   ipld.Node
	extensions []graphsync.ExtensionData
}

type fakeGraphSync struct {
	receivedRequests chan receivedGraphSyncRequest
}

// Request initiates a new GraphSync request to the given peer using the given selector spec.
func (fgs *fakeGraphSync) Request(ctx context.Context, p peer.ID, root ipld.Link, selector ipld.Node, extensions ...graphsync.ExtensionData) (<-chan graphsync.ResponseProgress, <-chan error) {
	fgs.receivedRequests <- receivedGraphSyncRequest{p, root, selector, extensions}
	responses := make(chan graphsync.ResponseProgress)
	errors := make(chan error)
	close(responses)
	close(errors)
	return responses, errors
}

// RegisterExtension adds a user supplied extension with the given extension config
func (fgs *fakeGraphSync) RegisterExtension(config graphsync.ExtensionConfig) error {
	return nil
}
