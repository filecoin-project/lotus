package sectorbuilder

import (
	"io"
	"sort"
	"unsafe"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/address"
)

const PoStReservedWorkers = 1
const PoRepProofPartitions = 2

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
	ssize  uint64

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

func New(cfg *Config) (*SectorBuilder, error) {
	if cfg.WorkerThreads <= PoStReservedWorkers {
		return nil, xerrors.Errorf("minimum worker threads is %d, specified %d", PoStReservedWorkers+1, cfg.WorkerThreads)
	}

	proverId := addressToProverID(cfg.Miner)

	sbp, err := sectorbuilder.InitSectorBuilder(cfg.SectorSize, PoRepProofPartitions, 0, cfg.MetadataDir, proverId, cfg.SealedDir, cfg.StagedDir, cfg.CacheDir, 16, cfg.WorkerThreads)
	if err != nil {
		return nil, err
	}

	return &SectorBuilder{
		handle: sbp,
		ssize:  cfg.SectorSize,

		stagedDir: cfg.StagedDir,
		sealedDir: cfg.SealedDir,
		cacheDir:  cfg.CacheDir,

		Miner:     cfg.Miner,
		rateLimit: make(chan struct{}, cfg.WorkerThreads-PoStReservedWorkers),
	}, nil
}

func (sb *SectorBuilder) rlimit() func() {
	if cap(sb.rateLimit) == len(sb.rateLimit) {
		log.Warn("rate-limiting sectorbuilder call")
	}
	sb.rateLimit <- struct{}{}

	return func() {
		<-sb.rateLimit
	}
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
	return sectorbuilder.AcquireSectorId(sb.handle)
}

func (sb *SectorBuilder) AddPiece(pieceSize uint64, sectorId uint64, file io.Reader) (PublicPieceInfo, error) {
	f, werr, err := toReadableFile(file, int64(pieceSize))
	if err != nil {
		return PublicPieceInfo{}, err
	}

	ret := sb.rlimit()
	defer ret()

	stagedFile, err := sb.stagedSectorFile(sectorId)
	if err != nil {
		return PublicPieceInfo{}, err
	}

	writeUnpadded, commP, err := sectorbuilder.StandaloneWriteWithoutAlignment(f, pieceSize, stagedFile)
	if err != nil {
		return PublicPieceInfo{}, err
	}
	if writeUnpadded != pieceSize {
		return PublicPieceInfo{}, xerrors.Errorf("writeUnpadded != pieceSize: %d != %d", writeUnpadded, pieceSize)
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
	ret := sb.rlimit()
	defer ret()

	return sectorbuilder.ReadPieceFromSealedSector(sb.handle, pieceKey)
}

func (sb *SectorBuilder) SealPreCommit(sectorID uint64, ticket SealTicket, pieces []PublicPieceInfo) (RawSealPreCommitOutput, error) {
	ret := sb.rlimit()
	defer ret()

	cacheDir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return RawSealPreCommitOutput{}, err
	}

	sealedPath, err := sb.sealedSectorPath(sectorID)
	if err != nil {
		return RawSealPreCommitOutput{}, err
	}

	return sectorbuilder.StandaloneSealPreCommit(
		sb.ssize,
		PoRepProofPartitions,
		cacheDir,
		sb.stagedSectorPath(sectorID),
		sealedPath,
		sectorID,
		addressToProverID(sb.Miner),
		ticket.TicketBytes,
		pieces,
	)
}

func (sb *SectorBuilder) SealCommit(sectorID uint64, ticket SealTicket, seed SealSeed, pieces []PublicPieceInfo, pieceKeys []string, rspco RawSealPreCommitOutput) (proof []byte, err error) {
	ret := sb.rlimit()
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

func (sb *SectorBuilder) SealStatus(sector uint64) (SectorSealingStatus, error) {
	return sectorbuilder.GetSectorSealingStatusByID(sb.handle, sector)
}

func (sb *SectorBuilder) GetAllStagedSectors() ([]uint64, error) {
	sectors, err := sectorbuilder.GetAllStagedSectors(sb.handle)
	if err != nil {
		return nil, err
	}

	out := make([]uint64, len(sectors))
	for i, v := range sectors {
		out[i] = v.SectorID
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})

	return out, nil
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

func VerifyPost(sectorSize uint64, sectorInfo SortedSectorInfo, challengeSeed [CommLen]byte, proof []byte, faults []uint64) (bool, error) {
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
