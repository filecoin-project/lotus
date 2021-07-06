package retrievaladapter

import (
	"context"
	"io"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/types"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-state-types/abi"
	specstorage "github.com/filecoin-project/specs-storage/storage"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("retrievaladapter")

type retrievalProviderNode struct {
	maddr address.Address
	secb  sectorblocks.SectorBuilder
	pp    sectorstorage.PieceProvider
	full  v1api.FullNode
}

// NewRetrievalProviderNode returns a new node adapter for a retrieval provider that talks to the
// Lotus Node
func NewRetrievalProviderNode(maddr dtypes.MinerAddress, secb sectorblocks.SectorBuilder, pp sectorstorage.PieceProvider, full v1api.FullNode) retrievalmarket.RetrievalProviderNode {
	return &retrievalProviderNode{address.Address(maddr), secb, pp, full}
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
	si, err := rpn.sectorsStatus(ctx, sectorID, true)
	if err != nil {
		return nil, err
	}

	mid, err := address.IDFromAddress(rpn.maddr)
	if err != nil {
		return nil, err
	}

	ref := specstorage.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: sectorID,
		},
		ProofType: si.SealProof,
	}

	var commD cid.Cid
	if si.CommD != nil {
		commD = *si.CommD
	}

	// Get a reader for the piece, unsealing the piece if necessary
	log.Debugf("read piece in sector %d, offset %d, length %d from miner %d", sectorID, offset, length, mid)
	r, unsealed, err := rpn.pp.ReadPiece(ctx, ref, storiface.UnpaddedByteIndex(offset), length, si.Ticket.Value, commD)
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
	si, err := rpn.secb.SectorsStatus(ctx, sectorID, false)
	if err != nil {
		return false, xerrors.Errorf("failed to get sector info: %w", err)
	}

	mid, err := address.IDFromAddress(rpn.maddr)
	if err != nil {
		return false, err
	}

	ref := specstorage.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: sectorID,
		},
		ProofType: si.SealProof,
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

func (rpn *retrievalProviderNode) sectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) {
	sInfo, err := rpn.secb.SectorsStatus(ctx, sid, false)
	if err != nil {
		return api.SectorInfo{}, err
	}

	if !showOnChainInfo {
		return sInfo, nil
	}

	onChainInfo, err := rpn.full.StateSectorGetInfo(ctx, rpn.maddr, sid, types.EmptyTSK)
	if err != nil {
		return sInfo, err
	}
	if onChainInfo == nil {
		return sInfo, nil
	}
	sInfo.SealProof = onChainInfo.SealProof
	sInfo.Activation = onChainInfo.Activation
	sInfo.Expiration = onChainInfo.Expiration
	sInfo.DealWeight = onChainInfo.DealWeight
	sInfo.VerifiedDealWeight = onChainInfo.VerifiedDealWeight
	sInfo.InitialPledge = onChainInfo.InitialPledge

	ex, err := rpn.full.StateSectorExpiration(ctx, rpn.maddr, sid, types.EmptyTSK)
	if err != nil {
		return sInfo, nil
	}
	sInfo.OnTime = ex.OnTime
	sInfo.Early = ex.Early

	return sInfo, nil
}
