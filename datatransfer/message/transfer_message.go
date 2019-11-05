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
// IsRequest returns true if this message is a data request
func (tm *transferMessage) IsRequest() bool {
	return tm.IsRq
}

// IsResponse returns true if this message is a data response
func (tm *transferMessage) IsResponse() bool {
	return !tm.IsRq
}

func (tm *transferMessage) TransferID() datatransfer.TransferID {
	return datatransfer.TransferID(tm.XferID)
}

func (tm *transferMessage) IsPull() bool {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.IsPull()
	}
	return false
}

func (tm *transferMessage) Voucher() []byte {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.Voucher()
	}
	return nil
}

func (tm *transferMessage) VoucherType() string {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.VoucherType()
	}
	return "invalid"
}

func (tm *transferMessage) BaseCid() cid.Cid {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.BaseCid()
	}
	return cid.Undef
}

func (tm *transferMessage) Selector() []byte {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.Selector()
	}
	return nil
}

func (tm *transferMessage) IsCancel() bool {
	if tm.IsRq && tm.Request != nil {
		return tm.Request.IsCancel()
	}
	return false
}

func (tm *transferMessage) Accepted() bool {
	if !tm.IsRq && tm.Response != nil {
		return tm.Response.Accepted()
	}
	return false
}

// ToNet serializes a transfer message type. It's a wrapper for MarshalCBOR to provide
// symmetry with FromNet
func (tm *transferMessage) ToNet(w io.Writer) error {
	return tm.MarshalCBOR(w)
}
