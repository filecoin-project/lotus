package sealer

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type apres struct {
	pi  abi.PieceInfo
	err error
}

type testExec struct {
	apch chan chan apres
}

func (t *testExec) DownloadSectorData(ctx context.Context, sector storiface.SectorRef, finalized bool, src map[storiface.SectorFileType]storiface.SectorLocation) error {
	panic("implement me")
}

func (t *testExec) DataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (abi.PieceInfo, error) {
	panic("implement me")
}

func (t *testExec) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	panic("implement me")
}

func (t *testExec) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, ppt abi.RegisteredPoStProof, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) (miner []proof.PoStProof, skipped []abi.SectorID, err error) {
	panic("implement me")
}

func (t *testExec) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte) ([]proof.PoStProof, error) {
	panic("implement me")
}

func (t *testExec) GenerateWindowPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte, partitionIdx int) (proof.PoStProof, error) {
	panic("implement me")
}

func (t *testExec) SealPreCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.PreCommit1Out, error) {
	panic("implement me")
}

func (t *testExec) SealPreCommit2(ctx context.Context, sector storiface.SectorRef, pc1o storiface.PreCommit1Out) (storiface.SectorCids, error) {
	panic("implement me")
}

func (t *testExec) SealCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storiface.SectorCids) (storiface.Commit1Out, error) {
	panic("implement me")
}

func (t *testExec) SealCommit2(ctx context.Context, sector storiface.SectorRef, c1o storiface.Commit1Out) (storiface.Proof, error) {
	panic("implement me")
}

func (t *testExec) FinalizeSector(ctx context.Context, sector storiface.SectorRef) error {
	panic("implement me")
}

func (t *testExec) ReleaseUnsealed(ctx context.Context, sector storiface.SectorRef, keepUnsealed []storiface.Range) error {
	panic("implement me")
}

func (t *testExec) ReleaseSealed(ctx context.Context, sector storiface.SectorRef) error {
	panic("implement me")
}

func (t *testExec) ReleaseSectorKey(ctx context.Context, sector storiface.SectorRef) error {
	panic("implement me")
}

func (t *testExec) ReleaseReplicaUpgrade(ctx context.Context, sector storiface.SectorRef) error {
	panic("implement me")
}

func (t *testExec) Remove(ctx context.Context, sector storiface.SectorRef) error {
	panic("implement me")
}

func (t *testExec) ReplicaUpdate(ctx context.Context, sector storiface.SectorRef, pieces []abi.PieceInfo) (storiface.ReplicaUpdateOut, error) {
	panic("implement me")
}

func (t *testExec) ProveReplicaUpdate1(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (storiface.ReplicaVanillaProofs, error) {
	panic("implement me")
}

func (t *testExec) ProveReplicaUpdate2(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storiface.ReplicaVanillaProofs) (storiface.ReplicaUpdateProof, error) {
	panic("implement me")
}

func (t *testExec) GenerateSectorKeyFromData(ctx context.Context, sector storiface.SectorRef, commD cid.Cid) error {
	panic("implement me")
}

func (t *testExec) FinalizeReplicaUpdate(ctx context.Context, sector storiface.SectorRef) error {
	panic("implement me")
}

func (t *testExec) NewSector(ctx context.Context, sector storiface.SectorRef) error {
	panic("implement me")
}

func (t *testExec) AddPiece(ctx context.Context, sector storiface.SectorRef, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (abi.PieceInfo, error) {
	resp := make(chan apres)
	t.apch <- resp
	ar := <-resp
	return ar.pi, ar.err
}

func (t *testExec) UnsealPiece(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd cid.Cid) error {
	panic("implement me")
}

func (t *testExec) ReadPiece(ctx context.Context, writer io.Writer, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error) {
	panic("implement me")
}

var _ storiface.Storage = &testExec{}
