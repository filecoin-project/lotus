package dagstore

import (
	"context"
	"io"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type LotusAccessor interface {
	FetchUnsealedPiece(ctx context.Context, pieceCid cid.Cid) (io.ReadCloser, error)
	GetUnpaddedCARSize(ctx context.Context, pieceCid cid.Cid) (uint64, error)
}

type lotusAccessor struct {
	pieceStore piecestore.PieceStore
	rm         retrievalmarket.RetrievalProviderNode

	startLk  sync.Mutex
	started  bool
	startErr error
}

var _ LotusAccessor = (*lotusAccessor)(nil)

func NewLotusAccessor(store piecestore.PieceStore, rm retrievalmarket.RetrievalProviderNode) *lotusAccessor {
	return &lotusAccessor{
		pieceStore: store,
		rm:         rm,
	}
}

func (m *lotusAccessor) start(ctx context.Context) error {
	m.startLk.Lock()
	defer m.startLk.Unlock()

	if m.started {
		return m.startErr
	}

	// Wait for the piece store to startup
	ready := make(chan error)
	m.pieceStore.OnReady(func(err error) {
		select {
		case <-ctx.Done():
		case ready <- err:
		}
	})

	select {
	case <-ctx.Done():
		return xerrors.Errorf("context cancelled waiting for piece store startup: %w", ctx.Err())
	case err := <-ready:
		// Piece store has started up, check if there was an error
		if err != nil {
			m.startErr = err
			return err
		}
	}

	// Piece store has started up successfully
	return nil
}

func (m *lotusAccessor) FetchUnsealedPiece(ctx context.Context, pieceCid cid.Cid) (io.ReadCloser, error) {
	err := m.start(ctx)
	if err != nil {
		return nil, err
	}

	pieceInfo, err := m.pieceStore.GetPieceInfo(pieceCid)
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch pieceInfo for piece %s: %w", pieceCid, err)
	}

	if len(pieceInfo.Deals) == 0 {
		return nil, xerrors.Errorf("no storage deals found for Piece %s", pieceCid)
	}

	// prefer an unsealed sector containing the piece if one exists
	for _, deal := range pieceInfo.Deals {
		isUnsealed, err := m.rm.IsUnsealed(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
		if err != nil {
			log.Warnf("failed to check if deal %d unsealed: %s", deal.DealID, err)
			continue
		}
		if isUnsealed {
			// UnsealSector will NOT unseal a sector if we already have an unsealed copy lying around.
			reader, err := m.rm.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
			if err == nil {
				return reader, nil
			}
		}
	}

	lastErr := xerrors.New("no sectors found to unseal from")
	// if there is no unsealed sector containing the piece, just read the piece from the first sector we are able to unseal.
	for _, deal := range pieceInfo.Deals {
		// Note that if the deal data is not already unsealed, unsealing may
		// block for a long time with the current PoRep
		reader, err := m.rm.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
		if err != nil {
			lastErr = xerrors.Errorf("failed to unseal deal %d: %w", deal.DealID, err)
			log.Warn(lastErr.Error())
			continue
		}

		// Successfully fetched the deal data so return a reader over the data
		return reader, nil
	}

	return nil, lastErr
}

func (m *lotusAccessor) GetUnpaddedCARSize(ctx context.Context, pieceCid cid.Cid) (uint64, error) {
	err := m.start(ctx)
	if err != nil {
		return 0, err
	}

	pieceInfo, err := m.pieceStore.GetPieceInfo(pieceCid)
	if err != nil {
		return 0, xerrors.Errorf("failed to fetch pieceInfo for piece %s: %w", pieceCid, err)
	}

	if len(pieceInfo.Deals) == 0 {
		return 0, xerrors.Errorf("no storage deals found for piece %s", pieceCid)
	}

	len := pieceInfo.Deals[0].Length

	return uint64(len), nil
}
