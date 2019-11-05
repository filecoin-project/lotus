package message

import (
	"io"

	"github.com/ipfs/go-cid"
	cborgen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/lotus/datatransfer"
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
	ToNet(w io.Writer) error
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
func NewRequest(id datatransfer.TransferID, isPull bool, voucherIdentifier string, voucher []byte, baseCid cid.Cid, selector []byte) DataTransferRequest {
	return &transferRequest{
		Pull:   isPull,
		Vouch:  voucher,
		Stor:   selector,
		BCid:   baseCid.String(),
		VTyp:   voucherIdentifier,
		XferID: uint64(id),
	}
}

// CancelRequest request generates a request to cancel an in progress request
func CancelRequest(id datatransfer.TransferID) DataTransferRequest {
	return &transferRequest{
		Canc:   true,
		XferID: uint64(id),
	}
}

// NewResponse builds a new Data Transfer response
func NewResponse(id datatransfer.TransferID, accepted bool) DataTransferResponse {
	return &transferResponse{Acpt: accepted, XferID: uint64(id)}
}

// FromNet can read a network stream to deserialize a GraphSyncMessage
func FromNet(r io.Reader) (DataTransferMessage, error) {
	tresp := transferMessage{}
	err := tresp.UnmarshalCBOR(r)
	if tresp.IsRequest() {
		return tresp.Request, nil
	}
	return tresp.Response, err
}
