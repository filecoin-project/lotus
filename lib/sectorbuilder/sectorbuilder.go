package sectorbuilder

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	sectorbuilder "github.com/filecoin-project/filecoin-ffi"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

const PoStReservedWorkers = 1
const PoRepProofPartitions = 10

var lastSectorIdKey = datastore.NewKey("/sectorbuilder/last")

var log = logging.Logger("sectorbuilder")

type SortedPublicSectorInfo = sectorbuilder.SortedPublicSectorInfo
type SortedPrivateSectorInfo = sectorbuilder.SortedPrivateSectorInfo

type SealTicket = sectorbuilder.SealTicket

type SealSeed = sectorbuilder.SealSeed

type SealPreCommitOutput = sectorbuilder.SealPreCommitOutput

type SealCommitOutput = sectorbuilder.SealCommitOutput

type PublicPieceInfo = sectorbuilder.PublicPieceInfo

type RawSealPreCommitOutput = sectorbuilder.RawSealPreCommitOutput

type EPostCandidate = sectorbuilder.Candidate

const CommLen = sectorbuilder.CommitmentBytesLen

type SectorBuilder struct {
	ds   dtypes.MetadataDS
	idLk sync.Mutex

	ssize  uint64
	lastID uint64

	Miner address.Address

	stagedDir   string
	sealedDir   string
	cacheDir    string
	unsealedDir string

	unsealLk sync.Mutex

	rateLimit chan struct{}
}

type Config struct {
	SectorSize uint64
	Miner      address.Address

	WorkerThreads uint8

	CacheDir    string
	SealedDir   string
	StagedDir   string
	UnsealedDir string
}

func New(cfg *Config, ds dtypes.MetadataDS) (*SectorBuilder, error) {
	if cfg.WorkerThreads <= PoStReservedWorkers {
		return nil, xerrors.Errorf("minimum worker threads is %d, specified %d", PoStReservedWorkers+1, cfg.WorkerThreads)
	}

	for _, dir := range []string{cfg.StagedDir, cfg.SealedDir, cfg.CacheDir, cfg.UnsealedDir} {
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

	sb := &SectorBuilder{
		ds: ds,

		ssize:  cfg.SectorSize,
		lastID: lastUsedID,

		stagedDir:   cfg.StagedDir,
		sealedDir:   cfg.SealedDir,
		cacheDir:    cfg.CacheDir,
		unsealedDir: cfg.UnsealedDir,

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

func (sb *SectorBuilder) AcquireSectorId() (uint64, error) {
	sb.idLk.Lock()
	defer sb.idLk.Unlock()

	sb.lastID++
	id := sb.lastID

	err := sb.ds.Put(lastSectorIdKey, []byte(fmt.Sprint(id)))
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

	_, _, commP, err := sectorbuilder.WriteWithAlignment(f, pieceSize, stagedFile, existingPieceSizes)
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

func (sb *SectorBuilder) ReadPieceFromSealedSector(sectorID uint64, offset uint64, size uint64, ticket []byte, commD []byte) (io.ReadCloser, error) {
	ret := sb.RateLimit() // TODO: check perf, consider remote unseal worker
	defer ret()

	sb.unsealLk.Lock() // TODO: allow unsealing unrelated sectors in parallel
	defer sb.unsealLk.Unlock()

	cacheDir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return nil, err
	}

	sealedPath, err := sb.sealedSectorPath(sectorID)
	if err != nil {
		return nil, err
	}

	unsealedPath := sb.unsealedSectorPath(sectorID)

	// TODO: GC for those
	//  (Probably configurable count of sectors to be kept unsealed, and just
	//   remove last used one (or use whatever other cache policy makes sense))
	f, err := os.OpenFile(unsealedPath, os.O_RDONLY, 0644)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		var commd [CommLen]byte
		copy(commd[:], commD)

		var tkt [CommLen]byte
		copy(tkt[:], ticket)

		err = sectorbuilder.Unseal(sb.ssize,
			PoRepProofPartitions,
			cacheDir,
			sealedPath,
			unsealedPath,
			sectorID,
			addressToProverID(sb.Miner),
			tkt,
			commd)
		if err != nil {
			return nil, xerrors.Errorf("unseal failed: %w", err)
		}

		f, err = os.OpenFile(unsealedPath, os.O_RDONLY, 0644)
		if err != nil {
			return nil, err
		}
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

	rspco, err := sectorbuilder.SealPreCommit(
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

func (sb *SectorBuilder) SealCommit(sectorID uint64, ticket SealTicket, seed SealSeed, pieces []PublicPieceInfo, rspco RawSealPreCommitOutput) (proof []byte, err error) {
	ret := sb.RateLimit()
	defer ret()

	cacheDir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return nil, err
	}

	proof, err = sectorbuilder.SealCommit(
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
		return nil, xerrors.Errorf("SealCommit: %w", err)
	}

	return proof, nil
}

func (sb *SectorBuilder) SectorSize() uint64 {
	return sb.ssize
}

func (sb *SectorBuilder) ComputeElectionPoSt(sectorInfo SortedPublicSectorInfo, challengeSeed []byte, winners []EPostCandidate) ([]byte, error) {
	if len(challengeSeed) != CommLen {
		return nil, xerrors.Errorf("given challenge seed was the wrong length: %d != %d", len(challengeSeed), CommLen)
	}
	var cseed [CommLen]byte
	copy(cseed[:], challengeSeed)

	privsects, err := sb.pubSectorToPriv(sectorInfo)
	if err != nil {
		return nil, err
	}

	proverID := addressToProverID(sb.Miner)

	return sectorbuilder.GeneratePoSt(sb.ssize, proverID, privsects, cseed, winners)
}

func (sb *SectorBuilder) GenerateEPostCandidates(sectorInfo SortedPublicSectorInfo, challengeSeed [CommLen]byte, faults []uint64) ([]EPostCandidate, error) {
	privsectors, err := sb.pubSectorToPriv(sectorInfo)
	if err != nil {
		return nil, err
	}

	challengeCount := ElectionPostChallengeCount(uint64(len(sectorInfo.Values())))

	proverID := addressToProverID(sb.Miner)
	return sectorbuilder.GenerateCandidates(sb.ssize, proverID, challengeSeed, challengeCount, privsectors)
}

func (sb *SectorBuilder) pubSectorToPriv(sectorInfo SortedPublicSectorInfo) (SortedPrivateSectorInfo, error) {
	var out []sectorbuilder.PrivateSectorInfo
	for _, s := range sectorInfo.Values() {
		cachePath, err := sb.sectorCacheDir(s.SectorID)
		if err != nil {
			return SortedPrivateSectorInfo{}, xerrors.Errorf("getting cache path for sector %d: %w", s.SectorID, err)
		}

		sealedPath, err := sb.sealedSectorPath(s.SectorID)
		if err != nil {
			return SortedPrivateSectorInfo{}, xerrors.Errorf("getting sealed path for sector %d: %w", s.SectorID, err)
		}

		out = append(out, sectorbuilder.PrivateSectorInfo{
			SectorID:         s.SectorID,
			CommR:            s.CommR,
			CacheDirPath:     cachePath,
			SealedSectorPath: sealedPath,
		})
	}
	return NewSortedPrivateSectorInfo(out), nil
}

func (sb *SectorBuilder) GenerateFallbackPoSt(sectorInfo SortedPublicSectorInfo, challengeSeed [CommLen]byte, faults []uint64) ([]EPostCandidate, []byte, error) {
	privsectors, err := sb.pubSectorToPriv(sectorInfo)
	if err != nil {
		return nil, nil, err
	}

	challengeCount := fallbackPostChallengeCount(uint64(len(sectorInfo.Values())))

	proverID := addressToProverID(sb.Miner)
	candidates, err := sectorbuilder.GenerateCandidates(sb.ssize, proverID, challengeSeed, challengeCount, privsectors)
	if err != nil {
		return nil, nil, err
	}

	proof, err := sectorbuilder.GeneratePoSt(sb.ssize, proverID, privsectors, challengeSeed, candidates)
	return candidates, proof, err
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

func NewSortedPrivateSectorInfo(sectors []sectorbuilder.PrivateSectorInfo) SortedPrivateSectorInfo {
	return sectorbuilder.NewSortedPrivateSectorInfo(sectors...)
}

func NewSortedPublicSectorInfo(sectors []sectorbuilder.PublicSectorInfo) SortedPublicSectorInfo {
	return sectorbuilder.NewSortedPublicSectorInfo(sectors...)
}

func VerifyElectionPost(ctx context.Context, sectorSize uint64, sectorInfo SortedPublicSectorInfo, challengeSeed []byte, proof []byte, candidates []EPostCandidate, proverID address.Address) (bool, error) {
	challengeCount := ElectionPostChallengeCount(uint64(len(sectorInfo.Values())))
	return verifyPost(ctx, sectorSize, sectorInfo, challengeCount, challengeSeed, proof, candidates, proverID)
}

func VerifyFallbackPost(ctx context.Context, sectorSize uint64, sectorInfo SortedPublicSectorInfo, challengeSeed []byte, proof []byte, candidates []EPostCandidate, proverID address.Address) (bool, error) {
	challengeCount := fallbackPostChallengeCount(uint64(len(sectorInfo.Values())))
	return verifyPost(ctx, sectorSize, sectorInfo, challengeCount, challengeSeed, proof, candidates, proverID)
}

func verifyPost(ctx context.Context, sectorSize uint64, sectorInfo SortedPublicSectorInfo, challengeCount uint64, challengeSeed []byte, proof []byte, candidates []EPostCandidate, proverID address.Address) (bool, error) {
	if challengeCount != uint64(len(candidates)) {
		log.Warnf("verifyPost with wrong candidate count: expected %d, got %d", challengeCount, len(candidates))
		return false, nil // user input, dont't error
	}

	var challengeSeeda [CommLen]byte
	copy(challengeSeeda[:], challengeSeed)

	_, span := trace.StartSpan(ctx, "VerifyPoSt")
	defer span.End()
	prover := addressToProverID(proverID)
	return sectorbuilder.VerifyPoSt(sectorSize, sectorInfo, challengeSeeda, challengeCount, proof, candidates, prover)
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

func ElectionPostChallengeCount(sectors uint64) uint64 {
	// ceil(sectors / build.SectorChallengeRatioDiv)
	return (sectors + build.SectorChallengeRatioDiv - 1) / build.SectorChallengeRatioDiv
}

func fallbackPostChallengeCount(sectors uint64) uint64 {
	challengeCount := ElectionPostChallengeCount(sectors)
	if challengeCount > build.MaxFallbackPostChallengeCount {
		return build.MaxFallbackPostChallengeCount
	}
	return challengeCount
}

func (sb *SectorBuilder) ImportFrom(osb *SectorBuilder) error {
	if err := moveAllFiles(osb.cacheDir, sb.cacheDir); err != nil {
		return err
	}

	if err := moveAllFiles(osb.sealedDir, sb.sealedDir); err != nil {
		return err
	}

	if err := moveAllFiles(osb.stagedDir, sb.stagedDir); err != nil {
		return err
	}

	val, err := osb.ds.Get(lastSectorIdKey)
	if err != nil {
		return err
	}

	if err := sb.ds.Put(lastSectorIdKey, val); err != nil {
		return err
	}

	sb.lastID = osb.lastID

	return nil
}

func moveAllFiles(from, to string) error {
	dir, err := os.Open(from)
	if err != nil {
		return err
	}

	names, err := dir.Readdirnames(0)
	if err != nil {
		return xerrors.Errorf("failed to list items in dir: %w", err)
	}
	for _, n := range names {
		if err := os.Rename(filepath.Join(from, n), filepath.Join(to, n)); err != nil {
			return xerrors.Errorf("moving file failed: %w", err)
		}
	}

	return nil
}
