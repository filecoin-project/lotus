package sectorbuilder

import (
	"io"
	"os"
	"sort"
	"sync"
	"unsafe"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/address"
)

const PoStReservedWorkers = 1

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

const CommLen = sectorbuilder.CommitmentBytesLen

type SectorBuilder struct {
	handle unsafe.Pointer
	ssize  uint64

	Miner address.Address

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

	sbp, err := sectorbuilder.InitSectorBuilder(cfg.SectorSize, 2, 0, cfg.MetadataDir, proverId, cfg.SealedDir, cfg.StagedDir, cfg.CacheDir, 16, cfg.WorkerThreads)
	if err != nil {
		return nil, err
	}

	return &SectorBuilder{
		handle: sbp,
		ssize:  cfg.SectorSize,

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

func (sb *SectorBuilder) AddPiece(pieceKey string, pieceSize uint64, file io.Reader) (uint64, error) {
	f, werr, err := toReadableFile(file, int64(pieceSize))
	if err != nil {
		return 0, err
	}

	ret := sb.rlimit()
	defer ret()

	sectorID, err := sectorbuilder.AddPieceFromFile(sb.handle, pieceKey, pieceSize, f)
	if err != nil {
		return 0, err
	}

	return sectorID, werr()
}

// TODO: should *really really* return an io.ReadCloser
func (sb *SectorBuilder) ReadPieceFromSealedSector(pieceKey string) ([]byte, error) {
	ret := sb.rlimit()
	defer ret()

	return sectorbuilder.ReadPieceFromSealedSector(sb.handle, pieceKey)
}

func (sb *SectorBuilder) SealPreCommit(sectorID uint64, ticket SealTicket) (SealPreCommitOutput, error) {
	ret := sb.rlimit()
	defer ret()

	return sectorbuilder.SealPreCommit(sb.handle, sectorID, ticket)
}

func (sb *SectorBuilder) SealCommit(sectorID uint64, seed SealSeed) (SealCommitOutput, error) {
	ret := sb.rlimit()
	defer ret()

	return sectorbuilder.SealCommit(sb.handle, sectorID, seed)
}

func (sb *SectorBuilder) ResumeSealCommit(sectorID uint64) (SealCommitOutput, error) {
	ret := sb.rlimit()
	defer ret()

	return sectorbuilder.ResumeSealCommit(sb.handle, sectorID)
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

func toReadableFile(r io.Reader, n int64) (*os.File, func() error, error) {
	f, ok := r.(*os.File)
	if ok {
		return f, func() error { return nil }, nil
	}

	var w *os.File

	f, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	var wait sync.Mutex
	var werr error

	wait.Lock()
	go func() {
		defer wait.Unlock()

		copied, werr := io.CopyN(w, r, n)
		if werr != nil {
			log.Warnf("toReadableFile: copy error: %+v", werr)
		}

		err := w.Close()
		if werr == nil && err != nil {
			werr = err
			log.Warnf("toReadableFile: close error: %+v", err)
			return
		}
		if copied != n {
			log.Warnf("copied different amount than expected: %d != %d", copied, n)
			werr = xerrors.Errorf("copied different amount than expected: %d != %d", copied, n)
		}
	}()

	return f, func() error {
		wait.Lock()
		return werr
	}, nil
}
