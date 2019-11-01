package message

import (
	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/ipfs/go-cid"
)

type TransferRequest struct {
	XferID uint64
	Pull   bool
	Vouch  []byte
	PID    []byte
	Stor   []byte
	Part   bool
	Canc   bool
	BCid   string
	VID    string
}

// ========= DataTransferMessage interface

func (trq *TransferRequest) IsRequest() bool {
	return true
}
func (trq *TransferRequest) IsResponse() bool {
	return !trq.IsRequest()
}

// -------- Loggable interface

func (trq *TransferRequest) Loggable() map[string]interface{} {
	return make(map[string]interface{}, 0)
}

// ========= DataTransferRequest interface

func (trq *TransferRequest) IsPull() bool {
	return trq.Pull
}
func (trq *TransferRequest) VoucherIdentifier() string {
	return trq.VID
}
func (trq *TransferRequest) Voucher() []byte {
	return trq.Vouch
}
func (trq *TransferRequest) BaseCid() cid.Cid {
	res, err := cid.Decode(trq.BCid)
	if err != nil {
		return cid.Undef
	}
	return res
}
func (trq *TransferRequest) Selector() []byte {
	return trq.Stor
}
func (trq *TransferRequest) IsCancel() bool {
	return trq.Canc
}

func (trq *TransferRequest) IsPartial() bool {
	return trq.Part
}

func (trq *TransferRequest) TransferID() datatransfer.TransferID {
	return datatransfer.TransferID(trq.XferID)
}

func (trq *TransferRequest) Cancel() error {
	// do other stuff ?
	trq.Canc = true
	return nil
}