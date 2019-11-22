package sectorbuilder

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
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

type RawSealPreCommitOutput sectorbuilder.RawSealPreCommitOutput

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

	sealLocal bool
	rateLimit chan struct{}

	sealTasks chan workerCall

	taskCtr       uint64
	remoteLk      sync.Mutex
	remoteCtr     int
	remotes       map[int]*remote
	remoteResults map[uint64]chan<- SealRes

	stopping chan struct{}
}

type JsonRSPCO struct {
	CommC     []byte
	CommD     []byte
	CommR     []byte
	CommRLast []byte
}

func (rspco *RawSealPreCommitOutput) ToJson() JsonRSPCO {
	return JsonRSPCO{
		CommC:     rspco.CommC[:],
		CommD:     rspco.CommD[:],
		CommR:     rspco.CommR[:],
		CommRLast: rspco.CommRLast[:],
	}
}

func (rspco *JsonRSPCO) rspco() RawSealPreCommitOutput {
	var out RawSealPreCommitOutput
	copy(out.CommC[:], rspco.CommC)
	copy(out.CommD[:], rspco.CommD)
	copy(out.CommR[:], rspco.CommR)
	copy(out.CommRLast[:], rspco.CommRLast)
	return out
}

type SealRes struct {
	Err error

	Proof []byte
	Rspco JsonRSPCO
}

type remote struct {
	lk sync.Mutex

	sealTasks chan<- WorkerTask
	busy      uint64 // only for metrics
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
	if cfg.WorkerThreads < PoStReservedWorkers {
		return nil, xerrors.Errorf("minimum worker threads is %d, specified %d", PoStReservedWorkers, cfg.WorkerThreads)
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

	rlimit := cfg.WorkerThreads - PoStReservedWorkers

	sealLocal := rlimit > 0

	if rlimit == 0 {
		rlimit = 1
	}

	sb := &SectorBuilder{
		handle: sbp,
		ds:     ds,

		ssize: cfg.SectorSize,

		stagedDir: cfg.StagedDir,
		sealedDir: cfg.SealedDir,
		cacheDir:  cfg.CacheDir,

		Miner: cfg.Miner,

		sealLocal: sealLocal,
		rateLimit: make(chan struct{}, rlimit),

		taskCtr:       1,
		sealTasks:     make(chan workerCall),
		remoteResults: map[uint64]chan<- SealRes{},
		remotes:       map[int]*remote{},

		stopping: make(chan struct{}),
	}

	return sb, nil
}

func NewStandalone(cfg *Config) (*SectorBuilder, error) {
	for _, dir := range []string{cfg.StagedDir, cfg.SealedDir, cfg.CacheDir, cfg.MetadataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			if os.IsExist(err) {
				continue
			}
			return nil, err
		}
	}

	return &SectorBuilder{
		handle:    nil,
		ds:        nil,
		ssize:     cfg.SectorSize,
		Miner:     cfg.Miner,
		stagedDir: cfg.StagedDir,
		sealedDir: cfg.SealedDir,
		cacheDir:  cfg.CacheDir,

		sealLocal: true,
		taskCtr:   1,
		remotes:   map[int]*remote{},
		rateLimit: make(chan struct{}, cfg.WorkerThreads),
		stopping:  make(chan struct{}),
	}, nil
}

func (sb *SectorBuilder) checkRateLimit() {
	if cap(sb.rateLimit) == len(sb.rateLimit) {
		log.Warn("rate-limiting local sectorbuilder call")
	}
}

func (sb *SectorBuilder) RateLimit() func() {
	sb.checkRateLimit()

	sb.rateLimit <- struct{}{}

	return func() {
		<-sb.rateLimit
	}
}

type WorkerStats struct {
	LocalFree     int
	LocalReserved int
	LocalTotal    int
	// todo: post in progress
	RemotesTotal int
	RemotesFree  int
}

func (sb *SectorBuilder) WorkerStats() WorkerStats {
	sb.remoteLk.Lock()
	defer sb.remoteLk.Unlock()

	remoteFree := len(sb.remotes)
	for _, r := range sb.remotes {
		if r.busy > 0 {
			remoteFree--
		}
	}

	return WorkerStats{
		LocalFree:     cap(sb.rateLimit) - len(sb.rateLimit),
		LocalReserved: PoStReservedWorkers,
		LocalTotal:    cap(sb.rateLimit) + PoStReservedWorkers,
		RemotesTotal:  len(sb.remotes),
		RemotesFree:   remoteFree,
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

func (sb *SectorBuilder) sealPreCommitRemote(call workerCall) (RawSealPreCommitOutput, error) {
	select {
	case ret := <-call.ret:
		return ret.Rspco.rspco(), ret.Err
	case <-sb.stopping:
		return RawSealPreCommitOutput{}, xerrors.New("sectorbuilder stopped")
	}
}

func (sb *SectorBuilder) SealPreCommit(sectorID uint64, ticket SealTicket, pieces []PublicPieceInfo) (RawSealPreCommitOutput, error) {
	call := workerCall{
		task: WorkerTask{
			Type:       WorkerPreCommit,
			TaskID:     atomic.AddUint64(&sb.taskCtr, 1),
			SectorID:   sectorID,
			SealTicket: ticket,
			Pieces:     pieces,
		},
		ret: make(chan SealRes),
	}

	select { // prefer remote
	case sb.sealTasks <- call:
		return sb.sealPreCommitRemote(call)
	default:
	}

	sb.checkRateLimit()

	select { // use whichever is available
	case sb.sealTasks <- call:
		return sb.sealPreCommitRemote(call)
	case sb.rateLimit <- struct{}{}:
	}

	// local

	defer func() {
		<-sb.rateLimit
	}()

	cacheDir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return RawSealPreCommitOutput{}, xerrors.Errorf("getting cache dir: %w", err)
	}

	sealedPath, err := sb.SealedSectorPath(sectorID)
	if err != nil {
		return RawSealPreCommitOutput{}, xerrors.Errorf("getting sealed sector path: %w", err)
	}

	var sum uint64
	for _, piece := range pieces {
		sum += piece.Size
	}
	ussize := UserBytesForSectorSize(sb.ssize)
	if sum != ussize {
		return RawSealPreCommitOutput{}, xerrors.Errorf("aggregated piece sizes don't match sector size: %d != %d (%d)", sum, ussize, int64(ussize-sum))
	}

	stagedPath := sb.StagedSectorPath(sectorID)

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

	return RawSealPreCommitOutput(rspco), nil
}

func (sb *SectorBuilder) sealCommitRemote(call workerCall) (proof []byte, err error) {
	select {
	case ret := <-call.ret:
		return ret.Proof, ret.Err
	case <-sb.stopping:
		return nil, xerrors.New("sectorbuilder stopped")
	}
}

func (sb *SectorBuilder) sealCommitLocal(sectorID uint64, ticket SealTicket, seed SealSeed, pieces []PublicPieceInfo, rspco RawSealPreCommitOutput) (proof []byte, err error) {
	defer func() {
		<-sb.rateLimit
	}()

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
		sectorbuilder.RawSealPreCommitOutput(rspco),
	)
	if err != nil {
		log.Warn("StandaloneSealCommit error: ", err)
		log.Warnf("sid:%d tkt:%v seed:%v, ppi:%v rspco:%v", sectorID, ticket, seed, pieces, rspco)

		return nil, xerrors.Errorf("StandaloneSealCommit: %w", err)
	}

	return proof, nil
}

func (sb *SectorBuilder) SealCommit(sectorID uint64, ticket SealTicket, seed SealSeed, pieces []PublicPieceInfo, pieceKeys []string, rspco RawSealPreCommitOutput) (proof []byte, err error) {
	call := workerCall{
		task: WorkerTask{
			Type:       WorkerCommit,
			TaskID:     atomic.AddUint64(&sb.taskCtr, 1),
			SectorID:   sectorID,
			SealTicket: ticket,
			Pieces:     pieces,

			SealSeed: seed,
			Rspco:    rspco,
		},
		ret: make(chan SealRes),
	}

	select { // prefer remote
	case sb.sealTasks <- call:
		proof, err = sb.sealCommitRemote(call)
	default:
		sb.checkRateLimit()

		select { // use whichever is available
		case sb.sealTasks <- call:
			proof, err = sb.sealCommitRemote(call)
		case sb.rateLimit <- struct{}{}:
			proof, err = sb.sealCommitLocal(sectorID, ticket, seed, pieces, rspco)
		}
	}
	if err != nil {
		return nil, xerrors.Errorf("commit: %w", err)
	}

	if pieceKeys == nil {
		return
	}

	pmeta := make([]sectorbuilder.PieceMetadata, len(pieces))
	for i, piece := range pieces {
		pmeta[i] = sectorbuilder.PieceMetadata{
			Key:   pieceKeys[i],
			Size:  piece.Size,
			CommP: piece.CommP,
		}
	}

	sealedPath, err := sb.SealedSectorPath(sectorID)
	if err != nil {
		return nil, err
	}

	cacheDir, err := sb.sectorCacheDir(sectorID)
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

func (sb *SectorBuilder) Stop() {
	close(sb.stopping)
}
