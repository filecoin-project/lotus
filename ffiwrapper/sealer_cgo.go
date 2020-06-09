//+build cgo

package ffiwrapper

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"math/bits"
	"os"
	"runtime"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/sector-storage/fr32"
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

	maxPieceSize := abi.PaddedPieceSize(sb.ssize)

	if offset.Padded()+pieceSize.Padded() > maxPieceSize {
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

	w, err := stagedFile.Writer(storiface.UnpaddedByteIndex(offset).Padded(), pieceSize.Padded())
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("getting partial file writer: %w", err)
	}

	pw := fr32.NewPadWriter(w)

	pr := io.TeeReader(io.LimitReader(file, int64(pieceSize)), pw)

	chunk := abi.PaddedPieceSize(4 << 20)

	buf := make([]byte, chunk.Unpadded())
	var pieceCids []abi.PieceInfo

	for {
		var read int
		for rbuf := buf; len(rbuf) > 0; {
			n, err := pr.Read(rbuf)
			if err != nil && err != io.EOF {
				return abi.PieceInfo{}, xerrors.Errorf("pr read error: %w", err)
			}

			rbuf = rbuf[n:]
			read += n

			if err == io.EOF {
				break
			}
		}
		if read == 0 {
			break
		}

		c, err := sb.pieceCid(buf[:read])
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("pieceCid error: %w", err)
		}
		pieceCids = append(pieceCids, abi.PieceInfo{
			Size:     abi.UnpaddedPieceSize(len(buf[:read])).Padded(),
			PieceCID: c,
		})
	}

	if err := pw.Close(); err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("closing padded writer: %w", err)
	}

	if err := stagedFile.MarkAllocated(storiface.UnpaddedByteIndex(offset).Padded(), pieceSize.Padded()); err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("marking data range as allocated: %w", err)
	}

	if err := stagedFile.Close(); err != nil {
		return abi.PieceInfo{}, err
	}
	stagedFile = nil

	if len(pieceCids) == 1 {
		return pieceCids[0], nil
	}

	pieceCID, err := ffi.GenerateUnsealedCID(sb.sealProofType, pieceCids)
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("generate unsealed CID: %w", err)
	}

	commp, err := commcid.CIDToDataCommitmentV1(pieceCID)
	if err != nil {
		return abi.PieceInfo{}, err
	}

	return abi.PieceInfo{
		Size:     pieceSize.Padded(),
		PieceCID: commcid.PieceCommitmentV1ToCID(commp),
	}, nil
}

func (sb *Sealer) pieceCid(in []byte) (cid.Cid, error) {
	prf, werr, err := ToReadableFile(bytes.NewReader(in), int64(len(in)))
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting tee reader pipe: %w", err)
	}

	pieceCID, err := ffi.GeneratePieceCIDFromFile(sb.sealProofType, prf, abi.UnpaddedPieceSize(len(in)))
	if err != nil {
		return cid.Undef, xerrors.Errorf("generating piece commitment: %w", err)
	}

	prf.Close()

	return pieceCID, werr()
}

func (sb *Sealer) UnsealPiece(ctx context.Context, sector abi.SectorID, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd cid.Cid) error {
	maxPieceSize := abi.PaddedPieceSize(sb.ssize)

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

	sealed, err := os.OpenFile(srcPaths.Sealed, os.O_RDONLY, 0644)
	if err != nil {
		return xerrors.Errorf("opening sealed file: %w", err)
	}
	defer sealed.Close()

	var at, nextat abi.PaddedPieceSize
	for {
		piece, err := toUnseal.NextRun()
		if err != nil {
			return xerrors.Errorf("getting next range to unseal: %w", err)
		}

		at = nextat
		nextat += abi.PaddedPieceSize(piece.Len)

		if !piece.Val {
			continue
		}

		out, err := pf.Writer(offset.Padded(), size.Padded())
		if err != nil {
			return xerrors.Errorf("getting partial file writer: %w", err)
		}

		// <eww>
		opr, opw, err := os.Pipe()
		if err != nil {
			return xerrors.Errorf("creating out pipe: %w", err)
		}

		var perr error
		outWait := make(chan struct{})

		{
			go func() {
				defer close(outWait)
				defer opr.Close()

				padwriter := fr32.NewPadWriter(out)
				if err != nil {
					perr = xerrors.Errorf("creating new padded writer: %w", err)
					return
				}

				bsize := uint64(size.Padded())
				if bsize > uint64(runtime.NumCPU())*fr32.MTTresh {
					bsize = uint64(runtime.NumCPU()) * fr32.MTTresh
				}

				bw := bufio.NewWriterSize(padwriter, int(abi.PaddedPieceSize(bsize).Unpadded()))

				_, err = io.CopyN(bw, opr, int64(size))
				if err != nil {
					perr = xerrors.Errorf("copying data: %w", err)
					return
				}

				if err := bw.Flush(); err != nil {
					perr = xerrors.Errorf("flushing unpadded data: %w", err)
					return
				}

				if err := padwriter.Close(); err != nil {
					perr = xerrors.Errorf("closing padwriter: %w", err)
					return
				}
			}()
		}
		// </eww>

		// TODO: This may be possible to do in parallel
		err = ffi.UnsealRange(sb.sealProofType,
			srcPaths.Cache,
			sealed,
			opw,
			sector.Number,
			sector.Miner,
			randomness,
			commd,
			uint64(at.Unpadded()),
			uint64(abi.PaddedPieceSize(piece.Len).Unpadded()))

		_ = opr.Close()

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

		if err := pf.MarkAllocated(storiface.PaddedByteIndex(at), abi.PaddedPieceSize(piece.Len)); err != nil {
			return xerrors.Errorf("marking unsealed range as allocated: %w", err)
		}

		if !toUnseal.HasNext() {
			break
		}
	}

	return nil
}

func (sb *Sealer) ReadPiece(ctx context.Context, writer io.Writer, sector abi.SectorID, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) error {
	path, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, stores.FTNone, false)
	if err != nil {
		return xerrors.Errorf("acquire unsealed sector path: %w", err)
	}
	defer done()

	maxPieceSize := abi.PaddedPieceSize(sb.ssize)

	pf, err := openPartialFile(maxPieceSize, path.Unsealed)
	if xerrors.Is(err, os.ErrNotExist) {
		return xerrors.Errorf("opening partial file: %w", err)
	}

	f, err := pf.Reader(offset.Padded(), size.Padded())
	if err != nil {
		pf.Close()
		return xerrors.Errorf("getting partial file reader: %w", err)
	}

	upr, err := fr32.NewUnpadReader(f, size.Padded())
	if err != nil {
		return xerrors.Errorf("creating unpadded reader: %w", err)
	}

	if _, err := io.CopyN(writer, upr, int64(size)); err != nil {
		pf.Close()
		return xerrors.Errorf("reading unsealed file: %w", err)
	}

	if err := pf.Close(); err != nil {
		return xerrors.Errorf("closing partial file: %w", err)
	}

	return nil
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
	f, werr, err := ToReadableFile(piece, int64(pieceSize))
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
