package message

import (
	"io"

	"github.com/filecoin-project/lotus/datatransfer"
)

type transferMessage struct {
	IsRq bool

	Request  *transferRequest
	Response *transferResponse
}

// ========= DataTransferMessage interface

// IsRequest returns true if this message is a data request
func (tm *transferMessage) IsRequest() bool {
	return tm.IsRq
}

// TransferID returns the TransferID of this message
func (tm *transferMessage) TransferID() datatransfer.TransferID {
	if tm.IsRequest() {
		return tm.Request.TransferID()
	}
	return tm.Response.TransferID()
}

// ToNet serializes a transfer message type. It is simply a wrapper for MarshalCBOR, to provide
// symmetry with FromNet
func (tm *transferMessage) ToNet(w io.Writer) error {
	return tm.MarshalCBOR(w)
}
