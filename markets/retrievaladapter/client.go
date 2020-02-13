package retrievaladapter

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/paychmgr"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type retrievalClientNode struct {
	pmgr   *paychmgr.Manager
	payapi payapi.PaychAPI
}

// NewRetrievalClientNode returns a new node adapter for a retrieval client that talks to the
// Lotus Node
func NewRetrievalClientNode(pmgr *paychmgr.Manager, payapi payapi.PaychAPI) retrievalmarket.RetrievalClientNode {
	return &retrievalClientNode{pmgr: pmgr, payapi: payapi}
}

// GetOrCreatePaymentChannel sets up a new payment channel if one does not exist
// between a client and a miner and insures the client has the given amount of funds available in the channel
func (rcn *retrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable abi.TokenAmount) (address.Address, error) {
	paych, _, err := rcn.pmgr.GetPaych(ctx, clientAddress, minerAddress, clientFundsAvailable)
	return paych, err
}

// Allocate late creates a lane within a payment channel so that calls to
// CreatePaymentVoucher will automatically make vouchers only for the difference
// in total
func (rcn *retrievalClientNode) AllocateLane(paymentChannel address.Address) (uint64, error) {
	return rcn.pmgr.AllocateLane(paymentChannel)
}

// CreatePaymentVoucher creates a new payment voucher in the given lane for a
// given payment channel so that all the payment vouchers in the lane add up
// to the given amount (so the payment voucher will be for the difference)
func (rcn *retrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount abi.TokenAmount, lane uint64) (*paychmgr.SignedVoucher, error) {
	voucher, err := rcn.payapi.PaychVoucherCreate(ctx, paymentChannel, amount, lane)
	if err != nil {
		return nil, err
	}
	return voucher
}
