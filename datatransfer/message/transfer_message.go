package message

import (
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/datatransfer"
)

type transferMessage struct {
	XferID uint64
	IsRq   bool

	Request  *transferRequest
	Response *transferResponse
}

// ========= DataTransferMessage interface
// Interface methods that are specific to DataTransferRequest, or to DataTransferResponse, are passthrough
// methods to each. These methods first check whether it is a request or a response, and either call the correct method
// on the request or response, or return zero-values as appropriate if it is the "wrong" type.
// Consumers of DTM are expected to make a check for the right type, i.e., call IsRequest, and then make the
// appropriate cast, however, mis-casting will not panic.  See message_test for more details about expectations and usage.

// IsRequest returns true if this message is a data request
func (tm *transferMessage) IsRequest() bool {
	return tm.IsRq
}

// IsResponse returns true if this message is a data response
func (tm *transferMessage) IsResponse() bool {
	return !tm.IsRq
}

// TransferID returns the TransferID of this message
func (tm *transferMessage) TransferID() datatransfer.TransferID {
	return datatransfer.TransferID(tm.XferID)
}

// IsPull returns the true if this message is a Pull request, false if it is not or is not a request at all.
func (tm *transferMessage) IsPull() bool {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.IsPull()
	}
	return false
}

// Voucher returns the Voucher bytes for a DataTransferRequest. If it is not a request it returns nil always.
func (tm *transferMessage) Voucher() []byte {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.Voucher()
	}
	return nil
}

// VoucherType returns the Voucher type for a DataTransferRequest. If it is not a request or request is nil
// it returns "invalid"
func (tm *transferMessage) VoucherType() string {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.VoucherType()
	}
	return "invalid"
}

// BaseCid returns the BaseCid type for a DataTransferRequest. If it is not a request or if request is nil
// it returns cid.Undef
func (tm *transferMessage) BaseCid() cid.Cid {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.BaseCid()
	}
	return cid.Undef
}

// Selector returns the Selector type for a DataTransferRequest. If it is not a request or request is nil
// it returns nil
func (tm *transferMessage) Selector() []byte {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.Selector()
	}
	return nil
}

// IsCancel returns true if DataTransferRequest is a Cancel request. If it is not a request or request is nil
// it returns false
func (tm *transferMessage) IsCancel() bool {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.IsCancel()
	}
	return false
}

// Accepted returns true if DataTransferResponse is Accepted response. If it is not a response or response is nil
// it returns false
func (tm *transferMessage) Accepted() bool {
	if !tm.IsRq && tm.Response != nil {
		return tm.Response.Accepted()
	}
	return false
}

// ToNet serializes a transfer message type. It is simply a wrapper for MarshalCBOR, to provide
// symmetry with FromNet
func (tm *transferMessage) ToNet(w io.Writer) error {
	return tm.MarshalCBOR(w)
}
