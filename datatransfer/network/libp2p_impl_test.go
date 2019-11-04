package network_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/datatransfer/message"
	"github.com/filecoin-project/lotus/datatransfer/network"
	"github.com/filecoin-project/lotus/datatransfer/testutil"
)

// Receiver is an interface for receiving messages from the DataTransferNetwork.
type receiver struct {
	messageReceived chan struct{}
	lastRequest     message.DataTransferRequest
	lastResponse    message.DataTransferResponse
	lastSender      peer.ID
	connectedPeers  chan peer.ID
}

func (r *receiver) ReceiveRequest(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferRequest) {
	r.lastSender = sender
	r.lastRequest = incoming
	select {
	case <-ctx.Done():
	case r.messageReceived <- struct{}{}:
	}
}

func (r *receiver) ReceiveResponse(
	ctx context.Context,
	sender peer.ID,
	incoming message.DataTransferResponse) {
	r.lastSender = sender
	r.lastResponse = incoming
	select {
	case <-ctx.Done():
	case r.messageReceived <- struct{}{}:
	}
}

func (r *receiver) ReceiveError(err error) {
}

// TODO: get passing to complete
// https://github.com/filecoin-project/go-data-transfer/issues/37
// should pass as soon as message is implemented
func TestMessageSendAndReceive(t *testing.T) {
	// create network
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	mn := mocknet.New(ctx)

	host1, err := mn.GenPeer()
	require.NoError(t, err)
	host2, err := mn.GenPeer()
	require.NoError(t, err)
	err = mn.LinkAll()
	require.NoError(t, err)

	dtnet1 := network.NewFromLibp2pHost(host1)
	dtnet2 := network.NewFromLibp2pHost(host2)
	r := &receiver{
		messageReceived: make(chan struct{}),
		connectedPeers:  make(chan peer.ID, 2),
	}
	dtnet1.SetDelegate(r)
	dtnet2.SetDelegate(r)

	err = dtnet1.ConnectTo(ctx, host2.ID())
	require.NoError(t, err)

	t.Run("Send Request", func(t *testing.T) {
		baseCid := testutil.GenerateCids(1)[0]
		selector := testutil.RandomBytes(100)
		isPull := false
		id := datatransfer.TransferID(rand.Int31())
		voucherIdentifier := "FakeVoucherType"
		voucher := testutil.RandomBytes(100)
		request := message.NewRequest(id, isPull, voucherIdentifier, voucher, baseCid, selector)

		go func() {
			require.NoError(t, dtnet1.SendMessage(ctx, host2.ID(), request))
		}()

		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case <-r.messageReceived:
		}

		sender := r.lastSender
		require.Equal(t, sender, host1.ID())

		receivedRequest := r.lastRequest
		require.NotNil(t, receivedRequest)

		assert.Equal(t, request.TransferID(), receivedRequest.TransferID())
		assert.Equal(t, request.IsCancel(), receivedRequest.IsCancel())
		assert.Equal(t, request.IsPull(), receivedRequest.IsPull())
		assert.Equal(t, request.IsRequest(), receivedRequest.IsRequest())
		assert.True(t, receivedRequest.BaseCid().Equals(request.BaseCid()))
		assert.Equal(t, request.VoucherType(), receivedRequest.VoucherType())
		assert.Equal(t, request.Voucher(), receivedRequest.Voucher())
		assert.Equal(t, request.Selector(), receivedRequest.Selector())
	})

	t.Run("Send Response", func(t *testing.T) {
		accepted := false
		id := datatransfer.TransferID(rand.Int31())
		response := message.NewResponse(id, accepted)

		go func() {
			require.NoError(t, dtnet2.SendMessage(ctx, host1.ID(), response))
		}()

		select {
		case <-ctx.Done():
			t.Fatal("did not receive message sent")
		case <-r.messageReceived:
		}

		sender := r.lastSender
		require.NotNil(t, sender)
		assert.Equal(t, sender, host2.ID())

		receivedResponse := r.lastResponse

		assert.Equal(t, response.TransferID(), receivedResponse.TransferID())
		assert.Equal(t, response.Accepted(), receivedResponse.Accepted())
		assert.Equal(t, response.IsRequest(), receivedResponse.IsRequest())

	})
}
