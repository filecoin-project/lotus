package sectorbuilder

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"unsafe"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

const PoStReservedWorkers = 1
const PoRepProofPartitions = 2

var lastSectorIdKey = datastore.NewKey("/sectorbuilder/last")

var log = logging.Logger("sectorbuilder")

type SectorSealingStatus = sectorbuilder.SectorSealingStatus

type StagedSectorMetadata = sectorbuilder.StagedSectorMetadata

type SortedSectorInfo = sectorbuilder.SortedSectorInfo

type SectorInfo = sectorbuilder.SectorInfo

type SealTicket = sectorbuilder.SealTicket

type SealSeed = sectorbuilder.SealSeed

type SealPreCommitOutput = sectorbuilder.SealPreCommitOutput

type SealCommitOutput = sectorbuilder.SealCommitOutput

type PublicPieceInfo = sectorbuilder.PublicPieceInfo

type RawSealPreCommitOutput = sectorbuilder.RawSealPreCommitOutput

const CommLen = sectorbuilder.CommitmentBytesLen

type SectorBuilder struct {
	handle unsafe.Pointer
	ds     dtypes.MetadataDS
	idLk   sync.Mutex

	ssize uint64

	Miner address.Address

	stagedDir string
	sealedDir string
	cacheDir  string

	rateLimit chan struct{}
}

type Config struct {
	SectorSize uint64
	Miner      address.Address

	WorkerThreads uint8

	CacheDir    string
	SealedDir   string
	StagedDir   string
	MetadataDir string
}

func New(cfg *Config, ds dtypes.MetadataDS) (*SectorBuilder, error) {
	if cfg.WorkerThreads <= PoStReservedWorkers {
		return nil, xerrors.Errorf("minimum worker threads is %d, specified %d", PoStReservedWorkers+1, cfg.WorkerThreads)
	}

	proverId := addressToProverID(cfg.Miner)

	for _, dir := range []string{cfg.StagedDir, cfg.SealedDir, cfg.CacheDir, cfg.MetadataDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			if os.IsExist(err) {
				continue
			}
			return nil, err
		}
	}

	var lastUsedID uint64
	b, err := ds.Get(lastSectorIdKey)
	switch err {
	case nil:
		i, err := strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			return nil, err
		}
		lastUsedID = uint64(i)
	case datastore.ErrNotFound:
	default:
		return nil, err
	}

	sbp, err := sectorbuilder.InitSectorBuilder(cfg.SectorSize, PoRepProofPartitions, lastUsedID, cfg.MetadataDir, proverId, cfg.SealedDir, cfg.StagedDir, cfg.CacheDir, 16, cfg.WorkerThreads)
	if err != nil {
		return nil, err
	}

	sb := &SectorBuilder{
		handle: sbp,
		ds:     ds,

		ssize: cfg.SectorSize,

		stagedDir: cfg.StagedDir,
		sealedDir: cfg.SealedDir,
		cacheDir:  cfg.CacheDir,

		Miner:     cfg.Miner,
		rateLimit: make(chan struct{}, cfg.WorkerThreads-PoStReservedWorkers),
	}

	return sb, nil
}

func (sb *SectorBuilder) RateLimit() func() {
	if cap(sb.rateLimit) == len(sb.rateLimit) {
		log.Warn("rate-limiting sectorbuilder call")
	}
	sb.rateLimit <- struct{}{}

	return func() {
		<-sb.rateLimit
	}
}

func (sb *SectorBuilder) WorkerStats() (free, reserved, total int) {
	return cap(sb.rateLimit) - len(sb.rateLimit), PoStReservedWorkers, cap(sb.rateLimit) + PoStReservedWorkers
}

func addressToProverID(a address.Address) [32]byte {
	var proverId [32]byte
	copy(proverId[:], a.Payload())
	return proverId
}

func (sb *SectorBuilder) Destroy() {
	sectorbuilder.DestroySectorBuilder(sb.handle)
}

func (sb *SectorBuilder) AcquireSectorId() (uint64, error) {
	sb.idLk.Lock()
	defer sb.idLk.Unlock()

	id, err := sectorbuilder.AcquireSectorId(sb.handle)
	if err != nil {
		return 0, err
	}
	err = sb.ds.Put(lastSectorIdKey, []byte(fmt.Sprint(id)))
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (sb *SectorBuilder) AddPiece(pieceSize uint64, sectorId uint64, file io.Reader, existingPieceSizes []uint64) (PublicPieceInfo, error) {
	ret := sb.RateLimit()
	defer ret()

	f, werr, err := toReadableFile(file, int64(pieceSize))
	if err != nil {
		return PublicPieceInfo{}, err
	}

	stagedFile, err := sb.stagedSectorFile(sectorId)
	if err != nil {
		return PublicPieceInfo{}, err
	}

	_, _, commP, err := sectorbuilder.StandaloneWriteWithAlignment(f, pieceSize, stagedFile, existingPieceSizes)
	if err != nil {
		return PublicPieceInfo{}, err
	}

	if err := stagedFile.Close(); err != nil {
		return PublicPieceInfo{}, err
	}

	if err := f.Close(); err != nil {
		return PublicPieceInfo{}, err
	}

	return PublicPieceInfo{
		Size:  pieceSize,
		CommP: commP,
	}, werr()
}

// TODO: should *really really* return an io.ReadCloser
func (sb *SectorBuilder) ReadPieceFromSealedSector(pieceKey string) ([]byte, error) {
	ret := sb.RateLimit()
	defer ret()

	return sectorbuilder.ReadPieceFromSealedSector(sb.handle, pieceKey)
}

func (sb *SectorBuilder) SealPreCommit(sectorID uint64, ticket SealTicket, pieces []PublicPieceInfo) (RawSealPreCommitOutput, error) {
	ret := sb.RateLimit()
	defer ret()

	cacheDir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return RawSealPreCommitOutput{}, err
	}

	sealedPath, err := sb.sealedSectorPath(sectorID)
	if err != nil {
		return RawSealPreCommitOutput{}, err
	}

	var sum uint64
	for _, piece := range pieces {
		sum += piece.Size
	}
	ussize := UserBytesForSectorSize(sb.ssize)
	if sum != ussize {
		return RawSealPreCommitOutput{}, xerrors.Errorf("aggregated piece sizes don't match sector size: %d != %d (%d)", sum, ussize, int64(ussize-sum))
	}

	stagedPath := sb.stagedSectorPath(sectorID)

	rspco, err := sectorbuilder.StandaloneSealPreCommit(
		sb.ssize,
		PoRepProofPartitions,
		cacheDir,
		stagedPath,
		sealedPath,
		sectorID,
		addressToProverID(sb.Miner),
		ticket.TicketBytes,
		pieces,
	)
	if err != nil {
		return RawSealPreCommitOutput{}, xerrors.Errorf("presealing sector %d (%s): %w", sectorID, stagedPath, err)
	}

	return rspco, nil
}

func (sb *SectorBuilder) SealCommit(sectorID uint64, ticket SealTicket, seed SealSeed, pieces []PublicPieceInfo, pieceKeys []string, rspco RawSealPreCommitOutput) (proof []byte, err error) {
	ret := sb.RateLimit()
	defer ret()

	cacheDir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return nil, err
	}

	proof, err = sectorbuilder.StandaloneSealCommit(
		sb.ssize,
		PoRepProofPartitions,
		cacheDir,
		sectorID,
		addressToProverID(sb.Miner),
		ticket.TicketBytes,
		seed.TicketBytes,
		pieces,
		rspco,
	)
	if err != nil {
		return nil, xerrors.Errorf("StandaloneSealCommit: %w", err)
	}

	pmeta := make([]sectorbuilder.PieceMetadata, len(pieces))
	for i, piece := range pieces {
		pmeta[i] = sectorbuilder.PieceMetadata{
			Key:   pieceKeys[i],
			Size:  piece.Size,
			CommP: piece.CommP,
		}
	}

	sealedPath, err := sb.sealedSectorPath(sectorID)
	if err != nil {
		return nil, err
	}

	err = sectorbuilder.ImportSealedSector(
		sb.handle,
		sectorID,
		cacheDir,
		sealedPath,
		ticket,
		seed,
		rspco.CommR,
		rspco.CommD,
		rspco.CommC,
		rspco.CommRLast,
		proof,
		pmeta,
	)
	if err != nil {
		return nil, xerrors.Errorf("ImportSealedSector: %w", err)
	}
	return proof, nil
}

func (sb *SectorBuilder) GeneratePoSt(sectorInfo SortedSectorInfo, challengeSeed [CommLen]byte, faults []uint64) ([]byte, error) {
	// Wait, this is a blocking method with no way of interrupting it?
	// does it checkpoint itself?
	return sectorbuilder.GeneratePoSt(sb.handle, sectorInfo, challengeSeed, faults)
}

func (sb *SectorBuilder) SectorSize() uint64 {
	return sb.ssize
}

var UserBytesForSectorSize = sectorbuilder.GetMaxUserBytesPerStagedSector

func VerifySeal(sectorSize uint64, commR, commD []byte, proverID address.Address, ticket []byte, seed []byte, sectorID uint64, proof []byte) (bool, error) {
	var commRa, commDa, ticketa, seeda [32]byte
	copy(commRa[:], commR)
	copy(commDa[:], commD)
	copy(ticketa[:], ticket)
	copy(seeda[:], seed)
	proverIDa := addressToProverID(proverID)

	return sectorbuilder.VerifySeal(sectorSize, commRa, commDa, proverIDa, ticketa, seeda, sectorID, proof)
}

func NewSortedSectorInfo(sectors []SectorInfo) SortedSectorInfo {
	return sectorbuilder.NewSortedSectorInfo(sectors...)
}

func VerifyPost(ctx context.Context, sectorSize uint64, sectorInfo SortedSectorInfo, challengeSeed [CommLen]byte, proof []byte, faults []uint64) (bool, error) {
	_, span := trace.StartSpan(ctx, "VerifyPoSt")
	defer span.End()
	return sectorbuilder.VerifyPoSt(sectorSize, sectorInfo, challengeSeed, proof, faults)
}

func GeneratePieceCommitment(piece io.Reader, pieceSize uint64) (commP [CommLen]byte, err error) {
	f, werr, err := toReadableFile(piece, int64(pieceSize))
	if err != nil {
		return [32]byte{}, err
	}

	commP, err = sectorbuilder.GeneratePieceCommitmentFromFile(f, pieceSize)
	if err != nil {
		return [32]byte{}, err
	}

	return commP, werr()
}

func GenerateDataCommitment(ssize uint64, pieces []PublicPieceInfo) ([CommLen]byte, error) {
	return sectorbuilder.GenerateDataCommitment(ssize, pieces)
}
