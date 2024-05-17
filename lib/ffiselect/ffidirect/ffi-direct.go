package ffidirect

import (
	"os"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"
	"github.com/ipfs/go-cid"

	ffi "github.com/filecoin-project/filecoin-ffi"
)

// This allow reflection accesss to the FFI functions.
type FFI struct{}

// FFI.VerifySeal
func (FFI) VerifySeal(info proof.SealVerifyInfo) (bool, error) {
	return ffi.VerifySeal(info)
}

// FFI.VerifyAggregateSeals
func (FFI) VerifyAggregateSeals(aggregate proof.AggregateSealVerifyProofAndInfos) (bool, error) {
	return ffi.VerifyAggregateSeals(aggregate)
}

// FFI.VerifyWinningPoSt
func (FFI) VerifyWinningPoSt(info proof.WinningPoStVerifyInfo) (bool, error) {
	return ffi.VerifyWinningPoSt(info)
}

// FFI.VerifyWindowPoSt
func (FFI) VerifyWindowPoSt(info proof.WindowPoStVerifyInfo) (bool, error) {
	return ffi.VerifyWindowPoSt(info)
}

// FFI.GeneratePieceCID
func (FFI) GeneratePieceCID(proofType abi.RegisteredSealProof, piecePath string, pieceSize abi.UnpaddedPieceSize) (cid.Cid, error) {
	return ffi.GeneratePieceCID(proofType, piecePath, pieceSize)
}

// FFI.GenerateUnsealedCID
func (FFI) GenerateUnsealedCID(proofType abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	return ffi.GenerateUnsealedCID(proofType, pieces)
}

// FFI.GeneratePieceCIDFromFile
func (FFI) GeneratePieceCIDFromFile(proofType abi.RegisteredSealProof, pieceFile *os.File, pieceSize abi.UnpaddedPieceSize) (cid.Cid, error) {
	return ffi.GeneratePieceCIDFromFile(proofType, pieceFile, pieceSize)
}

// FFI.WriteWithAlignment
func (FFI) WriteWithAlignment(proofType abi.RegisteredSealProof, pieceFile *os.File, pieceBytes abi.UnpaddedPieceSize, stagedSectorFile *os.File, existingPieceSizes []abi.UnpaddedPieceSize) (leftAlignment, total abi.UnpaddedPieceSize, pieceCID cid.Cid, retErr error) {
	return ffi.WriteWithAlignment(proofType, pieceFile, pieceBytes, stagedSectorFile, existingPieceSizes)
}

// FFI.WriteWithoutAlignment
func (FFI) WriteWithoutAlignment(proofType abi.RegisteredSealProof, pieceFile *os.File, pieceBytes abi.UnpaddedPieceSize, stagedSectorFile *os.File) (abi.UnpaddedPieceSize, cid.Cid, error) {
	return ffi.WriteWithoutAlignment(proofType, pieceFile, pieceBytes, stagedSectorFile)
}

// FFI.SealPreCommitPhase1
func (FFI) SealPreCommitPhase1(proofType abi.RegisteredSealProof, cacheDirPath string, stagedSectorPath string, sealedSectorPath string, sectorNum abi.SectorNumber, minerID abi.ActorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (phase1Output []byte, err error) {
	return ffi.SealPreCommitPhase1(proofType, cacheDirPath, stagedSectorPath, sealedSectorPath, sectorNum, minerID, ticket, pieces)
}

// FFI.SealPreCommitPhase2
func (FFI) SealPreCommitPhase2(phase1Output []byte, cacheDirPath string, sealedSectorPath string) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	return ffi.SealPreCommitPhase2(phase1Output, cacheDirPath, sealedSectorPath)
}

// FFI.SealCommitPhase1
func (FFI) SealCommitPhase1(proofType abi.RegisteredSealProof, sealedCID cid.Cid, unsealedCID cid.Cid, cacheDirPath string, sealedSectorPath string, sectorNum abi.SectorNumber, minerID abi.ActorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo) (phase1Output []byte, err error) {
	return ffi.SealCommitPhase1(proofType, sealedCID, unsealedCID, cacheDirPath, sealedSectorPath, sectorNum, minerID, ticket, seed, pieces)
}

// FFI.SealCommitPhase2
func (FFI) SealCommitPhase2(phase1Output []byte, sectorNum abi.SectorNumber, minerID abi.ActorID) ([]byte, error) {
	return ffi.SealCommitPhase2(phase1Output, sectorNum, minerID)
}

// FFI.AggregateSealProofs
func (FFI) AggregateSealProofs(aggregateInfo proof.AggregateSealVerifyProofAndInfos, proofs [][]byte) (out []byte, err error) {
	return ffi.AggregateSealProofs(aggregateInfo, proofs)
}

// FFI.Unseal
func (FFI) Unseal(proofType abi.RegisteredSealProof, cacheDirPath string, sealedSector *os.File, unsealOutput *os.File, sectorNum abi.SectorNumber, minerID abi.ActorID, ticket abi.SealRandomness, unsealedCID cid.Cid) error {
	return ffi.Unseal(proofType, cacheDirPath, sealedSector, unsealOutput, sectorNum, minerID, ticket, unsealedCID)
}

// FFI.UnsealRange
func (FFI) UnsealRange(proofType abi.RegisteredSealProof, cacheDirPath string, sealedSector *os.File, unsealOutput *os.File, sectorNum abi.SectorNumber, minerID abi.ActorID, ticket abi.SealRandomness, unsealedCID cid.Cid, unpaddedByteIndex uint64, unpaddedBytesAmount uint64) error {
	return ffi.UnsealRange(proofType, cacheDirPath, sealedSector, unsealOutput, sectorNum, minerID, ticket, unsealedCID, unpaddedByteIndex, unpaddedBytesAmount)
}

// FFI.GenerateSDR
func (FFI) GenerateSDR(proofType abi.RegisteredSealProof, cacheDirPath string, replicaId [32]byte) (err error) {
	return ffi.GenerateSDR(proofType, cacheDirPath, replicaId)
}

// FFI.GenerateWinningPoStSectorChallenge
func (FFI) GenerateWinningPoStSectorChallenge(proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, eligibleSectorsLen uint64) ([]uint64, error) {
	return ffi.GenerateWinningPoStSectorChallenge(proofType, minerID, randomness, eligibleSectorsLen)
}

// FFI.GenerateWinningPoSt
func (FFI) GenerateWinningPoSt(minerID abi.ActorID, privateSectorInfo ffi.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	return ffi.GenerateWinningPoSt(minerID, privateSectorInfo, randomness)
}

// FFI.GenerateWindowPoSt
func (FFI) GenerateWindowPoSt(minerID abi.ActorID, privateSectorInfo ffi.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, []abi.SectorNumber, error) {
	return ffi.GenerateWindowPoSt(minerID, privateSectorInfo, randomness)
}

// FFI.GetGPUDevices
func (FFI) GetGPUDevices() ([]string, error) {
	return ffi.GetGPUDevices()
}

// FFI.GetSealVersion
func (FFI) GetSealVersion(proofType abi.RegisteredSealProof) (string, error) {
	return ffi.GetSealVersion(proofType)
}

// FFI.GetPoStVersion
func (FFI) GetPoStVersion(proofType abi.RegisteredPoStProof) (string, error) {
	return ffi.GetPoStVersion(proofType)
}

// FFI.GetNumPartitionForFallbackPost
func (FFI) GetNumPartitionForFallbackPost(proofType abi.RegisteredPoStProof, numSectors uint) (uint, error) {
	return ffi.GetNumPartitionForFallbackPost(proofType, numSectors)
}

// FFI.ClearCache
func (FFI) ClearCache(sectorSize uint64, cacheDirPath string) error {
	return ffi.ClearCache(sectorSize, cacheDirPath)
}

// FFI.ClearSyntheticProofs
func (FFI) ClearSyntheticProofs(sectorSize uint64, cacheDirPath string) error {
	return ffi.ClearSyntheticProofs(sectorSize, cacheDirPath)
}

// FFI.GenerateSynthProofs
func (FFI) GenerateSynthProofs(proofType abi.RegisteredSealProof, sealedCID, unsealedCID cid.Cid, cacheDirPath, replicaPath string, sector_id abi.SectorNumber, minerID abi.ActorID, ticket []byte, pieces []abi.PieceInfo) error {
	return ffi.GenerateSynthProofs(proofType, sealedCID, unsealedCID, cacheDirPath, replicaPath, sector_id, minerID, ticket, pieces)
}

// FFI.FauxRep
func (FFI) FauxRep(proofType abi.RegisteredSealProof, cacheDirPath string, sealedSectorPath string) (cid.Cid, error) {
	return ffi.FauxRep(proofType, cacheDirPath, sealedSectorPath)
}

// FFI.FauxRep2
func (FFI) FauxRep2(proofType abi.RegisteredSealProof, cacheDirPath string, existingPAuxPath string) (cid.Cid, error) {
	return ffi.FauxRep2(proofType, cacheDirPath, existingPAuxPath)
}
