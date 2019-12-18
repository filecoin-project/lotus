package sectorbuilder

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	sectorbuilder "github.com/filecoin-project/filecoin-ffi"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	dcopy "github.com/otiai10/copy"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
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

type RawSealPreCommitOutput sectorbuilder.RawSealPreCommitOutput

type EPostCandidate = sectorbuilder.Candidate

const CommLen = sectorbuilder.CommitmentBytesLen

type WorkerCfg struct {
	NoPreCommit bool
	NoCommit    bool

	// TODO: 'cost' info, probably in terms of sealing + transfer speed
}

type SectorBuilder struct {
	ds   dtypes.MetadataDS
	idLk sync.Mutex

	ssize  uint64
	lastID uint64

	Miner address.Address

	unsealLk sync.Mutex

	noCommit    bool
	noPreCommit bool
	rateLimit   chan struct{}

	precommitTasks chan workerCall
	commitTasks    chan workerCall

	taskCtr       uint64
	remoteLk      sync.Mutex
	remoteCtr     int
	remotes       map[int]*remote
	remoteResults map[uint64]chan<- SealRes

	addPieceWait  int32
	preCommitWait int32
	commitWait    int32
	unsealWait    int32

	fsLk       sync.Mutex
	filesystem *fs // TODO: multi-fs support

	stopping chan struct{}
}

type JsonRSPCO struct {
	CommD []byte
	CommR []byte
}

func (rspco *RawSealPreCommitOutput) ToJson() JsonRSPCO {
	return JsonRSPCO{
		CommD: rspco.CommD[:],
		CommR: rspco.CommR[:],
	}
}

func (rspco *JsonRSPCO) rspco() RawSealPreCommitOutput {
	var out RawSealPreCommitOutput
	copy(out.CommD[:], rspco.CommD)
	copy(out.CommR[:], rspco.CommR)
	return out
}

type SealRes struct {
	Err   string
	GoErr error `json:"-"`

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

	WorkerThreads  uint8
	FallbackLastID uint64
	NoCommit       bool
	NoPreCommit    bool

	Dir string
	_   struct{} // guard against nameless init
}

func New(cfg *Config, ds dtypes.MetadataDS) (*SectorBuilder, error) {
	if cfg.WorkerThreads < PoStReservedWorkers {
		return nil, xerrors.Errorf("minimum worker threads is %d, specified %d", PoStReservedWorkers, cfg.WorkerThreads)
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
		lastUsedID = cfg.FallbackLastID
	default:
		return nil, err
	}

	rlimit := cfg.WorkerThreads - PoStReservedWorkers

	sealLocal := rlimit > 0

	if rlimit == 0 {
		rlimit = 1
	}

	sb := &SectorBuilder{
		ds: ds,

		ssize:  cfg.SectorSize,
		lastID: lastUsedID,

		filesystem: openFs(cfg.Dir),

		Miner: cfg.Miner,

		noPreCommit: cfg.NoPreCommit || !sealLocal,
		noCommit:    cfg.NoCommit || !sealLocal,
		rateLimit:   make(chan struct{}, rlimit),

		taskCtr:        1,
		precommitTasks: make(chan workerCall),
		commitTasks:    make(chan workerCall),
		remoteResults:  map[uint64]chan<- SealRes{},
		remotes:        map[int]*remote{},

		stopping: make(chan struct{}),
	}

	if err := sb.filesystem.init(); err != nil {
		return nil, xerrors.Errorf("initializing sectorbuilder filesystem: %w", err)
	}

	return sb, nil
}

func NewStandalone(cfg *Config) (*SectorBuilder, error) {
	sb := &SectorBuilder{
		ds: nil,

		ssize: cfg.SectorSize,

		Miner:      cfg.Miner,
		filesystem: openFs(cfg.Dir),

		taskCtr:   1,
		remotes:   map[int]*remote{},
		rateLimit: make(chan struct{}, cfg.WorkerThreads),
		stopping:  make(chan struct{}),
	}

	if err := sb.filesystem.init(); err != nil {
		return nil, xerrors.Errorf("initializing sectorbuilder filesystem: %w", err)
	}

	return sb, nil
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

	AddPieceWait  int
	PreCommitWait int
	CommitWait    int
	UnsealWait    int
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

		AddPieceWait:  int(atomic.LoadInt32(&sb.addPieceWait)),
		PreCommitWait: int(atomic.LoadInt32(&sb.preCommitWait)),
		CommitWait:    int(atomic.LoadInt32(&sb.commitWait)),
		UnsealWait:    int(atomic.LoadInt32(&sb.unsealWait)),
	}
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
	fs := sb.filesystem

	if err := fs.reserve(dataStaging, sb.ssize); err != nil {
		return PublicPieceInfo{}, err
	}
	defer fs.free(dataStaging, sb.ssize)

	atomic.AddInt32(&sb.addPieceWait, 1)
	ret := sb.RateLimit()
	atomic.AddInt32(&sb.addPieceWait, -1)
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
	fs := sb.filesystem

	if err := fs.reserve(dataUnsealed, sb.ssize); err != nil { // TODO: this needs to get smarter when we start supporting partial unseals
		return nil, err
	}
	defer fs.free(dataUnsealed, sb.ssize)

	atomic.AddInt32(&sb.unsealWait, 1)
	// TODO: Don't wait if cached
	ret := sb.RateLimit() // TODO: check perf, consider remote unseal worker
	defer ret()
	atomic.AddInt32(&sb.unsealWait, -1)

	sb.unsealLk.Lock() // TODO: allow unsealing unrelated sectors in parallel
	defer sb.unsealLk.Unlock()

	cacheDir, err := sb.sectorCacheDir(sectorID)
	if err != nil {
		return nil, err
	}

	sealedPath, err := sb.SealedSectorPath(sectorID)
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

func (sb *SectorBuilder) sealPreCommitRemote(call workerCall) (RawSealPreCommitOutput, error) {
	atomic.AddInt32(&sb.preCommitWait, -1)

	select {
	case ret := <-call.ret:
		var err error
		if ret.Err != "" {
			err = xerrors.New(ret.Err)
		}
		return ret.Rspco.rspco(), err
	case <-sb.stopping:
		return RawSealPreCommitOutput{}, xerrors.New("sectorbuilder stopped")
	}
}

func (sb *SectorBuilder) SealPreCommit(sectorID uint64, ticket SealTicket, pieces []PublicPieceInfo) (RawSealPreCommitOutput, error) {
	fs := sb.filesystem

	if err := fs.reserve(dataCache, sb.ssize); err != nil {
		return RawSealPreCommitOutput{}, err
	}
	defer fs.free(dataCache, sb.ssize)

	if err := fs.reserve(dataSealed, sb.ssize); err != nil {
		return RawSealPreCommitOutput{}, err
	}
	defer fs.free(dataSealed, sb.ssize)

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

	atomic.AddInt32(&sb.preCommitWait, 1)

	select { // prefer remote
	case sb.precommitTasks <- call:
		return sb.sealPreCommitRemote(call)
	default:
	}

	sb.checkRateLimit()

	rl := sb.rateLimit
	if sb.noPreCommit {
		rl = make(chan struct{})
	}

	select { // use whichever is available
	case sb.precommitTasks <- call:
		return sb.sealPreCommitRemote(call)
	case rl <- struct{}{}:
	}

	atomic.AddInt32(&sb.preCommitWait, -1)

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

	e, err := os.OpenFile(sealedPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return RawSealPreCommitOutput{}, xerrors.Errorf("ensuring sealed file exists: %w", err)
	}
	if err := e.Close(); err != nil {
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

	stagedPath := sb.StagedSectorPath(sectorID)

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

	return RawSealPreCommitOutput(rspco), nil
}

func (sb *SectorBuilder) sealCommitRemote(call workerCall) (proof []byte, err error) {
	atomic.AddInt32(&sb.commitWait, -1)

	select {
	case ret := <-call.ret:
		if ret.Err != "" {
			err = xerrors.New(ret.Err)
		}
		return ret.Proof, err
	case <-sb.stopping:
		return nil, xerrors.New("sectorbuilder stopped")
	}
}

func (sb *SectorBuilder) sealCommitLocal(sectorID uint64, ticket SealTicket, seed SealSeed, pieces []PublicPieceInfo, rspco RawSealPreCommitOutput) (proof []byte, err error) {
	atomic.AddInt32(&sb.commitWait, -1)

	defer func() {
		<-sb.rateLimit
	}()

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
		sectorbuilder.RawSealPreCommitOutput(rspco),
	)
	if err != nil {
		log.Warn("StandaloneSealCommit error: ", err)
		log.Warnf("sid:%d tkt:%v seed:%v, ppi:%v rspco:%v", sectorID, ticket, seed, pieces, rspco)

		return nil, xerrors.Errorf("StandaloneSealCommit: %w", err)
	}

	return proof, nil
}

func (sb *SectorBuilder) SealCommit(sectorID uint64, ticket SealTicket, seed SealSeed, pieces []PublicPieceInfo, rspco RawSealPreCommitOutput) (proof []byte, err error) {
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

	atomic.AddInt32(&sb.commitWait, 1)

	select { // prefer remote
	case sb.commitTasks <- call:
		proof, err = sb.sealCommitRemote(call)
	default:
		sb.checkRateLimit()

		rl := sb.rateLimit
		if sb.noCommit {
			rl = make(chan struct{})
		}

		select { // use whichever is available
		case sb.commitTasks <- call:
			proof, err = sb.sealCommitRemote(call)
		case rl <- struct{}{}:
			proof, err = sb.sealCommitLocal(sectorID, ticket, seed, pieces, rspco)
		}
	}
	if err != nil {
		return nil, xerrors.Errorf("commit: %w", err)
	}

	return proof, nil
}

func (sb *SectorBuilder) ComputeElectionPoSt(sectorInfo SortedPublicSectorInfo, challengeSeed []byte, winners []EPostCandidate) ([]byte, error) {
	if len(challengeSeed) != CommLen {
		return nil, xerrors.Errorf("given challenge seed was the wrong length: %d != %d", len(challengeSeed), CommLen)
	}
	var cseed [CommLen]byte
	copy(cseed[:], challengeSeed)

	privsects, err := sb.pubSectorToPriv(sectorInfo, nil) // TODO: faults
	if err != nil {
		return nil, err
	}

	proverID := addressToProverID(sb.Miner)

	return sectorbuilder.GeneratePoSt(sb.ssize, proverID, privsects, cseed, winners)
}

func (sb *SectorBuilder) GenerateEPostCandidates(sectorInfo SortedPublicSectorInfo, challengeSeed [CommLen]byte, faults []uint64) ([]EPostCandidate, error) {
	privsectors, err := sb.pubSectorToPriv(sectorInfo, faults)
	if err != nil {
		return nil, err
	}

	challengeCount := types.ElectionPostChallengeCount(uint64(len(sectorInfo.Values())), len(faults))

	proverID := addressToProverID(sb.Miner)
	return sectorbuilder.GenerateCandidates(sb.ssize, proverID, challengeSeed, challengeCount, privsectors)
}

func (sb *SectorBuilder) pubSectorToPriv(sectorInfo SortedPublicSectorInfo, faults []uint64) (SortedPrivateSectorInfo, error) {
	fmap := map[uint64]struct{}{}
	for _, fault := range faults {
		fmap[fault] = struct{}{}
	}

	var out []sectorbuilder.PrivateSectorInfo
	for _, s := range sectorInfo.Values() {
		if _, faulty := fmap[s.SectorID]; faulty {
			continue
		}

		cachePath, err := sb.sectorCacheDir(s.SectorID)
		if err != nil {
			return SortedPrivateSectorInfo{}, xerrors.Errorf("getting cache path for sector %d: %w", s.SectorID, err)
		}

		sealedPath, err := sb.SealedSectorPath(s.SectorID)
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
	privsectors, err := sb.pubSectorToPriv(sectorInfo, faults)
	if err != nil {
		return nil, nil, err
	}

	challengeCount := fallbackPostChallengeCount(uint64(len(sectorInfo.Values())), len(faults))

	proverID := addressToProverID(sb.Miner)
	candidates, err := sectorbuilder.GenerateCandidates(sb.ssize, proverID, challengeSeed, challengeCount, privsectors)
	if err != nil {
		return nil, nil, err
	}

	proof, err := sectorbuilder.GeneratePoSt(sb.ssize, proverID, privsectors, challengeSeed, candidates)
	return candidates, proof, err
}

func (sb *SectorBuilder) Stop() {
	close(sb.stopping)
}

func fallbackPostChallengeCount(sectors uint64, faults int) uint64 {
	challengeCount := types.ElectionPostChallengeCount(sectors, faults)
	if challengeCount > build.MaxFallbackPostChallengeCount {
		return build.MaxFallbackPostChallengeCount
	}
	return challengeCount
}

func (sb *SectorBuilder) ImportFrom(osb *SectorBuilder, symlink bool) error {
	if err := migrate(osb.filesystem.pathFor(dataCache), sb.filesystem.pathFor(dataCache), symlink); err != nil {
		return err
	}

	if err := migrate(osb.filesystem.pathFor(dataStaging), sb.filesystem.pathFor(dataStaging), symlink); err != nil {
		return err
	}

	if err := migrate(osb.filesystem.pathFor(dataSealed), sb.filesystem.pathFor(dataSealed), symlink); err != nil {
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

func (sb *SectorBuilder) SetLastSectorID(id uint64) error {
	if err := sb.ds.Put(lastSectorIdKey, []byte(fmt.Sprint(id))); err != nil {
		return err
	}

	sb.lastID = id
	return nil
}

func migrate(from, to string, symlink bool) error {
	st, err := os.Stat(from)
	if err != nil {
		return err
	}

	if st.IsDir() {
		return migrateDir(from, to, symlink)
	}
	return migrateFile(from, to, symlink)
}

func migrateDir(from, to string, symlink bool) error {
	tost, err := os.Stat(to)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}

		if err := os.MkdirAll(to, 0755); err != nil {
			return err
		}
	} else if !tost.IsDir() {
		return xerrors.Errorf("target %q already exists and is a file (expected directory)")
	}

	dirents, err := ioutil.ReadDir(from)
	if err != nil {
		return err
	}

	for _, inf := range dirents {
		n := inf.Name()
		if inf.IsDir() {
			if err := migrate(filepath.Join(from, n), filepath.Join(to, n), symlink); err != nil {
				return err
			}
		} else {
			if err := migrate(filepath.Join(from, n), filepath.Join(to, n), symlink); err != nil {
				return err
			}
		}
	}

	return nil
}

func migrateFile(from, to string, symlink bool) error {
	if symlink {
		return os.Symlink(from, to)
	}

	return dcopy.Copy(from, to)
}
