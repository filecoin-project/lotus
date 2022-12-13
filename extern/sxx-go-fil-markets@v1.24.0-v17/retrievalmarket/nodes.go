package retrievalmarket

import (
	"context"

	"github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/go-fil-markets/shared"
)

// RetrievalClientNode are the node dependencies for a RetrievalClient
type RetrievalClientNode interface {
	GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)

	// GetOrCreatePaymentChannel sets up a new payment channel if one does not exist
	// between a client and a miner and ensures the client has the given amount of funds available in the channel
	GetOrCreatePaymentChannel(ctx context.Context, clientAddress, minerAddress address.Address,
		clientFundsAvailable abi.TokenAmount, tok shared.TipSetToken) (address.Address, cid.Cid, error)

	// CheckAvailableFunds returns the amount of current and incoming funds in a channel
	CheckAvailableFunds(ctx context.Context, paymentChannel address.Address) (ChannelAvailableFunds, error)

	// Allocate late creates a lane within a payment channel so that calls to
	// CreatePaymentVoucher will automatically make vouchers only for the difference
	// in total
	AllocateLane(ctx context.Context, paymentChannel address.Address) (uint64, error)

	// CreatePaymentVoucher creates a new payment voucher in the given lane for a
	// given payment channel so that all the payment vouchers in the lane add up
	// to the given amount (so the payment voucher will be for the difference)
	CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount abi.TokenAmount,
		lane uint64, tok shared.TipSetToken) (*paychtypes.SignedVoucher, error)

	// WaitForPaymentChannelReady just waits for the payment channel's pending operations to complete
	WaitForPaymentChannelReady(ctx context.Context, waitSentinel cid.Cid) (address.Address, error)

	// GetKnownAddresses gets any on known multiaddrs for a given address, so we can add to the peer store
	GetKnownAddresses(ctx context.Context, p RetrievalPeer, tok shared.TipSetToken) ([]ma.Multiaddr, error)
}

// RetrievalProviderNode are the node dependencies for a RetrievalProvider
type RetrievalProviderNode interface {
	GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error)

	// returns the worker address associated with a miner
	GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error)
	SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *paychtypes.SignedVoucher, proof []byte, expectedAmount abi.TokenAmount, tok shared.TipSetToken) (abi.TokenAmount, error)

	GetRetrievalPricingInput(ctx context.Context, pieceCID cid.Cid, storageDeals []abi.DealID) (PricingInput, error)
}
