package message

import (
	"fmt"
	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/datatransfer/message/pb"
	"io"
)

type DataTransferRequest struct {
	msg pb.Message
}

// ======= DataTransferMessage interface functions

// IsRequest returns true if this is a request (this should always be true)
func (dtr *DataTransferRequest) IsRequest() bool {
	return dtr.msg.IsRequest
}

func (dtr *DataTransferRequest) ToProto() *pb.Message {
	return nil
}

func (dtr *DataTransferRequest) ToNet(w io.Writer) error {
	if _, err := fmt.Fprint(w, "NOT IMPLEMENTED"); err != nil {
		return err
	}
	return nil
}

// ======= DataTransferMessage interface functions

// IsPull returns true if this is a pull request for data
func (dtr *DataTransferRequest) IsPull() bool {
	return dtr.msg.Request.GetIsPull()
}

// VoucherIdentifier returns the voucher identifier for this request
func (dtr *DataTransferRequest) VoucherIdentifier() datatransfer.TransferID {
	return datatransfer.TransferID(0)
}

func (dtr *DataTransferRequest)Voucher() []byte {
	return dtr.msg.Request.GetVoucher()
}
func (dtr *DataTransferRequest) BaseCid() string {
	return dtr.msg.Request.GetBaseCid()
}
func (dtr *DataTransferRequest) Selector() []byte {
	return dtr.msg.Request.GetSelector()
}
func (dtr *DataTransferRequest)IsCancel() bool {
	return dtr.msg.Request.GetIsCancel()
}

