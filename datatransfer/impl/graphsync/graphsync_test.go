package graphsync_test

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/datatransfer"
	dtgraphsync "github.com/filecoin-project/lotus/datatransfer/impl/graphsync"
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

func (r *receiver) Connected(p peer.ID) {
}

func (r *receiver) Disconnected(p peer.ID) {
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
	mn := mocknet.New(ctx)

	// setup network
	host1, err := mn.GenPeer()
	require.NoError(t, err)
	host2, err := mn.GenPeer()
	require.NoError(t, err)
	err = mn.LinkAll()
	require.NoError(t, err)

	// setup receiving peer to just record message coming in
	dtnet2 := network.NewFromLibp2pHost(host2)
	r := &receiver{
		messageReceived: make(chan receivedMessage),
	}
	dtnet2.SetDelegate(r)

	bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))

	dt := dtgraphsync.NewGraphSyncDataTransfer(ctx, host1, bs)

	t.Run("OpenPushDataTransfer", func(t *testing.T) {
		ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

		// this is the selector for "get the whole DAG"
		// TODO: support storage deals with custom payload selectors
		stor := ssb.ExploreRecursive(selector.RecursionLimitNone(),
			ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

		voucher := fakeDTType{"applesauce"}
		baseCid := testutil.GenerateCids(1)[0]
		channelID, err := dt.OpenPushDataChannel(context.Background(), host2.ID(), &voucher, baseCid, stor)
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
		channelID, err := dt.OpenPullDataChannel(context.Background(), host2.ID(), &voucher, baseCid, stor)
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
	mn := mocknet.New(ctx)

	// setup network
	host1, err := mn.GenPeer()
	require.NoError(t, err)
	host2, err := mn.GenPeer()
	require.NoError(t, err)
	err = mn.LinkAll()
	require.NoError(t, err)

	// setup receiving peer to just record message coming in
	bs1 := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
	bs2 := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))

	dt1 := dtgraphsync.NewGraphSyncDataTransfer(ctx, host1, bs1)
	dt2 := dtgraphsync.NewGraphSyncDataTransfer(ctx, host2, bs2)

	fv := &fakeValidator{ctx, make(chan receivedValidation)}

	err = dt2.RegisterVoucherType(reflect.TypeOf(&fakeDTType{}), fv)
	require.NoError(t, err)

	// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/15
	t.Run("ValidatePush", func(t *testing.T) {
		ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

		// this is the selector for "get the whole DAG"
		// TODO: support storage deals with custom payload selectors
		stor := ssb.ExploreRecursive(selector.RecursionLimitNone(),
			ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

		voucher := fakeDTType{"applesauce"}
		baseCid := testutil.GenerateCids(1)[0]
		channelID, err := dt1.OpenPushDataChannel(context.Background(), host2.ID(), &voucher, baseCid, stor)
		require.NoError(t, err)

		assert.Equal(t, channelID.To, host2.ID())
		assert.NoError(t, err)

		var validation receivedValidation
		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case validation = <-fv.validationsReceived:
		}

		assert.False(t, validation.isPull)
		assert.Equal(t, validation.other, host1.ID())
		assert.Equal(t, validation.voucher, voucher)
		assert.Equal(t, validation.baseCid, baseCid)
		assert.Equal(t, validation.selector, stor)
	})

	// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/18
	t.Run("ValidatePull", func(t *testing.T) {
		ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

		// this is the selector for "get the whole DAG"
		// TODO: support storage deals with custom payload selectors
		stor := ssb.ExploreRecursive(selector.RecursionLimitNone(),
			ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

		voucher := fakeDTType{"applesauce"}
		baseCid := testutil.GenerateCids(1)[0]
		channelID, err := dt1.OpenPullDataChannel(context.Background(), host2.ID(), &voucher, baseCid, stor)
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
		assert.Equal(t, validation.voucher, voucher)
		assert.Equal(t, validation.baseCid, baseCid)
		assert.Equal(t, validation.selector, stor)
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
	mn := mocknet.New(ctx)

	// setup network
	host1, err := mn.GenPeer()
	require.NoError(t, err)
	host2, err := mn.GenPeer()
	require.NoError(t, err)
	err = mn.LinkAll()
	require.NoError(t, err)

	// setup receiving peer to just record message coming in
	dtnet1 := network.NewFromLibp2pHost(host1)
	r := &receiver{
		messageReceived: make(chan receivedMessage),
	}
	dtnet1.SetDelegate(r)

	bs := bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))

	dt := dtgraphsync.NewGraphSyncDataTransfer(ctx, host2, bs)
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	stor := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()
	voucher := fakeDTType{"applesauce"}
	baseCid := testutil.GenerateCids(1)[0]
	id := datatransfer.TransferID(rand.Int31())
	var buffer bytes.Buffer
	err = dagcbor.Encoder(stor, &buffer)
	require.NoError(t, err)

	// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/14
	t.Run("Response to push with successful validation", func(t *testing.T) {

		sv := newSV()
		sv.expectSuccessPush()
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

		assert.Equal(t, receivedResponse.TransferID(), id)
		require.True(t, receivedResponse.Accepted())

	})

	// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/14
	t.Run("Response to push with error validation", func(t *testing.T) {

		sv := newSV()
		sv.expectErrorPush()
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

	// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/17
	t.Run("Response to pull with successful validation", func(t *testing.T) {

		sv := newSV()
		sv.expectSuccessPull()
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

	// TODO: get passing to complete https://github.com/filecoin-project/go-data-transfer/issues/17
	t.Run("Response to push with error validation", func(t *testing.T) {

		sv := newSV()
		sv.expectErrorPull()

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
