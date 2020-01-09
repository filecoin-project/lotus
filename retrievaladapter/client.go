package retrievaladapter

import (
	"context"

	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/paych"
	retrievalmarket "github.com/filecoin-project/lotus/retrieval"
)

type retrievalClientNode struct {
	pmgr   *paych.Manager
	payapi payapi.PaychAPI
}

// NewRetrievalClientNode returns a new node adapter for a retrieval client that talks to the
// Lotus Node
func NewRetrievalClientNode(pmgr *paych.Manager, payapi payapi.PaychAPI) retrievalmarket.RetrievalClientNode {
	return &retrievalClientNode{pmgr: pmgr, payapi: payapi}
}

// GetOrCreatePaymentChannel sets up a new payment channel if one does not exist
// between a client and a miner and insures the client has the given amount of funds available in the channel
func (rcn *retrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress retrievalmarket.Address, minerAddress retrievalmarket.Address, clientFundsAvailable retrievalmarket.BigInt) (retrievalmarket.Address, error) {
	paych, _, err := rcn.pmgr.GetPaych(ctx, clientAddress, minerAddress, clientFundsAvailable)
	return paych, err
}

// Allocate late creates a lane within a payment channel so that calls to
// CreatePaymentVoucher will automatically make vouchers only for the difference
// in total
func (rcn *retrievalClientNode) AllocateLane(paymentChannel retrievalmarket.Address) (uint64, error) {
	return rcn.pmgr.AllocateLane(paymentChannel)
}

// CreatePaymentVoucher creates a new payment voucher in the given lane for a
// given payment channel so that all the payment vouchers in the lane add up
// to the given amount (so the payment voucher will be for the difference)
func (rcn *retrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel retrievalmarket.Address, amount retrievalmarket.BigInt, lane uint64) (*retrievalmarket.SignedVoucher, error) {
	return rcn.payapi.PaychVoucherCreate(ctx, paymentChannel, amount, lane)
}
