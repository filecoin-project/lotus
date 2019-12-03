package graphsyncimpl_test

import (
	"bytes"
	"context"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/ipld/go-ipld-prime/encoding/dagcbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/datatransfer"
	. "github.com/filecoin-project/lotus/datatransfer/impl/graphsync"
	"github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/network"
	"github.com/filecoin-project/lotus/datatransfer/testutil"
)

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
		requests: make(chan receivedGraphSyncRequest, 1),
	}

	voucher := fakeDTType{"applesauce"}
	baseCid := testutil.GenerateCids(1)[0]
	var buffer bytes.Buffer
	err := dagcbor.Encoder(gsData.allSelector, &buffer)
	require.NoError(t, err)

	t.Run("Response to push with successful validation", func(t *testing.T) {
		id := datatransfer.TransferID(rand.Int31())
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
		id := datatransfer.TransferID(rand.Int31())
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
		id := datatransfer.TransferID(rand.Int31())
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
		id := datatransfer.TransferID(rand.Int31())
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
