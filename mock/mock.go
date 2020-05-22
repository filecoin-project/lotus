package mock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"sync"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
)

var log = logging.Logger("sbmock")

type SectorMgr struct {
	sectors      map[abi.SectorID]*sectorState
	sectorSize   abi.SectorSize
	nextSectorID abi.SectorNumber
	proofType    abi.RegisteredProof

	lk sync.Mutex
}

type mockVerif struct{}

func NewMockSectorMgr(ssize abi.SectorSize) *SectorMgr {
	rt, err := ffiwrapper.SealProofTypeFromSectorSize(ssize)
	if err != nil {
		panic(err)
	}

	return &SectorMgr{
		sectors:      make(map[abi.SectorID]*sectorState),
		sectorSize:   ssize,
		nextSectorID: 5,
		proofType:    rt,
	}
}

const (
	statePacking = iota
	statePreCommit
	stateCommit // nolint
)

type sectorState struct {
	pieces []cid.Cid
	failed bool

	state int

	lk sync.Mutex
}

func (mgr *SectorMgr) NewSector(ctx context.Context, sector abi.SectorID) error {
	return nil
}

func (mgr *SectorMgr) AddPiece(ctx context.Context, sectorId abi.SectorID, existingPieces []abi.UnpaddedPieceSize, size abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	log.Warn("Add piece: ", sectorId, size, mgr.proofType)
	mgr.lk.Lock()
	ss, ok := mgr.sectors[sectorId]
	if !ok {
		ss = &sectorState{
			state: statePacking,
		}
		mgr.sectors[sectorId] = ss
	}
	mgr.lk.Unlock()
	ss.lk.Lock()
	defer ss.lk.Unlock()

	c, err := ffiwrapper.GeneratePieceCIDFromFile(mgr.proofType, r, size)
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("failed to generate piece cid: %w", err)
	}

	log.Warn("Generated Piece CID: ", c)

	ss.pieces = append(ss.pieces, c)
	return abi.PieceInfo{
		Size:     size.Padded(),
		PieceCID: c,
	}, nil
}

func (mgr *SectorMgr) SectorSize() abi.SectorSize {
	return mgr.sectorSize
}

func (mgr *SectorMgr) AcquireSectorNumber() (abi.SectorNumber, error) {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()
	id := mgr.nextSectorID
	mgr.nextSectorID++
	return id, nil
}

func (mgr *SectorMgr) SealPreCommit1(ctx context.Context, sid abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
	mgr.lk.Lock()
	ss, ok := mgr.sectors[sid]
	mgr.lk.Unlock()
	if !ok {
		return nil, xerrors.Errorf("no sector with id %d in storage", sid)
	}

	ss.lk.Lock()
	defer ss.lk.Unlock()

	ussize := abi.PaddedPieceSize(mgr.sectorSize).Unpadded()

	// TODO: verify pieces in sinfo.pieces match passed in pieces

	var sum abi.UnpaddedPieceSize
	for _, p := range pieces {
		sum += p.Size.Unpadded()
	}

	if sum != ussize {
		return nil, xerrors.Errorf("aggregated piece sizes don't match up: %d != %d", sum, ussize)
	}

	if ss.state != statePacking {
		return nil, xerrors.Errorf("cannot call pre-seal on sector not in 'packing' state")
	}

	opFinishWait(ctx)

	ss.state = statePreCommit

	pis := make([]abi.PieceInfo, len(ss.pieces))
	for i, piece := range ss.pieces {
		pis[i] = abi.PieceInfo{
			Size:     pieces[i].Size,
			PieceCID: piece,
		}
	}

	commd, err := MockVerifier.GenerateDataCommitment(mgr.proofType, pis)
	if err != nil {
		return nil, err
	}

	cc, _, err := commcid.CIDToCommitment(commd)
	if err != nil {
		panic(err)
	}

	cc[0] ^= 'd'

	return cc, nil
}

func (mgr *SectorMgr) SealPreCommit2(ctx context.Context, sid abi.SectorID, phase1Out storage.PreCommit1Out) (cids storage.SectorCids, err error) {
	db := []byte(string(phase1Out))
	db[0] ^= 'd'

	d := commcid.DataCommitmentV1ToCID(db)

	commr := make([]byte, 32)
	for i := range db {
		commr[32-(i+1)] = db[i]
	}

	commR := commcid.ReplicaCommitmentV1ToCID(commr)

	return storage.SectorCids{
		Unsealed: d,
		Sealed:   commR,
	}, nil
}

func (mgr *SectorMgr) SealCommit1(ctx context.Context, sid abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (output storage.Commit1Out, err error) {
	mgr.lk.Lock()
	ss, ok := mgr.sectors[sid]
	mgr.lk.Unlock()
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
		out[i] = cids.Unsealed.Bytes()[i] + cids.Sealed.Bytes()[31-i] - ticket[i]*seed[i] ^ byte(sid.Number&0xff)
	}

	return out[:], nil
}

func (mgr *SectorMgr) SealCommit2(ctx context.Context, sid abi.SectorID, phase1Out storage.Commit1Out) (proof storage.Proof, err error) {
	var out [32]byte
	for i := range out {
		out[i] = phase1Out[i] ^ byte(sid.Number&0xff)
	}

	return out[:], nil
}

// Test Instrumentation Methods

func (mgr *SectorMgr) FailSector(sid abi.SectorID) error {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()
	ss, ok := mgr.sectors[sid]
	if !ok {
		return fmt.Errorf("no such sector in storage")
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

func (mgr *SectorMgr) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []abi.SectorInfo, randomness abi.PoStRandomness) ([]abi.PoStProof, error) {
	return generateFakePoSt(sectorInfo), nil
}

func (mgr *SectorMgr) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []abi.SectorInfo, randomness abi.PoStRandomness) ([]abi.PoStProof, error) {
	return generateFakePoSt(sectorInfo), nil
}

func generateFakePoSt(sectorInfo []abi.SectorInfo) []abi.PoStProof {
	se, err := sectorInfo[0].RegisteredProof.WindowPoStPartitionSectors()
	if err != nil {
		panic(err)
	}
	return []abi.PoStProof{
		{
			RegisteredProof: sectorInfo[0].RegisteredProof,
			ProofBytes:      make([]byte, 192*int(math.Ceil(float64(len(sectorInfo))/float64(se)))),
		},
	}
}

func (mgr *SectorMgr) ReadPieceFromSealedSector(ctx context.Context, sectorID abi.SectorID, offset ffiwrapper.UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, commD cid.Cid) (io.ReadCloser, error) {
	if len(mgr.sectors[sectorID].pieces) > 1 {
		panic("implme")
	}
	return ioutil.NopCloser(io.LimitReader(bytes.NewReader(mgr.sectors[sectorID].pieces[0].Bytes()[offset:]), int64(size))), nil
}

func (mgr *SectorMgr) StageFakeData(mid abi.ActorID) (abi.SectorID, []abi.PieceInfo, error) {
	usize := abi.PaddedPieceSize(mgr.sectorSize).Unpadded()
	sid, err := mgr.AcquireSectorNumber()
	if err != nil {
		return abi.SectorID{}, nil, err
	}

	buf := make([]byte, usize)
	rand.Read(buf)

	id := abi.SectorID{
		Miner:  mid,
		Number: sid,
	}

	pi, err := mgr.AddPiece(context.TODO(), id, nil, usize, bytes.NewReader(buf))
	if err != nil {
		return abi.SectorID{}, nil, err
	}

	return id, []abi.PieceInfo{pi}, nil
}

func (mgr *SectorMgr) FinalizeSector(context.Context, abi.SectorID) error {
	return nil
}

func (mgr *SectorMgr) CheckProvable(context.Context, abi.RegisteredProof, []abi.SectorID) ([]abi.SectorID, error) {
	return nil, nil
}

func (m mockVerif) VerifySeal(svi abi.SealVerifyInfo) (bool, error) {
	if len(svi.Proof) != 32 { // Real ones are longer, but this should be fine
		return false, nil
	}

	for i, b := range svi.Proof {
		if b != svi.UnsealedCID.Bytes()[i]+svi.SealedCID.Bytes()[31-i]-svi.InteractiveRandomness[i]*svi.Randomness[i] {
			return false, nil
		}
	}

	return true, nil
}

func (m mockVerif) VerifyWinningPoSt(ctx context.Context, info abi.WinningPoStVerifyInfo) (bool, error) {
	return true, nil
}

func (m mockVerif) VerifyWindowPoSt(ctx context.Context, info abi.WindowPoStVerifyInfo) (bool, error) {
	return true, nil
}

func (m mockVerif) GenerateDataCommitment(pt abi.RegisteredProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	return ffiwrapper.GenerateUnsealedCID(pt, pieces)
}

func (m mockVerif) GenerateWinningPoStSectorChallenge(ctx context.Context, proofType abi.RegisteredProof, minerID abi.ActorID, randomness abi.PoStRandomness, eligibleSectorCount uint64) ([]uint64, error) {
	return []uint64{0}, nil
}

var MockVerifier = mockVerif{}

var _ ffiwrapper.Verifier = MockVerifier
