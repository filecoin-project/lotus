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
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/fr32"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/extern/sector-storage/zerocomm"
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
		stagedPath, done, err = sb.sectors.AcquireSector(ctx, sector, 0, stores.FTUnsealed, stores.PathSealing)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("acquire unsealed sector: %w", err)
		}

		stagedFile, err = createPartialFile(maxPieceSize, stagedPath.Unsealed)
		if err != nil {
			return abi.PieceInfo{}, xerrors.Errorf("creating unsealed sector file: %w", err)
		}
	} else {
		stagedPath, done, err = sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, 0, stores.PathSealing)
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

	// validate that the pieceCID was properly formed
	if _, err := commcid.CIDToPieceCommitmentV1(pieceCID); err != nil {
		return abi.PieceInfo{}, err
	}

	return abi.PieceInfo{
		Size:     pieceSize.Padded(),
		PieceCID: pieceCID,
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

	_ = prf.Close()

	return pieceCID, werr()
}

func (sb *Sealer) UnsealPiece(ctx context.Context, sector abi.SectorID, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd cid.Cid) error {
	maxPieceSize := abi.PaddedPieceSize(sb.ssize)

	// try finding existing
	unsealedPath, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, stores.FTNone, stores.PathStorage)
	var pf *partialFile

	switch {
	case xerrors.Is(err, storiface.ErrSectorNotFound):
		unsealedPath, done, err = sb.sectors.AcquireSector(ctx, sector, stores.FTNone, stores.FTUnsealed, stores.PathStorage)
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
	defer pf.Close() // nolint

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

	srcPaths, srcDone, err := sb.sectors.AcquireSector(ctx, sector, stores.FTCache|stores.FTSealed, stores.FTNone, stores.PathStorage)
	if err != nil {
		return xerrors.Errorf("acquire sealed sector paths: %w", err)
	}
	defer srcDone()

	sealed, err := os.OpenFile(srcPaths.Sealed, os.O_RDONLY, 0644) // nolint:gosec
	if err != nil {
		return xerrors.Errorf("opening sealed file: %w", err)
	}
	defer sealed.Close() // nolint

	var at, nextat abi.PaddedPieceSize
	first := true
	for first || toUnseal.HasNext() {
		first = false

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
				defer opr.Close() // nolint

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

		_ = opw.Close()

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

func (sb *Sealer) ReadPiece(ctx context.Context, writer io.Writer, sector abi.SectorID, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error) {
	path, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, stores.FTNone, stores.PathStorage)
	if err != nil {
		return false, xerrors.Errorf("acquire unsealed sector path: %w", err)
	}
	defer done()

	maxPieceSize := abi.PaddedPieceSize(sb.ssize)

	pf, err := openPartialFile(maxPieceSize, path.Unsealed)
	if xerrors.Is(err, os.ErrNotExist) {
		return false, xerrors.Errorf("opening partial file: %w", err)
	}

	ok, err := pf.HasAllocated(offset, size)
	if err != nil {
		_ = pf.Close()
		return false, err
	}

	if !ok {
		_ = pf.Close()
		return false, nil
	}

	f, err := pf.Reader(offset.Padded(), size.Padded())
	if err != nil {
		_ = pf.Close()
		return false, xerrors.Errorf("getting partial file reader: %w", err)
	}

	upr, err := fr32.NewUnpadReader(f, size.Padded())
	if err != nil {
		return false, xerrors.Errorf("creating unpadded reader: %w", err)
	}

	if _, err := io.CopyN(writer, upr, int64(size)); err != nil {
		_ = pf.Close()
		return false, xerrors.Errorf("reading unsealed file: %w", err)
	}

	if err := pf.Close(); err != nil {
		return false, xerrors.Errorf("closing partial file: %w", err)
	}

	return false, nil
}

func (sb *Sealer) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
	paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, stores.FTSealed|stores.FTCache, stores.PathSealing)
	if err != nil {
		return nil, xerrors.Errorf("acquiring sector paths: %w", err)
	}
	defer done()

	e, err := os.OpenFile(paths.Sealed, os.O_RDWR|os.O_CREATE, 0644) // nolint:gosec
	if err != nil {
		return nil, xerrors.Errorf("ensuring sealed file exists: %w", err)
	}
	if err := e.Close(); err != nil {
		return nil, err
	}

	if err := os.Mkdir(paths.Cache, 0755); err != nil { // nolint
		if os.IsExist(err) {
			log.Warnf("existing cache in %s; removing", paths.Cache)

			if err := os.RemoveAll(paths.Cache); err != nil {
				return nil, xerrors.Errorf("remove existing sector cache from %s (sector %d): %w", paths.Cache, sector, err)
			}

			if err := os.Mkdir(paths.Cache, 0755); err != nil { // nolint:gosec
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
	paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTSealed|stores.FTCache, 0, stores.PathSealing)
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
	paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTSealed|stores.FTCache, 0, stores.PathSealing)
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

func (sb *Sealer) FinalizeSector(ctx context.Context, sector abi.SectorID, keepUnsealed []storage.Range) error {
	if len(keepUnsealed) > 0 {
		maxPieceSize := abi.PaddedPieceSize(sb.ssize)

		sr := pieceRun(0, maxPieceSize)

		for _, s := range keepUnsealed {
			si := &rlepluslazy.RunSliceIterator{}
			if s.Offset != 0 {
				si.Runs = append(si.Runs, rlepluslazy.Run{Val: false, Len: uint64(s.Offset)})
			}
			si.Runs = append(si.Runs, rlepluslazy.Run{Val: true, Len: uint64(s.Size)})

			var err error
			sr, err = rlepluslazy.Subtract(sr, si)
			if err != nil {
				return err
			}
		}

		paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTUnsealed, 0, stores.PathStorage)
		if err != nil {
			return xerrors.Errorf("acquiring sector cache path: %w", err)
		}
		defer done()

		pf, err := openPartialFile(maxPieceSize, paths.Unsealed)
		if xerrors.Is(err, os.ErrNotExist) {
			return xerrors.Errorf("opening partial file: %w", err)
		}

		var at uint64
		for sr.HasNext() {
			r, err := sr.NextRun()
			if err != nil {
				_ = pf.Close()
				return err
			}

			offset := at
			at += r.Len
			if !r.Val {
				continue
			}

			err = pf.Free(storiface.PaddedByteIndex(abi.UnpaddedPieceSize(offset).Padded()), abi.UnpaddedPieceSize(r.Len).Padded())
			if err != nil {
				_ = pf.Close()
				return xerrors.Errorf("free partial file range: %w", err)
			}
		}

		if err := pf.Close(); err != nil {
			return err
		}
	}

	paths, done, err := sb.sectors.AcquireSector(ctx, sector, stores.FTCache, 0, stores.PathStorage)
	if err != nil {
		return xerrors.Errorf("acquiring sector cache path: %w", err)
	}
	defer done()

	return ffi.ClearCache(uint64(sb.ssize), paths.Cache)
}

func (sb *Sealer) ReleaseUnsealed(ctx context.Context, sector abi.SectorID, safeToFree []storage.Range) error {
	// This call is meant to mark storage as 'freeable'. Given that unsealing is
	// very expensive, we don't remove data as soon as we can - instead we only
	// do that when we don't have free space for data that really needs it

	// This function should not be called at this layer, everything should be
	// handled in localworker
	return xerrors.Errorf("not supported at this layer")
}

func (sb *Sealer) Remove(ctx context.Context, sector abi.SectorID) error {
	return xerrors.Errorf("not supported at this layer") // happens in localworker
}

func GeneratePieceCIDFromFile(proofType abi.RegisteredSealProof, piece io.Reader, pieceSize abi.UnpaddedPieceSize) (cid.Cid, error) {
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

func GetRequiredPadding(oldLength abi.PaddedPieceSize, newPieceLength abi.PaddedPieceSize) ([]abi.PaddedPieceSize, abi.PaddedPieceSize) {

	padPieces := make([]abi.PaddedPieceSize, 0)

	toFill := uint64(-oldLength % newPieceLength)

	n := bits.OnesCount64(toFill)
	var sum abi.PaddedPieceSize
	for i := 0; i < n; i++ {
		next := bits.TrailingZeros64(toFill)
		psize := uint64(1) << uint(next)
		toFill ^= psize

		padded := abi.PaddedPieceSize(psize)
		padPieces = append(padPieces, padded)
		sum += padded
	}

	return padPieces, sum
}

func GenerateUnsealedCID(proofType abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	ssize, err := proofType.SectorSize()
	if err != nil {
		return cid.Undef, err
	}

	pssize := abi.PaddedPieceSize(ssize)
	allPieces := make([]abi.PieceInfo, 0, len(pieces))
	if len(pieces) == 0 {
		allPieces = append(allPieces, abi.PieceInfo{
			Size:     pssize,
			PieceCID: zerocomm.ZeroPieceCommitment(pssize.Unpadded()),
		})
	} else {
		var sum abi.PaddedPieceSize

		padTo := func(pads []abi.PaddedPieceSize) {
			for _, p := range pads {
				allPieces = append(allPieces, abi.PieceInfo{
					Size:     p,
					PieceCID: zerocomm.ZeroPieceCommitment(p.Unpadded()),
				})

				sum += p
			}
		}

		for _, p := range pieces {
			ps, _ := GetRequiredPadding(sum, p.Size)
			padTo(ps)

			allPieces = append(allPieces, p)
			sum += p.Size
		}

		ps, _ := GetRequiredPadding(sum, pssize)
		padTo(ps)
	}

	return ffi.GenerateUnsealedCID(proofType, allPieces)
}
