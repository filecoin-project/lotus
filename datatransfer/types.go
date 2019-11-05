package datatransfer

import (
	"context"
	"reflect"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Voucher is used to validate
// a data transfer request against the underlying storage or retrieval deal
// that precipitated it. The only requirement is a voucher can read and write
// from bytes, and has a string identifier type
type Voucher interface {
	// ToBytes converts the Voucher to raw bytes
	ToBytes() ([]byte, error)
	// FromBytes reads a Voucher from raw bytes
	FromBytes([]byte) error
	// Identifier is a unique string identifier for this voucher type
	Identifier() string
}

// Status is the status of transfer for a given channel
type Status int

const (
	// Ongoing means the data transfer is in progress
	Ongoing Status = iota

	// Completed means the data transfer is completed successfully
	Completed

	// Failed means the data transfer failed
	Failed

	// ChannelNotFoundError means the searched for data transfer does not exist
	ChannelNotFoundError
)

// TransferID is an identifier for a data transfer, shared between
// request/responder and unique to the requestor
type TransferID uint64

// ChannelID is a unique identifier for a channel, distinct by both the other
// party's peer ID + the transfer ID
type ChannelID struct {
	To peer.ID
	ID TransferID
}

// Channel represents all the parameters for a single data transfer
type Channel struct {
	// an identifier for this channel shared by request and responder, set by requestor through protocol
	transferID TransferID
	// base CID for the piece being transferred
	baseCid cid.Cid
	// portion of Piece to return, spescified by an IPLD selector
	selector ipld.Node
	// used to verify this channel
	voucher Voucher
	// the party that is sending the data (not who initiated the request)
	sender peer.ID
	// the party that is receiving the data (not who initiated the request)
	recipient peer.ID
	// expected amount of data to be transferred
	totalSize uint64
}

// NewChannel makes a new channel
func NewChannel(transferID TransferID, baseCid cid.Cid,
	selector ipld.Node,
	voucher Voucher,
	sender peer.ID,
	recipient peer.ID,
	totalSize uint64) Channel {
	return Channel{transferID, baseCid, selector, voucher, sender, recipient, totalSize}
}

// TransferID returns the transfer id for this channel
func (c Channel) TransferID() TransferID { return c.transferID }

// BaseCID returns the CID that is at the root of this data transfer
func (c Channel) BaseCID() cid.Cid { return c.baseCid }

// Selector returns the IPLD selector for this data transfer (represented as
// an IPLD node)
func (c Channel) Selector() ipld.Node { return c.selector }

// Voucher returns the voucher for this data transfer
func (c Channel) Voucher() Voucher { return c.voucher }

// Sender returns the peer id for the node that is sending data
func (c Channel) Sender() peer.ID { return c.sender }

// Recipient returns the peer id for the node that is receiving data
func (c Channel) Recipient() peer.ID { return c.recipient }

// TotalSize returns the total size for the data being transferred
func (c Channel) TotalSize() uint64 { return c.totalSize }

// ChannelState is immutable channel data plus mutable state
type ChannelState struct {
	Channel
	// total bytes sent from this node (0 if receiver)
	sent uint64
	// total bytes received by this node (0 if sender)
	received uint64
}

// Sent returns the number of bytes sent
func (c ChannelState) Sent() uint64 { return c.sent }

// Received returns the number of bytes received
func (c ChannelState) Received() uint64 { return c.received }

// Event is a name for an event that occurs on a data transfer channel
type Event int

const (
	// Open is an event occurs when a channel is first opened
	Open Event = iota

	// Progress is an event that gets emitted every time more data is transferred
	Progress

	// Error is an event that emits when an error occurs in a data transfer
	Error

	// Complete is emitted when a data transfer is complete
	Complete
)

// Subscriber is a callback that is called when events are emitted
type Subscriber func(event Event, channelState ChannelState)

// Unsubscribe is a function that gets called to unsubscribe from data transfer events
type Unsubscribe func()

// RequestValidator is an interface implemented by the client of the
// data transfer module to validate requests
type RequestValidator interface {
	// ValidatePush validates a push request received from the peer that will send data
	ValidatePush(
		sender peer.ID,
		voucher Voucher,
		baseCid cid.Cid,
		selector ipld.Node) error
	// ValidatePull validates a pull request received from the peer that will receive data
	ValidatePull(
		receiver peer.ID,
		voucher Voucher,
		baseCid cid.Cid,
		selector ipld.Node) error
}

// Manager is the core interface presented by all implementations of
// of the data transfer sub system
type Manager interface {
	// RegisterVoucherType registers a validator for the given voucher type
	// will error if voucher type does not implement voucher
	// or if there is a voucher type registered with an identical identifier
	RegisterVoucherType(voucherType reflect.Type, validator RequestValidator) error

	// open a data transfer that will send data to the recipient peer and
	// open a data transfer that will send data to the recipient peer and
	// transfer parts of the piece that match the selector
	OpenPushDataChannel(ctx context.Context, to peer.ID, voucher Voucher, baseCid cid.Cid, selector ipld.Node) (ChannelID, error)

	// open a data transfer that will request data from the sending peer and
	// transfer parts of the piece that match the selector
	OpenPullDataChannel(ctx context.Context, to peer.ID, voucher Voucher, baseCid cid.Cid, selector ipld.Node) (ChannelID, error)

	// close an open channel (effectively a cancel)
	CloseDataTransferChannel(x ChannelID)

	// get status of a transfer
	TransferChannelStatus(x ChannelID) Status

	// get notified when certain types of events happen
	SubscribeToEvents(subscriber Subscriber) Unsubscribe

	// get all in progress transfers
	InProgressChannels() map[ChannelID]ChannelState
}
