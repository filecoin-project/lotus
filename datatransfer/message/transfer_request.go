package message

import (
	"fmt"
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/datatransfer/message/pb"
)

type TransferRequest struct {
	msg pb.Message
}

func NewTransferRequest(id datatransfer.TransferID, isPull bool, voucherIdentifier string, voucher []byte, baseCid cid.Cid, selector []byte) *TransferRequest {
	mr := pb.Message_Request{
		TransferID: uint64(id),
		IsPull:     isPull,
		Voucher:    voucher,
		PieceID:    nil,
		Selector:   selector,
		IsPartial:  false,
		IsCancel:   false,
		BaseCid:    baseCid.String(),
		VoucherID:  voucherIdentifier,
	}
	m := pb.Message{
		IsResponse: false,
		IsRequest:  true,
		Request:    &mr,
		Response:   nil,
	}
	return &TransferRequest{msg: m}
}

//
// ======= DataTransferMessage interface functions

// IsRequest returns true if this is a request (this should always be true)
func (dtr *TransferRequest) IsRequest() bool {
	return dtr.msg.IsRequest
}

func (dtr *TransferRequest) TransferID() datatransfer.TransferID {
	return datatransfer.TransferID(dtr.msg.Request.GetTransferID())
}

// ------- Exportable interface functions
func (dtr *TransferRequest) ToProto() *pb.Message {
	return &dtr.msg
}

func (dtr *TransferRequest) ToNet(w io.Writer) error {
	marshaled, err := dtr.msg.Marshal()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, marshaled); err != nil {
		return err
	}
	return nil
}

// ------- Loggable interface functions
func (dtr *TransferRequest) Loggable() map[string]interface{} {
	return make(map[string]interface{}, 0)
}

// ======= DataTransferMessage interface functions

// IsPull returns true if this is a pull request for data
func (dtr *TransferRequest) IsPull() bool {
	return dtr.msg.Request.GetIsPull()
}

// VoucherIdentifier returns the voucher identifier for this request
func (dtr *TransferRequest) VoucherIdentifier() string {
	return dtr.msg.Request.VoucherID
}

// Voucher returns the voucher for this request
func (dtr *TransferRequest) Voucher() []byte {
	return dtr.msg.Request.GetVoucher()
}

// BaseCid returns the BaseCid for this request
func (dtr *TransferRequest) BaseCid() string {
	return dtr.msg.Request.GetBaseCid()
}

// Selector returns the Selector for this request
func (dtr *TransferRequest) Selector() []byte {
	return dtr.msg.Request.GetSelector()
}

// IsCancel returns true if this request is a cancel request
func (dtr *TransferRequest) IsCancel() bool {
	return dtr.msg.Request.GetIsCancel()
}
