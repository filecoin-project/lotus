package message

import (
	"io"

	"github.com/filecoin-project/lotus/datatransfer"
)

// transferResponse is a private struct that satisfies the DataTransferResponse interface
type transferResponse struct {
	Acpt   bool
	XferID uint64
}

func (trsp *transferResponse) TransferID() datatransfer.TransferID {
	return datatransfer.TransferID(trsp.XferID)
}

// IsRequest always returns false in this case because this is a transfer response
func (trsp *transferResponse) IsRequest() bool {
	return false
}

// 	Accepted returns true if the request is accepted in the response
func (trsp *transferResponse) Accepted() bool {
	return trsp.Acpt
}

// ToNet serializes a transfer response. It's a wrapper for MarshalCBOR to provide
// symmetry with FromNet
func (trsp *transferResponse) ToNet(w io.Writer) error {
	msg := transferMessage{
		IsRq:     false,
		Request:  nil,
		Response: trsp,
	}
	return msg.MarshalCBOR(w)
}
