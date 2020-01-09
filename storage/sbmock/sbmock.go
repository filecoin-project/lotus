package sbmock

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"sync"
	"time"

	"github.com/filecoin-project/go-sectorbuilder"
	"golang.org/x/xerrors"
)

func randComm() [sectorbuilder.CommLen]byte {
	var out [sectorbuilder.CommLen]byte
	rand.Read(out[:])
	return out
}

type SBMock struct {
	sectors      map[uint64]*sectorState
	sectorSize   uint64
	nextSectorID uint64
	rateLimit    chan struct{}

	preCommitDelay time.Duration
	commitDelay    time.Duration

	lk sync.Mutex
}

func NewMockSectorBuilder(threads int, ssize uint64) *SBMock {
	return &SBMock{
		sectors:      make(map[uint64]*sectorState),
		sectorSize:   ssize,
		nextSectorID: 0,
		rateLimit:    make(chan struct{}, threads),
	}
}

const (
	statePacking = iota
	statePreCommit
	stateCommit
)

type sectorState struct {
	pieces [][]byte
	failed bool

	state int

	lk sync.Mutex
}

func (sb *SBMock) RateLimit() func() {
	sb.rateLimit <- struct{}{}

	// TODO: probably want to copy over rate limit code
	return func() {
		<-sb.rateLimit
	}
}

func (sb *SBMock) AddPiece(size uint64, sectorId uint64, r io.Reader, existingPieces []uint64) (sectorbuilder.PublicPieceInfo, error) {
	sb.lk.Lock()
	ss, ok := sb.sectors[sectorId]
	if !ok {
		ss = &sectorState{
			state: statePacking,
		}
		sb.sectors[sectorId] = ss
	}
	sb.lk.Unlock()
	ss.lk.Lock()
	defer ss.lk.Unlock()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return sectorbuilder.PublicPieceInfo{}, err
	}

	ss.pieces = append(ss.pieces, b)
	return sectorbuilder.PublicPieceInfo{
		Size: size,
		// TODO: should we compute a commP? maybe do it when we need it
	}, nil
}

func (sb *SBMock) SectorSize() uint64 {
	return sb.sectorSize
}

func (sb *SBMock) AcquireSectorId() (uint64, error) {
	sb.lk.Lock()
	defer sb.lk.Unlock()
	id := sb.nextSectorID
	sb.nextSectorID++
	return id, nil
}

func (sb *SBMock) Scrub(sectorbuilder.SortedPublicSectorInfo) []*sectorbuilder.Fault {
	return nil
}

func (sb *SBMock) GenerateFallbackPoSt(sectorbuilder.SortedPublicSectorInfo, [sectorbuilder.CommLen]byte, []uint64) ([]sectorbuilder.EPostCandidate, []byte, error) {
	panic("NYI")
}

func (sb *SBMock) SealPreCommit(sid uint64, ticket sectorbuilder.SealTicket, pieces []sectorbuilder.PublicPieceInfo) (sectorbuilder.RawSealPreCommitOutput, error) {
	sb.lk.Lock()
	ss, ok := sb.sectors[sid]
	sb.lk.Unlock()
	if !ok {
		return sectorbuilder.RawSealPreCommitOutput{}, xerrors.Errorf("no sector with id %d in sectorbuilder", sid)
	}

	ss.lk.Lock()
	defer ss.lk.Unlock()

	ussize := sectorbuilder.UserBytesForSectorSize(sb.sectorSize)

	// TODO: verify pieces in sinfo.pieces match passed in pieces

	var sum uint64
	for _, p := range pieces {
		sum += p.Size
	}

	if sum != ussize {
		return sectorbuilder.RawSealPreCommitOutput{}, xerrors.Errorf("aggregated piece sizes don't match up: %d != %d", sum, ussize)
	}

	if ss.state != statePacking {
		return sectorbuilder.RawSealPreCommitOutput{}, xerrors.Errorf("cannot call pre-seal on sector not in 'packing' state")
	}

	time.Sleep(sb.preCommitDelay)

	ss.state = statePreCommit

	return sectorbuilder.RawSealPreCommitOutput{
		CommD: randComm(),
		CommR: randComm(),
	}, nil
}

func (sb *SBMock) SealCommit(sid uint64, ticket sectorbuilder.SealTicket, seed sectorbuilder.SealSeed, pieces []sectorbuilder.PublicPieceInfo, precommit sectorbuilder.RawSealPreCommitOutput) ([]byte, error) {
	sb.lk.Lock()
	ss, ok := sb.sectors[sid]
	sb.lk.Unlock()
	if !ok {
		return nil, xerrors.Errorf("no such sector %d", sid)
	}
	ss.lk.Lock()
	defer ss.lk.Unlock()

	if ss.failed {
		return nil, xerrors.Errorf("[mock] cannot commit failed sector %d", sid)
	}

	if ss.state != statePreCommit {
		return nil, xerrors.Errorf("cannot commit sector that has not been precommitted")
	}

	time.Sleep(sb.commitDelay)

	buf := make([]byte, 32)
	rand.Read(buf)
	return buf, nil
}

func (sb *SBMock) GetPath(string, string) (string, error) {
	panic("nyi")
}

func (sb *SBMock) WorkerStats() sectorbuilder.WorkerStats {
	panic("nyi")
}

func (sb *SBMock) AddWorker(context.Context, sectorbuilder.WorkerCfg) (<-chan sectorbuilder.WorkerTask, error) {
	panic("nyi")
}

func (sb *SBMock) TaskDone(context.Context, uint64, sectorbuilder.SealRes) error {
	panic("nyi")
}

// Test Instrumentation Methods

func (sb *SBMock) FailSector(sid uint64) error {
	sb.lk.Lock()
	defer sb.lk.Unlock()
	ss, ok := sb.sectors[sid]
	if !ok {
		return fmt.Errorf("no such sector in sectorbuilder")
	}

	ss.failed = true
	return nil
}

func (sb *SBMock) SetPreCommitDelay(d time.Duration) {
	sb.lk.Lock()
	defer sb.lk.Unlock()
	sb.preCommitDelay = d
}

func (sb *SBMock) SetCommitDelay(d time.Duration) {
	sb.lk.Lock()
	defer sb.lk.Unlock()
	sb.commitDelay = d
}
