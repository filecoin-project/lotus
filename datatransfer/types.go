package datatransfer

import (
	"reflect"

	"github.com/ipfs/go-cid"
	selector "github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Voucher is used to validate
// a data transfer request against the underlying storage or retrieval deal
// that precipitated it. The only requirement is a voucher can read and write
// from bytes
type Voucher interface {
	ToBytes() []byte
	FromBytes([]byte) (Voucher, error)
	Identifier() string
}

// Status is the status of transfer for a given channel
type Status string

const (
	// Ongoing means the data transfer is in progress
	Ongoing = Status("Ongoing")

	// Completed means the data transfer is completed successfully
	Completed = Status("Completed")

	// Failed means the data transfer failed
	Failed = Status("Failed")

	// ChannelNotFoundError means the searched for data transfer does not exist
	ChannelNotFoundError = Status("ChannelNotFoundError")
)

// TransferID is an identifier for a data transfer, shared between
// request/responder and unique to the requestor
type TransferID uint64

// ChannelID is a unique identifier for a channel, distinct by both the other
// party's peer ID + the transfer ID
type ChannelID struct {
	to peer.ID
	id TransferID
}

// Channel represents all the parameters for a single data transfer
type Channel struct {
	// an identifier for this channel shared by request and responder, set by requestor through protocol
	transferID TransferID
	// base CID for the piece being transferred
	PieceRef cid.Cid
	// portion of Piece to return, spescified by an IPLD selector
	Selector selector.Selector
	// used to verify this channel
	voucher Voucher
	// the party that is sending the data (not who initiated the request)
	sender peer.ID
	// the party that is receiving the data (not who initiated the request)
	recipient peer.ID
	// expected amount of data to be transferred
	totalSize uint64
}

// ChannelState is immutable channel data plus mutable state
type ChannelState struct {
	Channel
	// total bytes sent from this node (0 if receiver)
	sent uint64
	// total bytes received by this node (0 if sender)
	received uint64
}

// Event is a name for an event that occurs on a data transfer channel
type Event string

const (
	// Open is an event occurs when a channel is first opened
	Open = Event("Open")

	// Progress is an event that gets emitted every time more data is transferred
	Progress = Event("Progress")

	// Error is an event that emits when an error occurs in a data transfer
	Error = Event("Error")

	// Complete is emitted when a data transfer is complete
	Complete = Event("Complete")
)

// Subscriber is a callback that is called when events are emitted
type Subscriber func(event Event, channelState ChannelState)

// RequestValidator is an interface implemented by the client of the
// data transfer module to validate requests
type RequestValidator interface {
	ValidatePush(
		sender peer.ID,
		voucher Voucher,
		PieceRef cid.Cid,
		Selector selector.Selector) error
	ValidatePull(
		receiver peer.ID,
		voucher Voucher,
		PieceRef cid.Cid,
		Selector selector.Selector) error
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
	OpenPushDataChannel(to peer.ID, voucher Voucher, PieceRef cid.Cid, Selector selector.Selector) ChannelID

	// open a data transfer that will request data from the sending peer and
	// transfer parts of the piece that match the selector
	OpenPullDataChannel(to peer.ID, voucher Voucher, PieceRef cid.Cid, Selector selector.Selector) ChannelID

	// close an open channel (effectively a cancel)
	CloseDataTransferChannel(x ChannelID)

	// get status of a transfer
	TransferChannelStatus(x ChannelID) Status

	// get notified when certain types of events happen
	SubscribeToEvents(subscriber Subscriber)

	// get all in progress transfers
	InProgressChannels() map[ChannelID]ChannelState
}
