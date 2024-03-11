package lpffi

import (
	"context"
	"io"
	"os"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func (sb *SealCalls) WritePiece(ctx context.Context, pieceID storiface.PieceNumber, size int64, data io.Reader) error {
	// todo: config(?): allow setting PathStorage for this
	// todo storage reservations
	paths, done, err := sb.sectors.AcquireSector(ctx, nil, pieceID.Ref(), storiface.FTNone, storiface.FTPiece, storiface.PathSealing)
	if err != nil {
		return err
	}
	defer done()

	dest := paths.Piece
	tempDest := dest + ".tmp"

	destFile, err := os.OpenFile(tempDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return xerrors.Errorf("creating temp piece file '%s': %w", tempDest, err)
	}

	removeTemp := true
	defer func() {
		if removeTemp {
			rerr := os.Remove(tempDest)
			if rerr != nil {
				log.Errorf("removing temp file: %+v", rerr)
			}
		}
	}()

	copyStart := time.Now()

	n, err := io.CopyBuffer(destFile, io.LimitReader(data, size), make([]byte, 8<<20))
	if err != nil {
		_ = destFile.Close()
		return xerrors.Errorf("copying piece data: %w", err)
	}

	if err := destFile.Close(); err != nil {
		return xerrors.Errorf("closing temp piece file: %w", err)
	}

	if n != size {
		return xerrors.Errorf("short write: %d", n)
	}

	copyEnd := time.Now()

	log.Infow("wrote parked piece", "piece", pieceID, "size", size, "duration", copyEnd.Sub(copyStart), "dest", dest, "MiB/s", float64(size)/(1<<20)/copyEnd.Sub(copyStart).Seconds())

	if err := os.Rename(tempDest, dest); err != nil {
		return xerrors.Errorf("rename temp piece to dest %s -> %s: %w", tempDest, dest, err)
	}

	removeTemp = false
	return nil
}

func (sb *SealCalls) PieceReader(ctx context.Context, id storiface.PieceNumber) (io.ReadCloser, error) {
	return sb.sectors.storage.ReaderSeq(ctx, id.Ref(), storiface.FTPiece)
}

func (sb *SealCalls) RemovePiece(ctx context.Context, id storiface.PieceNumber) error {
	return sb.sectors.storage.Remove(ctx, id.Ref().ID, storiface.FTPiece, true, nil)
}
