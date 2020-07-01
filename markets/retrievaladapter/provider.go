package retrievaladapter

import (
	"context"
	"github.com/filecoin-project/sector-storage/storiface"
	"io"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	sectorstorage "github.com/filecoin-project/sector-storage"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage"
)

type retrievalProviderNode struct {
	miner  *storage.Miner
	sealer sectorstorage.SectorManager
	full   api.FullNode
}

// NewRetrievalProviderNode returns a new node adapter for a retrieval provider that talks to the
// Lotus Node
func NewRetrievalProviderNode(miner *storage.Miner, sealer sectorstorage.SectorManager, full api.FullNode) retrievalmarket.RetrievalProviderNode {
	return &retrievalProviderNode{miner, sealer, full}
}

func (rpn *retrievalProviderNode) GetMinerWorkerAddress(ctx context.Context, miner address.Address, tok shared.TipSetToken) (address.Address, error) {
	tsk, err := types.TipSetKeyFromBytes(tok)
	if err != nil {
		return address.Undef, err
	}

	mi, err := rpn.full.StateMinerInfo(ctx, miner, tsk)
	return mi.Worker, err
}

func (rpn *retrievalProviderNode) UnsealSector(ctx context.Context, sectorID uint64, offset uint64, length uint64) (io.ReadCloser, error) {
	si, err := rpn.miner.GetSectorInfo(abi.SectorNumber(sectorID))
	if err != nil {
		return nil, err
	}

	mid, err := address.IDFromAddress(rpn.miner.Address())
	if err != nil {
		return nil, err
	}

	sid := abi.SectorID{
		Miner:  abi.ActorID(mid),
		Number: abi.SectorNumber(sectorID),
	}

	r, w := io.Pipe()
	go func() {
		err := rpn.sealer.ReadPiece(ctx, w, sid, storiface.UnpaddedByteIndex(offset), abi.UnpaddedPieceSize(length), si.TicketValue, *si.CommD)
		_ = w.CloseWithError(err)
	}()

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
