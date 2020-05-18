//+build cgo

package ffiwrapper

import (
	"context"
	"io"
	"io/ioutil"
	"math/bits"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/sector-storage/stores"
	"github.com/filecoin-project/sector-storage/storiface"
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
	var offset abi.UnpaddedPieceSize
	for _, size := range existingPieceSizes {
		offset += size
	}

	maxPieceSize := abi.PaddedPieceSize(sb.ssize).Unpadded()

	if offset+pieceSize > maxPieceSize {
		return abi.PieceInfo{}, xerrors.Errorf("can't add %d byte piece to sector %v with %d bytes of existing pieces", pieceSize, sector, offset)
	}

	var err error
	var done func()
	var stagedFile *partialFile

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

		stagedFile, err = createPartialFile(maxPieceSize, stagedPath.Unsealed)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("creating unsealed sector file: %w", err)
		}
	} else {
		stagedPath, done, err = sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, 0, true)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("acquire unsealed sector: %w", err)
		}

		stagedFile, err = openPartialFile(maxPieceSize, stagedPath.Unsealed)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("opening unsealed sector file: %w", err)
		}
	}

	w, err := stagedFile.Writer(UnpaddedByteIndex(offset), pieceSize)
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("getting partial file writer: %w", err)
	}
	pr := io.TeeReader(io.LimitReader(file, int64(pieceSize)), w)
	prf, werr, err := toReadableFile(pr, int64(pieceSize))
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("getting tee reader pipe: %w", err)
	}

	pieceCID, err := ffi.GeneratePieceCIDFromFile(sb.sealProofType, prf, pieceSize)
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("generating piece commitment: %w", err)
	}

	if err := stagedFile.MarkAllocated(UnpaddedByteIndex(offset), pieceSize); err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("marking data range as allocated: %w", err)
	}

	if err := stagedFile.Close(); err != nil {
		return abi.PieceInfo{}, err
	}
	stagedFile = nil

	return abi.PieceInfo{
		Size:     pieceSize.Padded(),
		PieceCID: pieceCID,
	}, werr()
}

type closerFunc func() error

func (cf closerFunc) Close() error {
	return cf()
}

func (sb *Sealer) UnsealPiece(ctx context.Context, sector abi.SectorID, offset UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd cid.Cid) error {
	maxPieceSize := abi.PaddedPieceSize(sb.ssize).Unpadded()

	// try finding existing
	unsealedPath, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, stores.FTNone, false)
	var pf *partialFile

	switch {
	case xerrors.Is(err, storiface.ErrSectorNotFound):
		unsealedPath, done, err = sb.sectors.AcquireSector(ctx, sector, stores.FTNone, stores.FTUnsealed, false)
		if err != nil {
			return xerrors.Errorf("acquire unsealed sector path (allocate): %w", err)
		}
		defer done()

		pf, err = createPartialFile(maxPieceSize, unsealedPath.Unsealed)
		if err != nil {
			return xerrors.Errorf("create unsealed file: %w", err)
		}

	case err == nil:
		defer done()

		pf, err = openPartialFile(maxPieceSize, unsealedPath.Unsealed)
		if err != nil {
			return xerrors.Errorf("opening partial file: %w", err)
		}
	default:
		return xerrors.Errorf("acquire unsealed sector path (existing): %w", err)
	}
	defer pf.Close()

	allocated, err := pf.Allocated()
	if err != nil {
		return xerrors.Errorf("getting bitruns of allocated data: %w", err)
	}

	toUnseal, err := computeUnsealRanges(allocated, offset, size)
	if err != nil {
		return xerrors.Errorf("computing unseal ranges: %w", err)
	}

	if !toUnseal.HasNext() {
		return nil
	}

	srcPaths, srcDone, err := sb.sectors.AcquireSector(ctx, sector, stores.FTCache|stores.FTSealed, stores.FTNone, false)
	if err != nil {
		return xerrors.Errorf("acquire sealed sector paths: %w", err)
	}
	defer srcDone()

	var at, nextat uint64
	for {
		piece, err := toUnseal.NextRun()
		if err != nil {
			return xerrors.Errorf("getting next range to unseal: %w", err)
		}

		at = nextat
		nextat += piece.Len

		if !piece.Val {
			continue
		}

		out, err := pf.Writer(offset, size)
		if err != nil {
			return xerrors.Errorf("getting partial file writer: %w", err)
		}

		// <eww>
		outpipe, err := ioutil.TempFile(os.TempDir(), "sector-storage-unseal-")
		if err != nil {
			return xerrors.Errorf("creating temp pipe file: %w", err)
		}
		var outpath string
		var perr error
		outWait := make(chan struct{})

		{
			outpath = outpipe.Name()
			if err := outpipe.Close(); err != nil {
				return xerrors.Errorf("close pipe temp: %w", err)
			}
			if err := os.Remove(outpath); err != nil {
				return xerrors.Errorf("rm pipe temp: %w", err)
			}

			// TODO: Make UnsealRange write to an FD
			if err := syscall.Mkfifo(outpath, 0600); err != nil {
				return xerrors.Errorf("mk temp fifo: %w", err)
			}

			go func() {
				defer close(outWait)
				defer os.Remove(outpath)

				outpipe, err = os.OpenFile(outpath, os.O_RDONLY, 0600)
				if err != nil {
					perr = xerrors.Errorf("open temp pipe: %w", err)
					return
				}
				defer outpipe.Close()

				_, perr = io.CopyN(out, outpipe, int64(size))
			}()
		}
		// </eww>

		// TODO: This may be possible to do in parallel
		err = ffi.UnsealRange(sb.sealProofType,
			srcPaths.Cache,
			srcPaths.Sealed,
			outpath,
			sector.Number,
			sector.Miner,
			randomness,
			commd,
			at,
			piece.Len)
		if err != nil {
			return xerrors.Errorf("unseal range: %w", err)
		}

		select {
		case <-outWait:
		case <-ctx.Done():
			return ctx.Err()
		}

		if perr != nil {
			return xerrors.Errorf("piping output to unsealed file: %w", perr)
		}

		if err := pf.MarkAllocated(UnpaddedByteIndex(at), abi.UnpaddedPieceSize(piece.Len)); err != nil {
			return xerrors.Errorf("marking unsealed range as allocated: %w", err)
		}

		if !toUnseal.HasNext() {
			break
		}
	}

	return nil
}

func (sb *Sealer) ReadPiece(ctx context.Context, writer io.Writer, sector abi.SectorID, offset UnpaddedByteIndex, size abi.UnpaddedPieceSize) error {
	path, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, stores.FTNone, false)
	if err != nil {
		return xerrors.Errorf("acquire unsealed sector path: %w", err)
	}
	defer done()

	maxPieceSize := abi.PaddedPieceSize(sb.ssize).Unpadded()

	pf, err := openPartialFile(maxPieceSize, path.Unsealed)
	if xerrors.Is(err, os.ErrNotExist) {
		return xerrors.Errorf("opening partial file: %w", err)
	}

	f, err := pf.Reader(offset, size)
	if err != nil {
		pf.Close()
		return xerrors.Errorf("getting partial file reader: %w", err)
	}

	if _, err := io.CopyN(writer, f, int64(size)); err != nil {
		pf.Close()
		return xerrors.Errorf("reading unsealed file: %w", err)
	}

	if err := pf.Close(); err != nil {
		return xerrors.Errorf("closing partial file: %w", err)
	}

	return nil
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

	staged := filepath.Join(paths.Cache, "staged")

	if err := sb.rewriteAsPadded(paths.Unsealed, staged); err != nil {
		return nil, xerrors.Errorf("rewriting sector as padded: %w", err)
	}
	defer func() {
		if err := os.Remove(staged); err != nil {
			log.Warnf("Removing staged sector file(%v): %s", sector, err)
		}
	}()

	// TODO: context cancellation respect
	p1o, err := ffi.SealPreCommitPhase1(
		sb.sealProofType,
		paths.Cache,
		staged,
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

func (sb *Sealer) rewriteAsPadded(unsealed string, staged string) error {
	maxPieceSize := abi.PaddedPieceSize(sb.ssize).Unpadded()

	pf, err := openPartialFile(maxPieceSize, unsealed)
	if err != nil {
		return xerrors.Errorf("opening unsealed file: %w", err)
	}

	upr, err := pf.Reader(0, maxPieceSize)
	if err != nil {
		pf.Close()
		return err
	}

	st, err := os.OpenFile(staged, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		pf.Close()
		return xerrors.Errorf("openning staged file: %w", err)
	}

	// OPTIMIZATION: upr is a file, so it could be passed straight to
	//  WriteWithAlignment IF it wouldn't care about the trailer
	lupr, werr, err := toReadableFile(io.LimitReader(upr, int64(maxPieceSize)), int64(maxPieceSize))
	if err != nil {
		return err
	}

	_, _, _, err = ffi.WriteWithAlignment(sb.sealProofType, lupr, maxPieceSize, st, nil)
	if err != nil {
		pf.Close()
		st.Close()
		return xerrors.Errorf("write with alignment: %w", err)
	}

	if err := st.Close(); err != nil {
		pf.Close()
		return err
	}

	if err := pf.Close(); err != nil {
		return err
	}

	return werr()
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
