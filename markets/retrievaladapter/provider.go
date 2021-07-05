package retrievaladapter

import (
	"context"
	"io"

	"github.com/filecoin-project/lotus/api/v1api"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/types"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/storage"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-state-types/abi"
	specstorage "github.com/filecoin-project/specs-storage/storage"
)

var log = logging.Logger("retrievaladapter")

type retrievalProviderNode struct {
	miner *storage.Miner
	pp    sectorstorage.PieceProvider
	full  v1api.FullNode
}

// NewRetrievalProviderNode returns a new node adapter for a retrieval provider that talks to the
// Lotus Node
func NewRetrievalProviderNode(miner *storage.Miner, pp sectorstorage.PieceProvider, full v1api.FullNode) retrievalmarket.RetrievalProviderNode {
	return &retrievalProviderNode{miner, pp, full}
}

func (rpn *retrievalProviderNode) GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return address.Undef, err
	}

	mi, err := rpn.full.StateMinerInfo(ctx, miner, tsk)
	return mi.Worker, err
}

func (rpn *retrievalProviderNode) UnsealSector(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (io.ReadCloser, error) {
	log.Debugf("get sector %d, offset %d, length %d", sectorID, offset, length)

	si, err := rpn.miner.GetSectorInfo(sectorID)
	if err != nil {
		return nil, err
	}

	mid, err := address.IDFromAddress(rpn.miner.Address())
	if err != nil {
		return nil, err
	}

	ref := specstorage.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: sectorID,
		},
		ProofType: si.SectorType,
	}

	var commD cid.Cid
	if si.CommD != nil {
		commD = *si.CommD
	}

	// Get a reader for the piece, unsealing the piece if necessary
	log.Debugf("read piece in sector %d, offset %d, length %d from miner %d", sectorID, offset, length, mid)
	r, unsealed, err := rpn.pp.ReadPiece(ctx, ref, storiface.UnpaddedByteIndex(offset), length, si.TicketValue, commD)
	if err != nil {
		return nil, xerrors.Errorf("failed to unseal piece from sector %d: %w", sectorID, err)
	}
	_ = unsealed // todo: use

	return r, nil
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

func (rpn *retrievalProviderNode) IsUnsealed(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (bool, error) {
	si, err := rpn.miner.GetSectorInfo(sectorID)
	if err != nil {
		return false, xerrors.Errorf("failed to get sectorinfo, err=%s", err)
	}

	mid, err := address.IDFromAddress(rpn.miner.Address())
	if err != nil {
		return false, err
	}

	ref := specstorage.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: sectorID,
		},
		ProofType: si.SectorType,
	}

	log.Debugf("will call IsUnsealed now sector=%+v, offset=%d, size=%d", sectorID, offset, length)
	return rpn.pp.IsUnsealed(ctx, ref, storiface.UnpaddedByteIndex(offset), length)
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

	var lastErr error

	for _, dealID := range storageDeals {
		ds, err := rpn.full.StateMarketStorageDeal(ctx, dealID, tsk)
		if err != nil {
			log.Warnf("failed to look up deal %d on chain: err=%w", dealID, err)
			lastErr = err
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
		if lastErr == nil {
			return resp, xerrors.New("failed to find matching piece")
		} else {
			return resp, xerrors.Errorf("failed to fetch storage deal state: %w", err)
		}
	}

	return resp, nil
}
