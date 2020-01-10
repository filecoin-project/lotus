package retrievaladapter

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievaltoken "github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	retrievaltypes "github.com/filecoin-project/go-fil-markets/shared/types"
	"github.com/filecoin-project/lotus/markets/utils"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/paych"
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
func (rcn *retrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable retrievaltoken.TokenAmount) (address.Address, error) {
	paych, _, err := rcn.pmgr.GetPaych(ctx, clientAddress, minerAddress, utils.FromSharedTokenAmount(clientFundsAvailable))
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
func (rcn *retrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount retrievaltoken.TokenAmount, lane uint64) (*retrievaltypes.SignedVoucher, error) {
	voucher, err := rcn.payapi.PaychVoucherCreate(ctx, paymentChannel, utils.FromSharedTokenAmount(amount), lane)
	if err != nil {
		return nil, err
	}
	return utils.ToSharedSignedVoucher(voucher)
}
