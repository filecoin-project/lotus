package retrievaladapter

import (
	"context"
	"io"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage"
)

type retrievalProviderNode struct {
	miner *storage.Miner
	sb    sectorbuilder.Interface
	full  api.FullNode
}

// NewRetrievalProviderNode returns a new node adapter for a retrieval provider that talks to the
// Lotus Node
func NewRetrievalProviderNode(miner *storage.Miner, sb sectorbuilder.Interface, full api.FullNode) retrievalmarket.RetrievalProviderNode {
	return &retrievalProviderNode{miner, sb, full}
}

func (rpn *retrievalProviderNode) UnsealSector(ctx context.Context, sectorID uint64, offset uint64, length uint64) (io.ReadCloser, error) {
	si, err := rpn.miner.GetSectorInfo(abi.SectorNumber(sectorID))
	if err != nil {
		return nil, err
	}
	return rpn.sb.ReadPieceFromSealedSector(ctx, abi.SectorNumber(sectorID), sectorbuilder.UnpaddedByteIndex(offset), abi.UnpaddedPieceSize(length), si.Ticket.Value, *si.CommD)
}

func (rpn *retrievalProviderNode) SavePaymentVoucher(ctx context.Context, paymentChannel address.Address, voucher *paych.SignedVoucher, proof []byte, expectedAmount abi.TokenAmount) (abi.TokenAmount, error) {
	added, err := rpn.full.PaychVoucherAdd(ctx, paymentChannel, voucher, proof, expectedAmount)
	return added, err
}
