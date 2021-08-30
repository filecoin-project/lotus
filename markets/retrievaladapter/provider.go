package retrievaladapter

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/types"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("retrievaladapter")

type retrievalProviderNode struct {
	full v1api.FullNode
}

var _ retrievalmarket.RetrievalProviderNode = (*retrievalProviderNode)(nil)

// NewRetrievalProviderNode returns a new node adapter for a retrieval provider that talks to the
// Lotus Node
func NewRetrievalProviderNode(full v1api.FullNode) retrievalmarket.RetrievalProviderNode {
	return &retrievalProviderNode{full: full}
}

func (rpn *retrievalProviderNode) GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return address.Undef, err
	}

	mi, err := rpn.full.StateMinerInfo(ctx, miner, tsk)
	return mi.Worker, err
}

func (rpn *retrievalProviderNode) SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *paych.SignedVoucher, proof []byte, expectedAmount abi.TokenAmount, tok shared.TipSetToken) (abi.TokenAmount, error) {
	// TODO: respect the provided TipSetToken (a serialized TipSetKey) when
	// querying the chain
	added, err := rpn.full.PaychVoucherAdd(ctx, paymentChannel, voucher, proof, expectedAmount)
	return added, err
}

func (rpn *retrievalProviderNode) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	head, err := rpn.full.ChainHead(ctx)
	if err != nil {
		return nil, 0, err
	}

	return head.Key().Bytes(), head.Height(), nil
}

// GetRetrievalPricingInput takes a set of candidate storage deals that can serve a retrieval request,
// and returns an minimally populated PricingInput. This PricingInput should be enhanced
// with more data, and passed to the pricing function to determine the final quoted price.
func (rpn *retrievalProviderNode) GetRetrievalPricingInput(ctx context.Context, pieceCID cid.Cid, storageDeals []abi.DealID) (retrievalmarket.PricingInput, error) {
	resp := retrievalmarket.PricingInput{}

	head, err := rpn.full.ChainHead(ctx)
	if err != nil {
		return resp, xerrors.Errorf("failed to get chain head: %w", err)
	}
	tsk := head.Key()

	var mErr error

	for _, dealID := range storageDeals {
		ds, err := rpn.full.StateMarketStorageDeal(ctx, dealID, tsk)
		if err != nil {
			log.Warnf("failed to look up deal %d on chain: err=%w", dealID, err)
			mErr = multierror.Append(mErr, err)
			continue
		}
		if ds.Proposal.VerifiedDeal {
			resp.VerifiedDeal = true
		}

		if ds.Proposal.PieceCID.Equals(pieceCID) {
			resp.PieceSize = ds.Proposal.PieceSize.Unpadded()
		}

		// If we've discovered a verified deal with the required PieceCID, we don't need
		// to lookup more deals and we're done.
		if resp.VerifiedDeal && resp.PieceSize != 0 {
			break
		}
	}

	// Note: The piece size can never actually be zero. We only use it to here
	// to assert that we didn't find a matching piece.
	if resp.PieceSize == 0 {
		if mErr == nil {
			return resp, xerrors.New("failed to find matching piece")
		}

		return resp, xerrors.Errorf("failed to fetch storage deal state: %w", mErr)
	}

	return resp, nil
}
