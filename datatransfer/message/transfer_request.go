package message

import (
	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/ipfs/go-cid"
)

// transferRequest is a struct that fulfills the DataTransferRequest interface.
// its members are exported to be used by cbor-gen
type transferRequest struct {
	XferID uint64
	Pull   bool
	Vouch  []byte
	PID    []byte
	Stor   []byte
	Part   bool
	Canc   bool
	BCid   string
	VTyp   string
}

// ========= DataTransferMessage interface
// IsRequest always returns true because this message is a data request
func (trq *transferRequest) IsRequest() bool {
	return true
}
// IsResponse always returns false because this message is not a data response
func (trq *transferRequest) IsResponse() bool {
	return !trq.IsRequest()
}

// ========= DataTransferRequest interface
// IsPull returns true if this is a data pull request
func (trq *transferRequest) IsPull() bool {
	return trq.Pull
}
// VoucherType returns the Voucher ID
func (trq *transferRequest) VoucherType() string {
	return trq.VTyp
}

// Voucher returns the Voucher bytes
func (trq *transferRequest) Voucher() []byte {
	return trq.Vouch
}

// BaseCid returns the Base CID
func (trq *transferRequest) BaseCid() cid.Cid {
	res, err := cid.Decode(trq.BCid)
	if err != nil {
		return cid.Undef
	}
	return res
}

// Selector returns the message Selector bytes
func (trq *transferRequest) Selector() []byte {
	return trq.Stor
}

// IsCancel returns true if this is a cancel request
func (trq *transferRequest) IsCancel() bool {
	return trq.Canc
}

// IsPartial returns true if this is a partial request
func (trq *transferRequest) IsPartial() bool {
	return trq.Part
}

// TransferID returns the message transfer ID
func (trq *transferRequest) TransferID() datatransfer.TransferID {
	return datatransfer.TransferID(trq.XferID)
}

// Cancel cancels
func (trq *transferRequest) Cancel() error {
	// do other stuff ?
	trq.Canc = true
	return nil
}