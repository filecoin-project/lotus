package network

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/filecoin-project/lotus/datatransfer/message"
)

var (
	// ProtocolDataTransfer is the protocol identifier for graphsync messages
	ProtocolDataTransfer protocol.ID = "/fil/datatransfer/1.0.0"
)

// DataTransferNetwork provides network connectivity for GraphSync.
type DataTransferNetwork interface {

	// SendMessage sends a GraphSync message to a peer.
	SendMessage(
		context.Context,
		peer.ID,
		message.DataTransferMessage) error

	// SetDelegate registers the Reciver to handle messages received from the
	// network.
	SetDelegate(Receiver)

	// ConnectTo establishes a connection to the given peer
	ConnectTo(context.Context, peer.ID) error

	NewMessageSender(context.Context, peer.ID) (MessageSender, error)
}

// MessageSender is an interface to send messages to a peer
type MessageSender interface {
	SendMsg(context.Context, message.DataTransferMessage) error
	Close() error
	Reset() error
}

// Receiver is an interface for receiving messages from the GraphSyncNetwork.
type Receiver interface {
	ReceiveRequest(
		ctx context.Context,
		sender peer.ID,
		incoming message.DataTransferRequest)

	ReceiveResponse(
		ctx context.Context,
		sender peer.ID,
		incoming message.DataTransferResponse)

	ReceiveError(error)
}
