package advmgr

import (
	"context"
	"io"
	"sync"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealmgr"
)

type LocalStorage interface {
	GetStorage() (config.StorageConfig, error)
	SetStorage(config.StorageConfig) error
}

type Path struct {
	ID string
	Weight uint64

	LocalPath string

	CanSeal bool
	CanStore bool
}

type Worker interface {
	sealmgr.Worker

	Paths() []Path
}

type Manager struct {
	workers []sealmgr.Worker

	localLk sync.RWMutex
	localStorage LocalStorage
}

func (m Manager) SectorSize() abi.SectorSize {
	panic("implement me")
}

func (m Manager) NewSector() (abi.SectorNumber, error) {
	panic("implement me")
}

func (m Manager) ReadPieceFromSealedSector(context.Context, abi.SectorNumber, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error) {
	panic("implement me")
}

func (m Manager) AddPiece(context.Context, abi.UnpaddedPieceSize, abi.SectorNumber, io.Reader, []abi.UnpaddedPieceSize) (abi.PieceInfo, error) {
	panic("implement me")
}

func (m Manager) SealPreCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out []byte, err error) {
	panic("implement me")
}

func (m Manager) SealPreCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out []byte) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	panic("implement me")
}

func (m Manager) SealCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, sealedCID cid.Cid, unsealedCID cid.Cid) (output []byte, err error) {
	panic("implement me")
}

func (m Manager) SealCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out []byte) (proof []byte, err error) {
	panic("implement me")
}

func (m Manager) FinalizeSector(context.Context, abi.SectorNumber) error {
	panic("implement me")
}

func (m Manager) GenerateEPostCandidates(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]ffi.PoStCandidateWithTicket, error) {
	panic("implement me")
}

func (m Manager) GenerateFallbackPoSt(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]ffi.PoStCandidateWithTicket, []abi.PoStProof, error) {
	panic("implement me")
}

func (m Manager) ComputeElectionPoSt(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, winners []abi.PoStCandidate) ([]abi.PoStProof, error) {
	panic("implement me")
}

func New(ls LocalStorage) *Manager {
	return &Manager{
		workers:      nil,

		localStorage: ls,
	}
}

var _ sealmgr.Manager = &Manager{}
