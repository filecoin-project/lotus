package retrievaladapter

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/address"
	retrievalmarket "github.com/filecoin-project/lotus/retrieval"
)

type retrievalProviderNode struct {
	full api.FullNode
}

// NewRetrievalProviderNode returns a new node adapter for a retrieval provider that talks to the
// Lotus Node
func NewRetrievalProviderNode(full api.FullNode) retrievalmarket.RetrievalProviderNode {
	return &retrievalProviderNode{full}
}

func (rpn *retrievalProviderNode) SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *retrievalmarket.SignedVoucher, proof []byte, expectedAmount retrievalmarket.BigInt) (retrievalmarket.BigInt, error) {
	return rpn.full.PaychVoucherAdd(ctx, paymentChannel, voucher, proof, expectedAmount)
}
