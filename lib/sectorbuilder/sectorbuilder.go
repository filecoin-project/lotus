package sectorbuilder

import (
	"context"
	"encoding/binary"
	"io"
	"unsafe"

	"golang.org/x/xerrors"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"

	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/lib/bytesink"
)

var log = logging.Logger("sectorbuilder")

type SectorSealingStatus = sectorbuilder.SectorSealingStatus

type StagedSectorMetadata = sectorbuilder.StagedSectorMetadata

const CommLen = sectorbuilder.CommitmentBytesLen

type SectorBuilder struct {
	handle unsafe.Pointer

	sschan chan SectorSealingStatus
}

type SectorBuilderConfig struct {
	SectorSize  uint64
	Miner       address.Address
	SealedDir   string
	StagedDir   string
	MetadataDir string
}

func New(cfg *SectorBuilderConfig) (*SectorBuilder, error) {
	proverId := addressToProverID(cfg.Miner)

	sbp, err := sectorbuilder.InitSectorBuilder(cfg.SectorSize, 1, 1, 1, cfg.MetadataDir, proverId, cfg.SealedDir, cfg.StagedDir, 16)
	if err != nil {
		return nil, err
	}

	return &SectorBuilder{
		handle: sbp,
		sschan: make(chan SectorSealingStatus, 32),
	}, nil
}

func addressToProverID(a address.Address) [31]byte {
	var proverId [31]byte
	copy(proverId[:], a.Payload())
	return proverId
}

func sectorIDtoBytes(sid uint64) [31]byte {
	var out [31]byte
	binary.LittleEndian.PutUint64(out[:], sid)
	return out
}

func (sb *SectorBuilder) Run(ctx context.Context) {
	go sb.pollForSealedSectors(ctx)
}

func (sb *SectorBuilder) Destroy() {
	sectorbuilder.DestroySectorBuilder(sb.handle)
}

func (sb *SectorBuilder) AddPiece(ctx context.Context, pieceRef string, pieceSize uint64, pieceReader io.ReadCloser) (uint64, error) {
	fifoFile, err := bytesink.NewFifo()
	if err != nil {
		return 0, err
	}

	// errCh holds any error encountered when streaming bytes or making the CGO
	// call. The channel is buffered so that the goroutines can exit, which will
	// close the pipe, which unblocks the CGO call.
	errCh := make(chan error, 2)

	// sectorIDCh receives a value if the CGO call indicates that the client
	// piece has successfully been added to a sector. The channel is buffered
	// so that the goroutine can exit if a value is sent to errCh before the
	// CGO call completes.
	sectorIDCh := make(chan uint64, 1)

	// goroutine attempts to copy bytes from piece's reader to the fifoFile
	go func() {
		// opening the fifoFile blocks the goroutine until a reader is opened on the
		// other end of the FIFO pipe
		err := fifoFile.Open()
		if err != nil {
			errCh <- xerrors.Errorf("failed to open fifoFile: %w", err)
			return
		}

		// closing theg s fifoFile signals to the reader that we're done writing, which
		// unblocks the reader
		defer func() {
			err := fifoFile.Close()
			if err != nil {
				log.Warnf("failed to close fifoFile: %s", err)
			}
		}()

		n, err := io.Copy(fifoFile, pieceReader)
		if err != nil {
			errCh <- xerrors.Errorf("failed to copy to pipe: %w", err)
			return
		}

		if uint64(n) != pieceSize {
			errCh <- xerrors.Errorf("expected to write %d bytes but wrote %d", pieceSize, n)
			return
		}
	}()

	// goroutine makes CGO call, which blocks until FIFO pipe opened for writing
	// from within other goroutine
	go func() {
		id, err := sectorbuilder.AddPiece(sb.handle, pieceRef, pieceSize, fifoFile.ID())
		if err != nil {
			msg := "CGO add_piece returned an error (err=%s, fifo path=%s)"
			log.Errorf(msg, err, fifoFile.ID())
			errCh <- err
			return
		}

		sectorIDCh <- id
	}()

	select {
	case <-ctx.Done():
		errStr := "context completed before CGO call could return"
		strFmt := "%s (sinkPath=%s)"
		log.Errorf(strFmt, errStr, fifoFile.ID())

		return 0, xerrors.New(errStr)
	case err := <-errCh:
		errStr := "error streaming piece-bytes"
		strFmt := "%s (sinkPath=%s)"
		log.Errorf(strFmt, errStr, fifoFile.ID())

		return 0, xerrors.Errorf("%w: %s", errStr, err)
	case sectorID := <-sectorIDCh:
		return sectorID, nil
	}
}

// TODO: should *really really* return an io.ReadCloser
func (sb *SectorBuilder) ReadPieceFromSealedSector(pieceKey string) ([]byte, error) {
	return sectorbuilder.ReadPieceFromSealedSector(sb.handle, pieceKey)
}

func (sb *SectorBuilder) SealAllStagedSectors() error {
	return sectorbuilder.SealAllStagedSectors(sb.handle)
}

func (sb *SectorBuilder) SealStatus(sector uint64) (SectorSealingStatus, error) {
	return sectorbuilder.GetSectorSealingStatusByID(sb.handle, sector)
}

func (sb *SectorBuilder) GetAllStagedSectors() ([]StagedSectorMetadata, error) {
	return sectorbuilder.GetAllStagedSectors(sb.handle)
}

func (sb *SectorBuilder) GeneratePoSt(sortedCommRs [][CommLen]byte, challengeSeed [CommLen]byte) ([][]byte, []uint64, error) {
	// Wait, this is a blocking method with no way of interrupting it?
	// does it checkpoint itself?
	return sectorbuilder.GeneratePoSt(sb.handle, sortedCommRs, challengeSeed)
}

func (sb *SectorBuilder) SealedSectorChan() <-chan SectorSealingStatus {
	// is this ever going to be multi-consumer? If so, switch to using pubsub/eventbus
	return sb.sschan
}

var UserBytesForSectorSize = sectorbuilder.GetMaxUserBytesPerStagedSector

func VerifySeal(sectorSize uint64, commR, commD, commRStar []byte, proverID address.Address, sectorID uint64, proof []byte) (bool, error) {
	var commRa, commDa, commRStara [32]byte
	copy(commRa[:], commR)
	copy(commDa[:], commD)
	copy(commRStara[:], commRStar)
	proverIDa := addressToProverID(proverID)
	sectorIDa := sectorIDtoBytes(sectorID)

	return sectorbuilder.VerifySeal(sectorSize, commRa, commDa, commRStara, proverIDa, sectorIDa, proof)
}

func VerifyPost(sectorSize uint64, sortedCommRs [][CommLen]byte, challengeSeed [CommLen]byte, proofs [][]byte, faults []uint64) (bool, error) {
	// sectorbuilder.VerifyPost()
	panic("no")
}
