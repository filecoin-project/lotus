package mock

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-commp-utils/v2"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("sbmock")

type SectorMgr struct {
	sectors      map[abi.SectorID]*sectorState
	failPoSt     bool
	pieces       map[cid.Cid][]byte
	nextSectorID abi.SectorNumber

	lk sync.Mutex
}

type mockVerifProver struct {
	aggregates map[string]prooftypes.AggregateSealVerifyProofAndInfos // used for logging bad verifies
}

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

func (mgr *SectorMgr) NewSector(ctx context.Context, sector storiface.SectorRef) error {
	return nil
}

func (mgr *SectorMgr) SectorsUnsealPiece(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd *cid.Cid) error {
	panic("SectorMgr: unsealing piece: implement me")
}

func (mgr *SectorMgr) DataCid(ctx context.Context, size abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	panic("todo")
}

func (mgr *SectorMgr) AddPiece(ctx context.Context, sectorID storiface.SectorRef, existingPieces []abi.UnpaddedPieceSize, size abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	log.Warn("Add piece: ", sectorID, size, sectorID.ProofType)

	var b bytes.Buffer
	tr := io.TeeReader(r, &b)

	c, err := commp.GeneratePieceCIDFromFile(sectorID.ProofType, tr, size)
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

func (mgr *SectorMgr) IsUnsealed(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error) {
	return false, nil
}

func (mgr *SectorMgr) ForceState(sid storiface.SectorRef, st int) error {
	mgr.lk.Lock()
	ss, ok := mgr.sectors[sid.ID]
	mgr.lk.Unlock()
	if !ok {
		return xerrors.Errorf("no sector with id %d in storage", sid)
	}

	ss.state = st

	return nil
}

func (mgr *SectorMgr) SealPreCommit1(ctx context.Context, sid storiface.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storiface.PreCommit1Out, err error) {
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

	cc, err := commcid.CIDToDataCommitmentV1(commd)
	if err != nil {
		panic(err)
	}

	cc[0] ^= 'd'

	return cc, nil
}

func (mgr *SectorMgr) SealPreCommit2(ctx context.Context, sid storiface.SectorRef, phase1Out storiface.PreCommit1Out) (cids storiface.SectorCids, err error) {
	db := []byte(string(phase1Out))
	db[0] ^= 'd'

	d, _ := commcid.DataCommitmentV1ToCID(db)

	commr := make([]byte, 32)
	for i := range db {
		commr[32-(i+1)] = db[i]
	}

	commR, _ := commcid.ReplicaCommitmentV1ToCID(commr)

	return storiface.SectorCids{
		Unsealed: d,
		Sealed:   commR,
	}, nil
}

func (mgr *SectorMgr) SealCommit1(ctx context.Context, sid storiface.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storiface.SectorCids) (output storiface.Commit1Out, err error) {
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

func (mgr *SectorMgr) SealCommit2(ctx context.Context, sid storiface.SectorRef, phase1Out storiface.Commit1Out) (proof storiface.Proof, err error) {
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

func (mgr *SectorMgr) ReplicaUpdate(ctx context.Context, sid storiface.SectorRef, pieces []abi.PieceInfo) (storiface.ReplicaUpdateOut, error) {
	out := storiface.ReplicaUpdateOut{}
	return out, nil
}

func (mgr *SectorMgr) ProveReplicaUpdate1(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (storiface.ReplicaVanillaProofs, error) {
	out := make([][]byte, 0)
	return out, nil
}

func (mgr *SectorMgr) ProveReplicaUpdate2(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storiface.ReplicaVanillaProofs) (storiface.ReplicaUpdateProof, error) {
	return make([]byte, 0), nil
}

func (mgr *SectorMgr) GenerateSectorKeyFromData(ctx context.Context, sector storiface.SectorRef, commD cid.Cid) error {
	return nil
}

func (mgr *SectorMgr) ReleaseSealed(ctx context.Context, sid storiface.SectorRef) error {
	return nil
}

// Test Instrumentation Methods

func (mgr *SectorMgr) MarkFailed(sid storiface.SectorRef, failed bool) error {
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

func (mgr *SectorMgr) MarkCorrupted(sid storiface.SectorRef, corrupted bool) error {
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

func (mgr *SectorMgr) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, xSectorInfo []prooftypes.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]prooftypes.PoStProof, error) {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()

	sectorInfo := make([]prooftypes.SectorInfo, len(xSectorInfo))
	for i, xssi := range xSectorInfo {
		sectorInfo[i] = prooftypes.SectorInfo{
			SealProof:    xssi.SealProof,
			SectorNumber: xssi.SectorNumber,
			SealedCID:    xssi.SealedCID,
		}
	}

	ppt, err := sectorInfo[0].SealProof.RegisteredWinningPoStProof()
	if err != nil {
		panic(err)
	}

	return generateFakePoSt(sectorInfo, ppt, randomness), nil
}

func (mgr *SectorMgr) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, ppt abi.RegisteredPoStProof, xSectorInfo []prooftypes.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]prooftypes.PoStProof, []abi.SectorID, error) {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()

	if mgr.failPoSt {
		return nil, nil, xerrors.Errorf("failed to post (mock)")
	}

	si := make([]prooftypes.ExtendedSectorInfo, 0, len(xSectorInfo))

	var skipped []abi.SectorID

	var err error

	for _, xsi := range xSectorInfo {
		sid := abi.SectorID{
			Miner:  minerID,
			Number: xsi.SectorNumber,
		}

		_, found := mgr.sectors[sid]

		if found && !mgr.sectors[sid].failed && !mgr.sectors[sid].corrupted {
			si = append(si, xsi)
		} else {
			skipped = append(skipped, sid)
			err = xerrors.Errorf("skipped some sectors")
		}
	}

	if err != nil {
		return nil, skipped, err
	}

	sectorInfo := make([]prooftypes.SectorInfo, len(si))
	for i, xssi := range si {
		sectorInfo[i] = prooftypes.SectorInfo{
			SealProof:    xssi.SealProof,
			SectorNumber: xssi.SectorNumber,
			SealedCID:    xssi.SealedCID,
		}
	}

	return generateFakePoSt(sectorInfo, ppt, randomness), skipped, nil
}

func generateFakePoStProof(sectorInfo []prooftypes.SectorInfo, randomness abi.PoStRandomness) []byte {
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

func generateFakePoSt(sectorInfo []prooftypes.SectorInfo, ppt abi.RegisteredPoStProof, randomness abi.PoStRandomness) []prooftypes.PoStProof {
	return []prooftypes.PoStProof{
		{
			PoStProof:  ppt,
			ProofBytes: generateFakePoStProof(sectorInfo, randomness),
		},
	}
}

func (mgr *SectorMgr) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]prooftypes.PoStProof, error) {
	panic("implement me")
}

func (mgr *SectorMgr) GenerateWindowPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte, partitionIdx int) (prooftypes.PoStProof, error) {
	panic("implement me")
}

func (mgr *SectorMgr) ReadPiece(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealed cid.Cid) (storiface.Reader, bool, error) {
	off := storiface.UnpaddedByteIndex(0)
	var piece cid.Cid

	mgr.lk.Lock()

	for _, c := range mgr.sectors[sector.ID].pieces {
		piece = c
		if off >= offset {
			break
		}
		off += storiface.UnpaddedByteIndex(len(mgr.pieces[piece]))
	}
	if off > offset {
		panic("non-aligned offset todo")
	}
	br := bytes.NewReader(mgr.pieces[piece][:size])

	mgr.lk.Unlock()

	return struct {
		io.ReadCloser
		io.Seeker
		io.ReaderAt
	}{
		ReadCloser: io.NopCloser(br),
		Seeker:     br,
		ReaderAt:   br,
	}, false, nil
}

func (mgr *SectorMgr) StageFakeData(mid abi.ActorID, spt abi.RegisteredSealProof) (storiface.SectorRef, []abi.PieceInfo, error) {
	psize, err := spt.SectorSize()
	if err != nil {
		return storiface.SectorRef{}, nil, err
	}
	usize := abi.PaddedPieceSize(psize).Unpadded()
	sid, err := mgr.AcquireSectorNumber()
	if err != nil {
		return storiface.SectorRef{}, nil, err
	}

	buf := make([]byte, usize)
	_, _ = rand.Reader.Read(buf)

	id := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  mid,
			Number: sid,
		},
		ProofType: spt,
	}

	pi, err := mgr.AddPiece(context.TODO(), id, nil, usize, bytes.NewReader(buf))
	if err != nil {
		return storiface.SectorRef{}, nil, err
	}

	return id, []abi.PieceInfo{pi}, nil
}

func (mgr *SectorMgr) FinalizeSector(context.Context, storiface.SectorRef) error {
	return nil
}

func (mgr *SectorMgr) FinalizeReplicaUpdate(context.Context, storiface.SectorRef) error {
	return nil
}

func (mgr *SectorMgr) ReleaseUnsealed(ctx context.Context, sector storiface.SectorRef, keepUnsealed []storiface.Range) error {
	return nil
}

func (mgr *SectorMgr) ReleaseReplicaUpgrade(ctx context.Context, sector storiface.SectorRef) error {
	return nil
}

func (mgr *SectorMgr) ReleaseSectorKey(ctx context.Context, sector storiface.SectorRef) error {
	return nil
}

func (mgr *SectorMgr) DownloadSectorData(ctx context.Context, sector storiface.SectorRef, finalized bool, src map[storiface.SectorFileType]storiface.SectorLocation) error {
	return xerrors.Errorf("not supported")
}

func (mgr *SectorMgr) Remove(ctx context.Context, sector storiface.SectorRef) error {
	mgr.lk.Lock()
	defer mgr.lk.Unlock()

	if _, has := mgr.sectors[sector.ID]; !has {
		return xerrors.Errorf("sector not found")
	}

	delete(mgr.sectors, sector.ID)
	return nil
}

func (mgr *SectorMgr) CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, ids []storiface.SectorRef, rg storiface.RGetter) (map[abi.SectorID]string, error) {
	bad := map[abi.SectorID]string{}

	for _, sid := range ids {
		_, found := mgr.sectors[sid.ID]

		if !found || mgr.sectors[sid.ID].failed {
			bad[sid.ID] = "mock fail"
		}
	}

	return bad, nil
}

var _ storiface.WorkerReturn = &SectorMgr{}

func (mgr *SectorMgr) ReturnDataCid(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnAddPiece(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnSealPreCommit1(ctx context.Context, callID storiface.CallID, p1o storiface.PreCommit1Out, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnSealPreCommit2(ctx context.Context, callID storiface.CallID, sealed storiface.SectorCids, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnSealCommit1(ctx context.Context, callID storiface.CallID, out storiface.Commit1Out, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnSealCommit2(ctx context.Context, callID storiface.CallID, proof storiface.Proof, err *storiface.CallError) error {
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

func (mgr *SectorMgr) ReturnReplicaUpdate(ctx context.Context, callID storiface.CallID, out storiface.ReplicaUpdateOut, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnProveReplicaUpdate1(ctx context.Context, callID storiface.CallID, out storiface.ReplicaVanillaProofs, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnProveReplicaUpdate2(ctx context.Context, callID storiface.CallID, out storiface.ReplicaUpdateProof, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnGenerateSectorKeyFromData(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnFinalizeReplicaUpdate(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	panic("not supported")
}

func (mgr *SectorMgr) ReturnDownloadSector(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	panic("not supported")
}

func (m mockVerifProver) VerifySeal(svi prooftypes.SealVerifyInfo) (bool, error) {
	plen, err := svi.SealProof.ProofSize()
	if err != nil {
		return false, err
	}

	if len(svi.Proof) != int(plen) {
		return false, nil
	}

	// only the first 32 bytes, the rest are 0.
	for i, b := range svi.Proof[:32] {
		// unsealed+sealed-seed*ticket
		if b != svi.UnsealedCID.Bytes()[i]+svi.SealedCID.Bytes()[31-i]-svi.InteractiveRandomness[i]*svi.Randomness[i] {
			return false, nil
		}
	}

	return true, nil
}

func (m mockVerifProver) VerifyAggregateSeals(aggregate prooftypes.AggregateSealVerifyProofAndInfos) (bool, error) {
	out := make([]byte, m.aggLen(len(aggregate.Infos)))
	for pi, svi := range aggregate.Infos {
		for i := 0; i < 32; i++ {
			b := svi.UnsealedCID.Bytes()[i] + svi.SealedCID.Bytes()[31-i] - svi.InteractiveRandomness[i]*svi.Randomness[i] // raw proof byte

			b *= uint8(pi) // with aggregate index
			out[i] += b
		}
	}

	ok := bytes.Equal(aggregate.Proof, out)
	if !ok {
		genInfo, found := m.aggregates[string(aggregate.Proof)]
		if !found {
			log.Errorf("BAD AGGREGATE: saved generate inputs not found; agg.Proof: %x; expected: %x", aggregate.Proof, out)
		} else {
			log.Errorf("BAD AGGREGATE (1): agg.Proof: %x; expected: %x", aggregate.Proof, out)
			log.Errorf("BAD AGGREGATE (2): Verify   Infos: %+v", aggregate.Infos)
			log.Errorf("BAD AGGREGATE (3): Generate Infos: %+v", genInfo.Infos)
		}
	}

	return ok, nil
}

func (m mockVerifProver) VerifyReplicaUpdate(update prooftypes.ReplicaUpdateInfo) (bool, error) {
	return true, nil
}

func (m mockVerifProver) AggregateSealProofs(aggregateInfo prooftypes.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	out := make([]byte, m.aggLen(len(aggregateInfo.Infos))) // todo: figure out more real length
	for pi, proof := range proofs {
		for i := range proof[:32] {
			out[i] += proof[i] * uint8(pi)
		}
	}

	m.aggregates[string(out)] = aggregateInfo

	return out, nil
}

func (m mockVerifProver) aggLen(nproofs int) int {
	switch {
	case nproofs <= 8:
		return 11220
	case nproofs <= 16:
		return 14196
	case nproofs <= 32:
		return 17172
	case nproofs <= 64:
		return 20148
	case nproofs <= 128:
		return 23124
	case nproofs <= 256:
		return 26100
	case nproofs <= 512:
		return 29076
	case nproofs <= 1024:
		return 32052
	case nproofs <= 2048:
		return 35028
	case nproofs <= 4096:
		return 38004
	case nproofs <= 8192:
		return 40980
	default:
		panic("too many proofs")
	}
}

func (m mockVerifProver) VerifyWinningPoSt(ctx context.Context, info prooftypes.WinningPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return true, nil
}

func (m mockVerifProver) VerifyWindowPoSt(ctx context.Context, info prooftypes.WindowPoStVerifyInfo) (bool, error) {
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

func (m mockVerifProver) GenerateDataCommitment(pt abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	pcid, _, err := commp.PieceAggregateCommP(pt, pieces)
	return pcid, err
}

func (m mockVerifProver) GenerateWinningPoStSectorChallenge(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, eligibleSectorCount uint64) ([]uint64, error) {
	return []uint64{0}, nil
}

var MockVerifier = mockVerifProver{
	aggregates: map[string]prooftypes.AggregateSealVerifyProofAndInfos{},
}

var MockProver = MockVerifier

var _ storiface.Sealer = &SectorMgr{}
var _ storiface.Verifier = MockVerifier
var _ storiface.Prover = MockProver
