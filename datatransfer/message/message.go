package message

import (
	"io"

	ggio "github.com/gogo/protobuf/io"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/network"
	cborgen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/datatransfer"
	pb "github.com/filecoin-project/lotus/datatransfer/message/pb"
)

// Reference file: https://github.com/ipfs/go-graphsync/blob/master/message/message.go
// though here we have a simpler message type that serializes/deserializes to two
// different types that share an interface, and we serialize to CBOR and not Protobuf.

// DataTransferMessage is a message for the data transfer protocol
// (either request or response) that can serialize to a protobuf
type DataTransferMessage interface {
	IsRequest() bool
	TransferID() datatransfer.TransferID
	cborgen.CBORMarshaler
	cborgen.CBORUnmarshaler
}

// DataTransferRequest is a response message for the data transfer protocol
type DataTransferRequest interface {
	DataTransferMessage
	IsPull() bool
	VoucherType() string
	Voucher() []byte
	BaseCid() cid.Cid
	Selector() []byte
	IsCancel() bool
}

// DataTransferResponse is a response message for the data transfer protocol
type DataTransferResponse interface {
	DataTransferMessage
	Accepted() bool
}

// NewRequest generates a new request for the data transfer protocol
// TODO: Write this method, and an implementation of the data transfer request interface
func NewRequest(id datatransfer.TransferID, isPull bool, voucherIdentifier string, voucher []byte, baseCid cid.Cid, selector []byte) DataTransferRequest {
	return &transferRequest{
		XferID: uint64(id),
		Pull:   isPull,
		Vouch:  voucher,
		Stor:   selector,
		BCid:   baseCid.String(),
		VTyp:   voucherIdentifier,
	}
}

// CancelRequest request generates a request to cancel an in progress request
// TODO: Write this method, and an implementation of the data transfer request interface
func CancelRequest(id datatransfer.TransferID) DataTransferRequest {
	return &transferRequest{
		XferID: uint64(id),
		Canc:   true,
	}
}

// NewResponse builds a new Data Transfer response
// TODO: Write this method, and an implementation of the data transfer response interface
func NewResponse(id datatransfer.TransferID, accepted bool) DataTransferResponse {
	return &transferResponse{
		Acpt:   accepted,
		XferID: uint64(id),
	}
}

// NewMessageFromProto generates a new DataTransferMessage
// which is either an underlying DataTransferRequest or DataTransferResponse
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
	// TODO: Needs generate actual PB Message struct to fulfill interface
	//if err := pbr.ReadMsg(pb); err != nil {
	//	return nil, err
	//}

	return nil, nil
}
