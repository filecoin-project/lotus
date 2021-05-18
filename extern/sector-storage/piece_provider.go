package sectorstorage

import (
	"bufio"
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/fr32"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

type Unsealer interface {
	SectorsUnsealPiece(ctx context.Context, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd *cid.Cid) error
}

type PieceProvider interface {
	ReadPiece(ctx context.Context, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealed cid.Cid) (io.ReadCloser, bool, error)
}

type pieceProvider struct {
	storage *stores.Remote
	index   stores.SectorIndex
	uns     Unsealer
}

func NewPieceProvider(storage *stores.Remote, index stores.SectorIndex, uns Unsealer) PieceProvider {
	return &pieceProvider{
		storage: storage,
		index:   index,
		uns:     uns,
	}
}

func (p *pieceProvider) tryReadUnsealedPiece(ctx context.Context, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (io.ReadCloser, context.CancelFunc, error) {
	// acquire a lock purely for reading unsealed sectors
	ctx, cancel := context.WithCancel(ctx)
	if err := p.index.StorageLock(ctx, sector.ID, storiface.FTUnsealed, storiface.FTNone); err != nil {
		cancel()
		return nil, nil, xerrors.Errorf("acquiring read sector lock: %w", err)
	}

	r, err := p.storage.Reader(ctx, sector, abi.PaddedPieceSize(offset.Padded()), size.Padded())
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if r == nil {
		cancel()
	}

	return r, cancel, nil
}

func (p *pieceProvider) ReadPiece(ctx context.Context, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealed cid.Cid) (io.ReadCloser, bool, error) {
	if err := offset.Valid(); err != nil {
		return nil, false, xerrors.Errorf("offset is not valid: %w", err)
	}
	if err := size.Validate(); err != nil {
		return nil, false, xerrors.Errorf("size is not a valid piece size: %w", err)
	}

	r, unlock, err := p.tryReadUnsealedPiece(ctx, sector, offset, size)
	if xerrors.Is(err, storiface.ErrSectorNotFound) {
		err = nil
	}
	if err != nil {
		return nil, false, err
	}

	var uns bool
	if r == nil {
		uns = true
		commd := &unsealed
		if unsealed == cid.Undef {
			commd = nil
		}
		if err := p.uns.SectorsUnsealPiece(ctx, sector, offset, size, ticket, commd); err != nil {
			return nil, false, xerrors.Errorf("unsealing piece: %w", err)
		}

		r, unlock, err = p.tryReadUnsealedPiece(ctx, sector, offset, size)
		if err != nil {
			return nil, true, xerrors.Errorf("read after unsealing: %w", err)
		}
		if r == nil {
			return nil, true, xerrors.Errorf("got no reader after unsealing piece")
		}
	}

	upr, err := fr32.NewUnpadReader(r, size.Padded())
	if err != nil {
		return nil, uns, xerrors.Errorf("creating unpadded reader: %w", err)
	}

	return &funcCloser{
		Reader: bufio.NewReaderSize(upr, 127),
		close: func() error {
			err = r.Close()
			unlock()
			return err
		},
	}, uns, nil
}

type funcCloser struct {
	io.Reader
	close func() error
}

func (fc *funcCloser) Close() error { return fc.close() }
