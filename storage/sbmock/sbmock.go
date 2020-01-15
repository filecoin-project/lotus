package sbmock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"math/rand"
	"sync"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"golang.org/x/xerrors"
)

type SBMock struct {
	sectors      map[uint64]*sectorState
	sectorSize   uint64
	nextSectorID uint64
	rateLimit    chan struct{}

	lk sync.Mutex
}

type mockVerif struct{}

func NewMockSectorBuilder(threads int, ssize uint64) *SBMock {
	return &SBMock{
		sectors:      make(map[uint64]*sectorState),
		sectorSize:   ssize,
		nextSectorID: 5,
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
		Size:  size,
		CommP: commD(b),
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
	sb.lk.Lock()
	mcopy := make(map[uint64]*sectorState)
	for k, v := range sb.sectors {
		mcopy[k] = v
	}
	sb.lk.Unlock()

	var out []*sectorbuilder.Fault
	for sid, ss := range mcopy {
		ss.lk.Lock()
		if ss.failed {
			out = append(out, &sectorbuilder.Fault{
				SectorID: sid,
				Err:      fmt.Errorf("mock sector failed"),
			})

		}
		ss.lk.Unlock()
	}

	return out
}

func (sb *SBMock) GenerateFallbackPoSt(sectorbuilder.SortedPublicSectorInfo, [sectorbuilder.CommLen]byte, []uint64) ([]sectorbuilder.EPostCandidate, []byte, error) {
	panic("NYI")
}

func (sb *SBMock) SealPreCommit(ctx context.Context, sid uint64, ticket sectorbuilder.SealTicket, pieces []sectorbuilder.PublicPieceInfo) (sectorbuilder.RawSealPreCommitOutput, error) {
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

	opFinishWait(ctx)

	ss.state = statePreCommit

	pis := make([]ffi.PublicPieceInfo, len(ss.pieces))
	for i, piece := range ss.pieces {
		pis[i] = ffi.PublicPieceInfo{
			Size:  uint64(len(piece)),
			CommP: commD(piece),
		}
	}

	commd, err := MockVerifier.GenerateDataCommitment(sb.sectorSize, pis)
	if err != nil {
		return sectorbuilder.RawSealPreCommitOutput{}, err
	}

	return sectorbuilder.RawSealPreCommitOutput{
		CommD: commd,
		CommR: commDR(commd[:]),
	}, nil
}

func (sb *SBMock) SealCommit(ctx context.Context, sid uint64, ticket sectorbuilder.SealTicket, seed sectorbuilder.SealSeed, pieces []sectorbuilder.PublicPieceInfo, precommit sectorbuilder.RawSealPreCommitOutput) ([]byte, error) {
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

	opFinishWait(ctx)

	var out [32]byte
	for i := range out {
		out[i] = precommit.CommD[i] + precommit.CommR[31-i] - ticket.TicketBytes[i]*seed.TicketBytes[i]
	}
	return out[:], nil
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

func opFinishWait(ctx context.Context) {
	val, ok := ctx.Value("opfinish").(chan struct{})
	if !ok {
		return
	}
	<-val
}

func AddOpFinish(ctx context.Context) (context.Context, func()) {
	done := make(chan struct{})

	return context.WithValue(ctx, "opfinish", done), func() {
		close(done)
	}
}

func (sb *SBMock) ComputeElectionPoSt(sectorInfo sectorbuilder.SortedPublicSectorInfo, challengeSeed []byte, winners []sectorbuilder.EPostCandidate) ([]byte, error) {
	panic("implement me")
}

func (sb *SBMock) GenerateEPostCandidates(sectorInfo sectorbuilder.SortedPublicSectorInfo, challengeSeed [sectorbuilder.CommLen]byte, faults []uint64) ([]sectorbuilder.EPostCandidate, error) {
	if len(faults) > 0 {
		panic("todo")
	}

	n := sectorbuilder.ElectionPostChallengeCount(uint64(len(sectorInfo.Values())), uint64(len(faults)))
	if n > uint64(len(sectorInfo.Values())) {
		n = uint64(len(sectorInfo.Values()))
	}

	out := make([]sectorbuilder.EPostCandidate, n)

	seed := big.NewInt(0).SetBytes(challengeSeed[:])
	start := seed.Mod(seed, big.NewInt(int64(len(sectorInfo.Values())))).Int64()

	for i := range out {
		out[i] = sectorbuilder.EPostCandidate{
			SectorID:             uint64((int(start) + i) % len(sectorInfo.Values())),
			PartialTicket:        challengeSeed,
			Ticket:               commDR(challengeSeed[:]),
			SectorChallengeIndex: 0,
		}
	}

	return out, nil
}

func (sb *SBMock) ReadPieceFromSealedSector(sectorID uint64, offset uint64, size uint64, ticket []byte, commD []byte) (io.ReadCloser, error) {
	if len(sb.sectors[sectorID].pieces) > 1 {
		panic("implme")
	}
	return ioutil.NopCloser(io.LimitReader(bytes.NewReader(sb.sectors[sectorID].pieces[0][offset:]), int64(size))), nil
}

func (sb *SBMock) StageFakeData() (uint64, []sectorbuilder.PublicPieceInfo, error) {
	usize := sectorbuilder.UserBytesForSectorSize(sb.sectorSize)
	sid, err := sb.AcquireSectorId()
	if err != nil {
		return 0, nil, err
	}

	buf := make([]byte, usize)
	rand.Read(buf)

	pi, err := sb.AddPiece(usize, sid, bytes.NewReader(buf), nil)
	if err != nil {
		return 0, nil, err
	}

	return sid, []sectorbuilder.PublicPieceInfo{pi}, nil
}

func (m mockVerif) VerifyElectionPost(ctx context.Context, sectorSize uint64, sectorInfo sectorbuilder.SortedPublicSectorInfo, challengeSeed []byte, proof []byte, candidates []sectorbuilder.EPostCandidate, proverID address.Address) (bool, error) {
	panic("implement me")
}

func (m mockVerif) VerifyFallbackPost(ctx context.Context, sectorSize uint64, sectorInfo sectorbuilder.SortedPublicSectorInfo, challengeSeed []byte, proof []byte, candidates []sectorbuilder.EPostCandidate, proverID address.Address, faults uint64) (bool, error) {
	panic("implement me")
}

func (m mockVerif) VerifySeal(sectorSize uint64, commR, commD []byte, proverID address.Address, ticket []byte, seed []byte, sectorID uint64, proof []byte) (bool, error) {
	if len(proof) != 32 { // Real ones are longer, but this should be fine
		return false, nil
	}

	for i, b := range proof {
		if b != commD[i]+commR[31-i]-ticket[i]*seed[i] {
			return false, nil
		}
	}

	return true, nil
}

func (m mockVerif) GenerateDataCommitment(ssize uint64, pieces []ffi.PublicPieceInfo) ([sectorbuilder.CommLen]byte, error) {
	if len(pieces) != 1 {
		panic("todo")
	}
	if pieces[0].Size != sectorbuilder.UserBytesForSectorSize(ssize) {
		panic("todo")
	}
	return pieces[0].CommP, nil
}

var MockVerifier = mockVerif{}

var _ sectorbuilder.Verifier = MockVerifier
var _ sectorbuilder.Interface = &SBMock{}
