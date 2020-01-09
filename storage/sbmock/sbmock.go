package sbmock

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"sync"

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

type sectorState struct {
	pieces [][]byte
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
	defer sb.lk.Unlock()

	ss, ok := sb.sectors[sectorId]
	if !ok {
		ss = &sectorState{}
		sb.sectors[sectorId] = ss
	}

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
	sb.lk.Unlock()
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
	_, ok := sb.sectors[sid]
	if !ok {
		return sectorbuilder.RawSealPreCommitOutput{}, xerrors.Errorf("no sector with id %d in sectorbuilder", sid)
	}

	ussize := sectorbuilder.UserBytesForSectorSize(sb.sectorSize)

	// TODO: verify pieces in sinfo.pieces match passed in pieces

	var sum uint64
	for _, p := range pieces {
		sum += p.Size
	}

	if sum != ussize {
		return sectorbuilder.RawSealPreCommitOutput{}, xerrors.Errorf("aggregated piece sizes don't match up: %d != %d", sum, ussize)
	}

	return sectorbuilder.RawSealPreCommitOutput{
		CommD: randComm(),
		CommR: randComm(),
	}, nil
}

func (sb *SBMock) SealCommit(sid uint64, ticket sectorbuilder.SealTicket, seed sectorbuilder.SealSeed, pieces []sectorbuilder.PublicPieceInfo, precommit sectorbuilder.RawSealPreCommitOutput) ([]byte, error) {
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
