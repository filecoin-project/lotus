package sealer

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/ipfs/go-cid"
	pool "github.com/libp2p/go-buffer-pool"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/fr32"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type Unsealer interface {
	// SectorsUnsealPiece will Unseal a Sealed sector file for the given sector.
	SectorsUnsealPiece(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd *cid.Cid) error
}

type PieceProvider interface {
	// ReadPiece is used to read an Unsealed piece at the given offset and of the given size from a Sector
	// pieceOffset + pieceSize specify piece bounds for unsealing (note: with SDR the entire sector will be unsealed by
	//  default in most cases, but this might matter with future PoRep)
	// startOffset is added to the pieceOffset to get the starting reader offset.
	// The number of bytes that can be read is pieceSize-startOffset
	ReadPiece(ctx context.Context, sector storiface.SectorRef, pieceOffset storiface.UnpaddedByteIndex, pieceSize abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealed cid.Cid) (storiface.Reader, bool, error)
	IsUnsealed(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error)
}

var _ PieceProvider = &pieceProvider{}

type pieceProvider struct {
	storage *paths.Remote
	index   paths.SectorIndex
	uns     Unsealer
}

func NewPieceProvider(storage *paths.Remote, index paths.SectorIndex, uns Unsealer) PieceProvider {
	return &pieceProvider{
		storage: storage,
		index:   index,
		uns:     uns,
	}
}

// IsUnsealed checks if we have the unsealed piece at the given offset in an already
// existing unsealed file either locally or on any of the workers.
func (p *pieceProvider) IsUnsealed(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error) {
	if err := offset.Valid(); err != nil {
		return false, xerrors.Errorf("offset is not valid: %w", err)
	}
	if err := size.Validate(); err != nil {
		return false, xerrors.Errorf("size is not a valid piece size: %w", err)
	}

	ctxLock, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := p.index.StorageLock(ctxLock, sector.ID, storiface.FTUnsealed, storiface.FTNone); err != nil {
		return false, xerrors.Errorf("acquiring read sector lock: %w", err)
	}

	return p.storage.CheckIsUnsealed(ctxLock, sector, abi.PaddedPieceSize(offset.Padded()), size.Padded())
}

// tryReadUnsealedPiece will try to read the unsealed piece from an existing unsealed sector file for the given sector from any worker that has it.
// It will NOT try to schedule an Unseal of a sealed sector file for the read.
//
// Returns a nil reader if the piece does NOT exist in any unsealed file or there is no unsealed file for the given sector on any of the workers.
func (p *pieceProvider) tryReadUnsealedPiece(ctx context.Context, pc cid.Cid, sector storiface.SectorRef, pieceOffset storiface.UnpaddedByteIndex, pieceSize abi.UnpaddedPieceSize) (storiface.Reader, error) {
	// acquire a lock purely for reading unsealed sectors
	ctx, cancel := context.WithCancel(ctx)
	if err := p.index.StorageLock(ctx, sector.ID, storiface.FTUnsealed, storiface.FTNone); err != nil {
		cancel()
		return nil, xerrors.Errorf("acquiring read sector lock: %w", err)
	}

	// Reader returns a reader getter for an unsealed piece at the given offset in the given sector.
	// The returned reader will be nil if none of the workers has an unsealed sector file containing
	// the unsealed piece.
	readerGetter, err := p.storage.Reader(ctx, sector, abi.PaddedPieceSize(pieceOffset.Padded()), pieceSize.Padded())
	if err != nil {
		cancel()
		log.Debugf("did not get storage reader;sector=%+v, err:%s", sector.ID, err)
		return nil, err
	}
	if readerGetter == nil {
		cancel()
		return nil, nil
	}

	pr, err := (&pieceReader{
		getReader: func(startOffset, readSize uint64) (io.ReadCloser, error) {
			// The request is for unpadded bytes, at any offset.
			// storage.Reader readers give us fr32-padded bytes, so we need to
			// do the unpadding here.

			startOffsetAligned := storiface.UnpaddedFloor(startOffset)
			startOffsetDiff := int(startOffset - uint64(startOffsetAligned))

			endOffset := startOffset + readSize
			endOffsetAligned := storiface.UnpaddedCeil(endOffset)

			r, err := readerGetter(startOffsetAligned.Padded(), endOffsetAligned.Padded())
			if err != nil {
				return nil, xerrors.Errorf("getting reader at +%d: %w", startOffsetAligned, err)
			}

			buf := pool.Get(fr32.BufSize(pieceSize.Padded()))

			upr, err := fr32.NewUnpadReaderBuf(r, pieceSize.Padded(), buf)
			if err != nil {
				r.Close() // nolint
				return nil, xerrors.Errorf("creating unpadded reader: %w", err)
			}

			bir := bufio.NewReaderSize(upr, 127)
			if startOffset > uint64(startOffsetAligned) {
				if _, err := bir.Discard(startOffsetDiff); err != nil {
					r.Close() // nolint
					return nil, xerrors.Errorf("discarding bytes for startOffset: %w", err)
				}
			}

			var closeOnce sync.Once

			return struct {
				io.Reader
				io.Closer
			}{
				Reader: bir,
				Closer: funcCloser(func() error {
					closeOnce.Do(func() {
						pool.Put(buf)
					})
					return r.Close()
				}),
			}, nil
		},
		len:      pieceSize,
		onClose:  cancel,
		pieceCid: pc,
	}).init(ctx)
	if err != nil || pr == nil { // pr == nil to make sure we don't return typed nil
		cancel()
		return nil, err
	}

	return pr, err
}

type funcCloser func() error

func (f funcCloser) Close() error {
	return f()
}

var _ io.Closer = funcCloser(nil)

// ReadPiece is used to read an Unsealed piece at the given offset and of the given size from a Sector
// If an Unsealed sector file exists with the Piece Unsealed in it, we'll use that for the read.
// Otherwise, we will Unseal a Sealed sector file for the given sector and read the Unsealed piece from it.
// If we do NOT have an existing unsealed file  containing the given piece thus causing us to schedule an Unseal,
// the returned boolean parameter will be set to true.
// If we have an existing unsealed file containing the given piece, the returned boolean will be set to false.
func (p *pieceProvider) ReadPiece(ctx context.Context, sector storiface.SectorRef, pieceOffset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealed cid.Cid) (storiface.Reader, bool, error) {
	if err := pieceOffset.Valid(); err != nil {
		return nil, false, xerrors.Errorf("pieceOffset is not valid: %w", err)
	}
	if err := size.Validate(); err != nil {
		return nil, false, xerrors.Errorf("size is not a valid piece size: %w", err)
	}

	r, err := p.tryReadUnsealedPiece(ctx, unsealed, sector, pieceOffset, size)

	if errors.Is(err, storiface.ErrSectorNotFound) {
		log.Debugf("no unsealed sector file with unsealed piece, sector=%+v, pieceOffset=%d, size=%d", sector, pieceOffset, size)
		err = nil
	}
	if err != nil {
		log.Errorf("returning error from ReadPiece:%s", err)
		return nil, false, err
	}

	var uns bool

	if r == nil {
		// a nil reader means that none of the workers has an unsealed sector file
		// containing the unsealed piece.
		// we now need to unseal a sealed sector file for the given sector to read the unsealed piece from it.
		uns = true
		commd := &unsealed
		if unsealed == cid.Undef {
			commd = nil
		}
		if err := p.uns.SectorsUnsealPiece(ctx, sector, pieceOffset, size, ticket, commd); err != nil {
			log.Errorf("failed to SectorsUnsealPiece: %s", err)
			return nil, false, xerrors.Errorf("unsealing piece: %w", err)
		}

		log.Debugf("unsealed a sector file to read the piece, sector=%+v, pieceOffset=%d, size=%d", sector, pieceOffset, size)

		r, err = p.tryReadUnsealedPiece(ctx, unsealed, sector, pieceOffset, size)
		if err != nil {
			log.Errorf("failed to tryReadUnsealedPiece after SectorsUnsealPiece: %s", err)
			return nil, true, xerrors.Errorf("read after unsealing: %w", err)
		}
		if r == nil {
			log.Errorf("got no reader after unsealing piece")
			return nil, true, xerrors.Errorf("got no reader after unsealing piece")
		}
		log.Debugf("got a reader to read unsealed piece, sector=%+v, pieceOffset=%d, size=%d", sector, pieceOffset, size)
	} else {
		log.Debugf("unsealed piece already exists, no need to unseal, sector=%+v, pieceOffset=%d, size=%d", sector, pieceOffset, size)
	}

	log.Debugf("returning reader to read unsealed piece, sector=%+v, pieceOffset=%d, size=%d", sector, pieceOffset, size)

	return r, uns, nil
}

var _ storiface.Reader = &pieceReader{}
