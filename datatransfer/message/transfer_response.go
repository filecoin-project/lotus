package message

import (
	"github.com/filecoin-project/lotus/datatransfer"
)

// transferResponse is a private struct that satisfies the DataTransferResponse interface
type transferResponse struct {
	Acpt bool
	XferID uint64
}

// IsRequest always returns false since this is a data transfer response
func (trsp *transferResponse) IsRequest() bool {
	return false
}

// IsResponse always returns true since this is a data transfer response
func (trsp *transferResponse) IsResponse() bool {
	return !trsp.IsRequest()
}

// TransferID returns the transfer id of the data request
func (trsp *transferResponse)	TransferID() datatransfer.TransferID {
	return datatransfer.TransferID(trsp.XferID)
}

// 	Accepted returns true if the request is accepted in the response
func (trsp *transferResponse) Accepted() bool {
	return trsp.Acpt
}