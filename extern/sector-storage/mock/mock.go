package mock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand"
	"sync"

	proof2 "github.com/filecoin-project/specs-actors/v2/actors/runtime/proof"

	ffiwrapper2 "github.com/filecoin-project/go-commp-utils/ffiwrapper"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

var log = logging.Logger("sbmock")

type SectorMgr struct {
	sectors      map[abi.SectorID]*sectorState
	failPoSt     bool
	pieces       map[cid.Cid][]byte
	nextSectorID abi.SectorNumber

	lk sync.Mutex
}

type mockVerif struct{}

func NewMockSectorMgr(genesisSectors []abi.SectorID) *SectorMgr {
	sectors := make(map[abi.SectorID]*sectorState)
	for _, sid := range genesisSectors {
		sectors[sid] = &sectorState{
			failed: false,
			state:  stateCommit,
		}
	}

	return &SectorMgr{
		sectors:      sectors,
		pieces:       map[cid.Cid][]byte{},
		nextSectorID: 5,
	}
}

const (
	statePacking = iota
	statePreCommit
	stateCommit // nolint
)

type sectorState struct {
	pieces    []cid.Cid
	failed    bool
	corrupted bool

	state int

	lk sync.Mutex
}

func (mgr *SectorMgr) NewSector(ctx context.Context, sector storage.SectorRef) error {
	return nil
}

func (mgr *SectorMgr) AddPiece(ctx context.Context, sectorID storage.SectorRef, existingPieces []abi.UnpaddedPieceSize, size abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	log.Warn("Add piece: ", sectorID, size, sectorID.ProofType)

	var b bytes.Buffer
	tr := io.TeeReader(r, &b)

	c, err := ffiwrapper2.GeneratePieceCIDFromFile(sectorID.ProofType, tr, size)
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("failed to generate piece cid: %w", err)
	}

	log.Warn("Generated Piece CID: ", c)

	mgr.lk.Lock()
	mgr.pieces[c] = b.Bytes()

	ss, ok := mgr.sectors[sectorID.ID]
	if !ok {
		ss = &sectorState{
			state: statePacking,
		}
		mgr.sectors[sectorID.ID] = ss
	}
	mgr.lk.Unlock()

	ss.lk.Lock()
	ss.pieces = append(ss.pieces, c)
	ss.lk.Unlock()

	return abi.PieceInfo{

		Size:     size.Padded(),
		PieceCID: c,
	}, nil
}

func (mgr *SectorMgr) AcquireSectorNumber() (abi.SectorNumber, error) {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()
	id := mgr.nextSectorID
	mgr.nextSectorID++
	return id, nil
}

func (mgr *SectorMgr) ForceState(sid storage.SectorRef, st int) error {
	mgr.lk.Lock()
	ss, ok := mgr.sectors[sid.ID]
	mgr.lk.Unlock()
	if !ok {
		return xerrors.Errorf("no sector with id %d in storage", sid)
	}

	ss.state = st

	return nil
}

func (mgr *SectorMgr) SealPreCommit1(ctx context.Context, sid storage.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
	mgr.lk.Lock()
	ss, ok := mgr.sectors[sid.ID]
	mgr.lk.Unlock()
	if !ok {
		return nil, xerrors.Errorf("no sector with id %d in storage", sid)
	}

	ssize, err := sid.ProofType.SectorSize()
	if err != nil {
		return nil, xerrors.Errorf("failed to get proof sector size: %w", err)
	}

	ss.lk.Lock()
	defer ss.lk.Unlock()

	ussize := abi.PaddedPieceSize(ssize).Unpadded()

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

	commd, err := MockVerifier.GenerateDataCommitment(sid.ProofType, pis)
	if err != nil {
		return nil, err
	}

	_, _, cc, err := commcid.CIDToCommitment(commd)
	if err != nil {
		panic(err)
	}

	cc[0] ^= 'd'

	return cc, nil
}

func (mgr *SectorMgr) SealPreCommit2(ctx context.Context, sid storage.SectorRef, phase1Out storage.PreCommit1Out) (cids storage.SectorCids, err error) {
	db := []byte(string(phase1Out))
	db[0] ^= 'd'

	d, _ := commcid.DataCommitmentV1ToCID(db)

	commr := make([]byte, 32)
	for i := range db {
		commr[32-(i+1)] = db[i]
	}

	commR, _ := commcid.ReplicaCommitmentV1ToCID(commr)

	return storage.SectorCids{
		Unsealed: d,
		Sealed:   commR,
	}, nil
}

func (mgr *SectorMgr) SealCommit1(ctx context.Context, sid storage.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (output storage.Commit1Out, err error) {
	mgr.lk.Lock()
	ss, ok := mgr.sectors[sid.ID]
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
		out[i] = cids.Unsealed.Bytes()[i] + cids.Sealed.Bytes()[31-i] - ticket[i]*seed[i] ^ byte(sid.ID.Number&0xff)
	}

	return out[:], nil
}

func (mgr *SectorMgr) SealCommit2(ctx context.Context, sid storage.SectorRef, phase1Out storage.Commit1Out) (proof storage.Proof, err error) {
	plen, err := sid.ProofType.ProofSize()
	if err != nil {
		return nil, err
	}

	out := make([]byte, plen)
	for i := range out[:len(phase1Out)] {
		out[i] = phase1Out[i] ^ byte(sid.ID.Number&0xff)
	}

	return out[:], nil
}

// Test Instrumentation Methods

func (mgr *SectorMgr) MarkFailed(sid storage.SectorRef, failed bool) error {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()
	ss, ok := mgr.sectors[sid.ID]
	if !ok {
		return fmt.Errorf("no such sector in storage")
	}

	ss.failed = failed
	return nil
}

func (mgr *SectorMgr) Fail() {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()
	mgr.failPoSt = true

	return
}

func (mgr *SectorMgr) MarkCorrupted(sid storage.SectorRef, corrupted bool) error {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()
	ss, ok := mgr.sectors[sid.ID]
	if !ok {
		return fmt.Errorf("no such sector in storage")
	}

	ss.corrupted = corrupted
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

	return context.WithValue(ctx, "opfinish", done), func() { // nolint
		close(done)
	}
}

func (mgr *SectorMgr) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof2.SectorInfo, randomness abi.PoStRandomness) ([]proof2.PoStProof, error) {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()

	return generateFakePoSt(sectorInfo, abi.RegisteredSealProof.RegisteredWinningPoStProof, randomness), nil
}

func (mgr *SectorMgr) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof2.SectorInfo, randomness abi.PoStRandomness) ([]proof2.PoStProof, []abi.SectorID, error) {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()

	if mgr.failPoSt {
		return nil, nil, xerrors.Errorf("failed to post (mock)")
	}

	si := make([]proof2.SectorInfo, 0, len(sectorInfo))
	var skipped []abi.SectorID

	var err error

	for _, info := range sectorInfo {
		sid := abi.SectorID{
			Miner:  minerID,
			Number: info.SectorNumber,
		}

		_, found := mgr.sectors[sid]

		if found && !mgr.sectors[sid].failed && !mgr.sectors[sid].corrupted {
			si = append(si, info)
		} else {
			skipped = append(skipped, sid)
			err = xerrors.Errorf("skipped some sectors")
		}
	}

	if err != nil {
		return nil, skipped, err
	}

	return generateFakePoSt(si, abi.RegisteredSealProof.RegisteredWindowPoStProof, randomness), skipped, nil
}

func generateFakePoStProof(sectorInfo []proof2.SectorInfo, randomness abi.PoStRandomness) []byte {
	randomness[31] &= 0x3f

	hasher := sha256.New()
	_, _ = hasher.Write(randomness)
	for _, info := range sectorInfo {
		err := info.MarshalCBOR(hasher)
		if err != nil {
			panic(err)
		}
	}
	return hasher.Sum(nil)

}

func generateFakePoSt(sectorInfo []proof2.SectorInfo, rpt func(abi.RegisteredSealProof) (abi.RegisteredPoStProof, error), randomness abi.PoStRandomness) []proof2.PoStProof {
	wp, err := rpt(sectorInfo[0].SealProof)
	if err != nil {
		panic(err)
	}

	return []proof2.PoStProof{
		{
			PoStProof:  wp,
			ProofBytes: generateFakePoStProof(sectorInfo, randomness),
		},
	}
}

func (mgr *SectorMgr) ReadPiece(ctx context.Context, w io.Writer, sectorID storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, c cid.Cid) error {
	if offset != 0 {
		panic("implme")
	}

	_, err := io.CopyN(w, bytes.NewReader(mgr.pieces[mgr.sectors[sectorID.ID].pieces[0]]), int64(size))
	return err
}

func (mgr *SectorMgr) StageFakeData(mid abi.ActorID, spt abi.RegisteredSealProof) (storage.SectorRef, []abi.PieceInfo, error) {
	psize, err := spt.SectorSize()
	if err != nil {
		return storage.SectorRef{}, nil, err
	}
	usize := abi.PaddedPieceSize(psize).Unpadded()
	sid, err := mgr.AcquireSectorNumber()
	if err != nil {
		return storage.SectorRef{}, nil, err
	}

	buf := make([]byte, usize)
	_, _ = rand.Read(buf) // nolint:gosec

	id := storage.SectorRef{
		ID: abi.SectorID{
			Miner:  mid,
			Number: sid,
		},
		ProofType: spt,
	}

	pi, err := mgr.AddPiece(context.TODO(), id, nil, usize, bytes.NewReader(buf))
	if err != nil {
		return storage.SectorRef{}, nil, err
	}

	return id, []abi.PieceInfo{pi}, nil
}

func (mgr *SectorMgr) FinalizeSector(context.Context, storage.SectorRef, []storage.Range) error {
	return nil
}

func (mgr *SectorMgr) ReleaseUnsealed(ctx context.Context, sector storage.SectorRef, safeToFree []storage.Range) error {
	return nil
}

func (mgr *SectorMgr) Remove(ctx context.Context, sector storage.SectorRef) error {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()

	if _, has := mgr.sectors[sector.ID]; !has {
		return xerrors.Errorf("sector not found")
	}

	delete(mgr.sectors, sector.ID)
	return nil
}

func (mgr *SectorMgr) CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, ids []storage.SectorRef, rg storiface.RGetter) (map[abi.SectorID]string, error) {
	bad := map[abi.SectorID]string{}

	for _, sid := range ids {
		_, found := mgr.sectors[sid.ID]

		if !found || mgr.sectors[sid.ID].failed {
			bad[sid.ID] = "mock fail"
		}
	}

	return bad, nil
}

func (mgr *SectorMgr) ReturnAddPiece(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnSealPreCommit1(ctx context.Context, callID storiface.CallID, p1o storage.PreCommit1Out, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnSealPreCommit2(ctx context.Context, callID storiface.CallID, sealed storage.SectorCids, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnSealCommit1(ctx context.Context, callID storiface.CallID, out storage.Commit1Out, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnSealCommit2(ctx context.Context, callID storiface.CallID, proof storage.Proof, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnFinalizeSector(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnReleaseUnsealed(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnMoveStorage(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnUnsealPiece(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnReadPiece(ctx context.Context, callID storiface.CallID, ok bool, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnFetch(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	panic("not supported")
}

func (m mockVerif) VerifySeal(svi proof2.SealVerifyInfo) (bool, error) {
	plen, err := svi.SealProof.ProofSize()
	if err != nil {
		return false, err
	}

	if len(svi.Proof) != int(plen) {
		return false, nil
	}

	// only the first 32 bytes, the rest are 0.
	for i, b := range svi.Proof[:32] {
		if b != svi.UnsealedCID.Bytes()[i]+svi.SealedCID.Bytes()[31-i]-svi.InteractiveRandomness[i]*svi.Randomness[i] {
			return false, nil
		}
	}

	return true, nil
}

func (m mockVerif) VerifyWinningPoSt(ctx context.Context, info proof2.WinningPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return true, nil
}

func (m mockVerif) VerifyWindowPoSt(ctx context.Context, info proof2.WindowPoStVerifyInfo) (bool, error) {
	if len(info.Proofs) != 1 {
		return false, xerrors.Errorf("expected 1 proof entry")
	}

	proof := info.Proofs[0]

	expected := generateFakePoStProof(info.ChallengedSectors, info.Randomness)
	if !bytes.Equal(proof.ProofBytes, expected) {
		return false, xerrors.Errorf("bad proof")
	}
	return true, nil
}

func (m mockVerif) GenerateDataCommitment(pt abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	return ffiwrapper.GenerateUnsealedCID(pt, pieces)
}

func (m mockVerif) GenerateWinningPoStSectorChallenge(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, eligibleSectorCount uint64) ([]uint64, error) {
	return []uint64{0}, nil
}

var MockVerifier = mockVerif{}

var _ storage.Sealer = &SectorMgr{}
var _ ffiwrapper.Verifier = MockVerifier
