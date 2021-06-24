package mount

import (
	"context"
	"io"

	"github.com/filecoin-project/go-fil-markets/dagstore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

var _ dagstore.LotusMountAPI = (*lotusMount)(nil)

type lotusMount struct {
	pieceStore piecestore.PieceStore
	rm         retrievalmarket.RetrievalProviderNode
}

func NewLotusMount(store piecestore.PieceStore, rm retrievalmarket.RetrievalProviderNode) *lotusMount {
	return &lotusMount{
		pieceStore: store,
		rm:         rm,
	}
}

func (l *lotusMount) FetchUnsealedPiece(ctx context.Context, pieceCid cid.Cid) (io.ReadCloser, error) {
	pieceInfo, err := l.pieceStore.GetPieceInfo(pieceCid)
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch pieceInfo, err=%w", err)
	}

	if len(pieceInfo.Deals) <= 0 {
		return nil, xerrors.New("no storage deals for Piece")
	}

	// prefer an unsealed sector containing the piece if one exists
	for _, deal := range pieceInfo.Deals {
		isUnsealed, err := l.rm.IsUnsealed(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
		if err != nil {
			continue
		}
		if isUnsealed {
			// UnsealSector will NOT unseal a sector if we already have an unsealed copy lying around.
			reader, err := l.rm.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
			if err == nil {
				return reader, nil
			}
		}
	}

	lastErr := xerrors.New("no sectors found to unseal from")
	// if there is no unsealed sector containing the piece, just read the piece from the first sector we are able to unseal.
	for _, deal := range pieceInfo.Deals {
		reader, err := l.rm.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
		if err == nil {
			return reader, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (l *lotusMount) GetUnpaddedCARSize(pieceCid cid.Cid) (uint64, error) {
	pieceInfo, err := l.pieceStore.GetPieceInfo(pieceCid)
	if err != nil {
		return 0, xerrors.Errorf("failed to fetch pieceInfo, err=%w", err)
	}

	if len(pieceInfo.Deals) <= 0 {
		return 0, xerrors.New("no storage deals for piece")
	}

	len := pieceInfo.Deals[0].Length.Unpadded()

	return uint64(len), nil
}
