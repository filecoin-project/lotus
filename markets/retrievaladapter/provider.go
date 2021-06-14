package retrievaladapter

import (
	"context"
	"io"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
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
	si, err := rpn.secb.SectorsStatus(ctx, sectorID, false)
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
