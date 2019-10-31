package message

import (
	"io"

	ggio "github.com/gogo/protobuf/io"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/network"

	"github.com/filecoin-project/lotus/datatransfer"
	pb "github.com/filecoin-project/lotus/datatransfer/message/pb"
)

// Reference file: https://github.com/ipfs/go-graphsync/blob/master/message/message.go
// though here we have a simpler message type that serializes/deserializes to two
// different types that share an interface

// DataTransferMessage is a message for the data transfer protocol
// (either request or response) that can serialize to a protobuf
type DataTransferMessage interface {
	IsRequest() bool
	TransferID() datatransfer.TransferID
	Exportable
	Loggable
}

// TransferRequest is a response message for the data transfer protocol
type DataTransferRequest interface {
	DataTransferMessage
	IsPull() bool
	VoucherIdentifier() string
	Voucher() []byte
	BaseCid() string
	Selector() []byte
	IsCancel() bool
}

// DataTransferResponse is a response message for the data transfer protocol
type DataTransferResponse interface {
	DataTransferMessage
	Accepted() bool
}

// Exportable is an interface that can serialize to a protobuf
type Exportable interface {
	ToProto() *pb.Message
	ToNet(w io.Writer) error
}

// Loggable means the message supports the loggable interface
type Loggable interface {
	Loggable() map[string]interface{}
}

// NewRequest generates a new request for the data transfer protocol
// TODO: Write this method, and an implementation of the data transfer request interface
func NewRequest(id datatransfer.TransferID, isPull bool, voucherIdentifier string, voucher []byte, baseCid cid.Cid, selector []byte) DataTransferRequest {
	return NewTransferRequest(id,isPull,voucherIdentifier,voucher,baseCid,selector)
}

// CancelRequest request generates a request to cancel an in progress request
// TODO: Write this method, and an implementation of the data transfer request interface
func CancelRequest(id datatransfer.TransferID) DataTransferRequest {
	return nil
}

// NewResponse builds a new Data Transfer response
// TODO: Write this method, and an implementation of the data transfer response interface
func NewResponse(id datatransfer.TransferID, accepted bool) DataTransferResponse {
	return nil
}

// NewMessageFromProto generates a new DataTransferMessage
// which is either an underlying TransferRequest or DataTransferResponse
// type, from a protobuf
// TODO: Write this method -- that can de serialize a protobuf message to
// either a data transfer request or a data transfer response
func NewMessageFromProto(pbm pb.Message) (DataTransferMessage, error) {
	return nil, nil
}

// FromNet can read a network stream to deserialized a GraphSyncMessage
func FromNet(r io.Reader) (DataTransferMessage, error) {
	pbr := ggio.NewDelimitedReader(r, network.MessageSizeMax)
	return FromPBReader(pbr)
}

// FromPBReader can deserialize a protobuf message into a GraphySyncMessage.
func FromPBReader(pbr ggio.Reader) (DataTransferMessage, error) {
	pb := new(pb.Message)
	// TODO: Needs generate actual PB Message struct to fulfill interface
	//if err := pbr.ReadMsg(pb); err != nil {
	//	return nil, err
	//}

	return NewMessageFromProto(*pb)
}