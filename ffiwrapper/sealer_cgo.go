//+build cgo

package ffiwrapper

import (
	"context"
	"io"
	"math/bits"
	"os"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/sector-storage/stores"
	"github.com/filecoin-project/sector-storage/zerocomm"
)

var _ Storage = &Sealer{}

func New(sectors SectorProvider, cfg *Config) (*Sealer, error) {
	sectorSize, err := sizeFromConfig(*cfg)
	if err != nil {
		return nil, err
	}

	sb := &Sealer{
		sealProofType: cfg.SealProofType,
		ssize:         sectorSize,

		sectors: sectors,

		stopping: make(chan struct{}),
	}

	return sb, nil
}

func (sb *Sealer) NewSector(ctx context.Context, sector abi.SectorID) error {
	// TODO: Allocate the sector here instead of in addpiece

	return nil
}

func (sb *Sealer) AddPiece(ctx context.Context, sector abi.SectorID, existingPieceSizes []abi.UnpaddedPieceSize, pieceSize abi.UnpaddedPieceSize, file storage.Data) (abi.PieceInfo, error) {
	f, werr, err := toReadableFile(file, int64(pieceSize))
	if err != nil {
		return abi.PieceInfo{}, err
	}

	var done func()
	var stagedFile *os.File

	defer func() {
		if done != nil {
			done()
		}

		if stagedFile != nil {
			if err := stagedFile.Close(); err != nil {
				log.Errorf("closing staged file: %+v", err)
			}
		}
	}()

	var stagedPath stores.SectorPaths
	if len(existingPieceSizes) == 0 {
		stagedPath, done, err = sb.sectors.AcquireSector(ctx, sector, 0, stores.FTUnsealed, true)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("acquire unsealed sector: %w", err)
		}

		stagedFile, err = os.Create(stagedPath.Unsealed)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("opening sector file: %w", err)
		}
	} else {
		stagedPath, done, err = sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, 0, true)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("acquire unsealed sector: %w", err)
		}

		stagedFile, err = os.OpenFile(stagedPath.Unsealed, os.O_RDWR, 0644)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("opening sector file: %w", err)
		}

		if _, err := stagedFile.Seek(0, io.SeekEnd); err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("seek end: %w", err)
		}
	}

	_, _, pieceCID, err := ffi.WriteWithAlignment(sb.sealProofType, f, pieceSize, stagedFile, existingPieceSizes)
	if err != nil {
		return abi.PieceInfo{}, err
	}

	if err := f.Close(); err != nil {
		return abi.PieceInfo{}, err
	}

	return abi.PieceInfo{
		Size:     pieceSize.Padded(),
		PieceCID: pieceCID,
	}, werr()
}

type closerFunc func() error

func (cf closerFunc) Close() error {
	return cf()
}

func (sb *Sealer) ReadPieceFromSealedSector(ctx context.Context, sector abi.SectorID, offset UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealedCID cid.Cid) (io.ReadCloser, error) {
	{
		path, doneUnsealed, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, stores.FTNone, false)
		if err != nil {
			return nil, xerrors.Errorf("acquire unsealed sector path: %w", err)
		}

		f, err := os.OpenFile(path.Unsealed, os.O_RDONLY, 0644)
		if err == nil {
			if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
				doneUnsealed()
				return nil, xerrors.Errorf("seek: %w", err)
			}

			lr := io.LimitReader(f, int64(size))

			return &struct {
				io.Reader
				io.Closer
			}{
				Reader: lr,
				Closer: closerFunc(func() error {
					doneUnsealed()
					return f.Close()
				}),
			}, nil
		}

		doneUnsealed()

		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	paths, doneSealed, err := sb.sectors.AcquireSector(ctx, sector, stores.FTSealed|stores.FTCache, stores.FTUnsealed, false)
	if err != nil {
		return nil, xerrors.Errorf("acquire sealed/cache sector path: %w", err)
	}
	defer doneSealed()

	// TODO: GC for those
	//  (Probably configurable count of sectors to be kept unsealed, and just
	//   remove last used one (or use whatever other cache policy makes sense))
	err = ffi.Unseal(
		sb.sealProofType,
		paths.Cache,
		paths.Sealed,
		paths.Unsealed,
		sector.Number,
		sector.Miner,
		ticket,
		unsealedCID,
	)
	if err != nil {
		return nil, xerrors.Errorf("unseal failed: %w", err)
	}

	f, err := os.OpenFile(paths.Unsealed, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}

	if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, xerrors.Errorf("seek: %w", err)
	}

	lr := io.LimitReader(f, int64(size))

	return &struct {
		io.Reader
		io.Closer
	}{
		Reader: lr,
		Closer: f,
	}, nil
}

func (sb *Sealer) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
	paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, stores.FTSealed|stores.FTCache, true)
	if err != nil {
		return nil, xerrors.Errorf("acquiring sector paths: %w", err)
	}
	defer done()

	e, err := os.OpenFile(paths.Sealed, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, xerrors.Errorf("ensuring sealed file exists: %w", err)
	}
	if err := e.Close(); err != nil {
		return nil, err
	}

	if err := os.Mkdir(paths.Cache, 0755); err != nil {
		if os.IsExist(err) {
			log.Warnf("existing cache in %s; removing", paths.Cache)

			if err := os.RemoveAll(paths.Cache); err != nil {
				return nil, xerrors.Errorf("remove existing sector cache from %s (sector %d): %w", paths.Cache, sector, err)
			}

			if err := os.Mkdir(paths.Cache, 0755); err != nil {
				return nil, xerrors.Errorf("mkdir cache path after cleanup: %w", err)
			}
		} else {
			return nil, err
		}
	}

	var sum abi.UnpaddedPieceSize
	for _, piece := range pieces {
		sum += piece.Size.Unpadded()
	}
	ussize := abi.PaddedPieceSize(sb.ssize).Unpadded()
	if sum != ussize {
		return nil, xerrors.Errorf("aggregated piece sizes don't match sector size: %d != %d (%d)", sum, ussize, int64(ussize-sum))
	}

	// TODO: context cancellation respect
	p1o, err := ffi.SealPreCommitPhase1(
		sb.sealProofType,
		paths.Cache,
		paths.Unsealed,
		paths.Sealed,
		sector.Number,
		sector.Miner,
		ticket,
		pieces,
	)
	if err != nil {
		return nil, xerrors.Errorf("presealing sector %d (%s): %w", sector.Number, paths.Unsealed, err)
	}
	return p1o, nil
}

func (sb *Sealer) SealPreCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage.PreCommit1Out) (storage.SectorCids, error) {
	paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTSealed|stores.FTCache, 0, true)
	if err != nil {
		return storage.SectorCids{}, xerrors.Errorf("acquiring sector paths: %w", err)
	}
	defer done()

	sealedCID, unsealedCID, err := ffi.SealPreCommitPhase2(phase1Out, paths.Cache, paths.Sealed)
	if err != nil {
		return storage.SectorCids{}, xerrors.Errorf("presealing sector %d (%s): %w", sector.Number, paths.Unsealed, err)
	}

	return storage.SectorCids{
		Unsealed: unsealedCID,
		Sealed:   sealedCID,
	}, nil
}

func (sb *Sealer) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (storage.Commit1Out, error) {
	paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTSealed|stores.FTCache, 0, true)
	if err != nil {
		return nil, xerrors.Errorf("acquire sector paths: %w", err)
	}
	defer done()
	output, err := ffi.SealCommitPhase1(
		sb.sealProofType,
		cids.Sealed,
		cids.Unsealed,
		paths.Cache,
		paths.Sealed,
		sector.Number,
		sector.Miner,
		ticket,
		seed,
		pieces,
	)
	if err != nil {
		log.Warn("StandaloneSealCommit error: ", err)
		log.Warnf("num:%d tkt:%v seed:%v, pi:%v sealedCID:%v, unsealedCID:%v", sector.Number, ticket, seed, pieces, cids.Sealed, cids.Unsealed)

		return nil, xerrors.Errorf("StandaloneSealCommit: %w", err)
	}
	return output, nil
}

func (sb *Sealer) SealCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage.Commit1Out) (storage.Proof, error) {
	return ffi.SealCommitPhase2(phase1Out, sector.Number, sector.Miner)
}

func (sb *Sealer) FinalizeSector(ctx context.Context, sector abi.SectorID) error {
	paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTCache, 0, false)
	if err != nil {
		return xerrors.Errorf("acquiring sector cache path: %w", err)
	}
	defer done()

	return ffi.ClearCache(uint64(sb.ssize), paths.Cache)
}

func GeneratePieceCIDFromFile(proofType abi.RegisteredProof, piece io.Reader, pieceSize abi.UnpaddedPieceSize) (cid.Cid, error) {
	f, werr, err := toReadableFile(piece, int64(pieceSize))
	if err != nil {
		return cid.Undef, err
	}

	pieceCID, err := ffi.GeneratePieceCIDFromFile(proofType, f, pieceSize)
	if err != nil {
		return cid.Undef, err
	}

	return pieceCID, werr()
}

func GenerateUnsealedCID(proofType abi.RegisteredProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	var sum abi.PaddedPieceSize
	for _, p := range pieces {
		sum += p.Size
	}

	ssize, err := proofType.SectorSize()
	if err != nil {
		return cid.Undef, err
	}

	{
		// pad remaining space with 0 CommPs
		toFill := uint64(abi.PaddedPieceSize(ssize) - sum)
		n := bits.OnesCount64(toFill)
		for i := 0; i < n; i++ {
			next := bits.TrailingZeros64(toFill)
			psize := uint64(1) << uint(next)
			toFill ^= psize

			unpadded := abi.PaddedPieceSize(psize).Unpadded()
			pieces = append(pieces, abi.PieceInfo{
				Size:     unpadded.Padded(),
				PieceCID: zerocomm.ZeroPieceCommitment(unpadded),
			})
		}
	}

	return ffi.GenerateUnsealedCID(proofType, pieces)
}
