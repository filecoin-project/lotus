package sealmgr

import (
	"context"
	"io"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/ipfs/go-cid"
)

type LocalWorker struct {
	sectorbuilder.Basic
}

var _ Worker = &LocalWorker{}

// Simple implements a very basic storage manager which has one local worker,
// running one thing locally
type Simple struct {
	maddr address.Address

	rateLimiter sync.Mutex
	worker      Worker
}

type sszgetter interface {
	SectorSize() abi.SectorSize
}

func (s *Simple) SectorSize() abi.SectorSize {
	return s.worker.(sszgetter).SectorSize()
}

func NewSimpleManager(maddr address.Address, sb sectorbuilder.Basic) (*Simple, error) {
	w := &LocalWorker{
		sb,
	}

	return &Simple{
		maddr:  maddr,
		worker: w,
	}, nil
}

func (s *Simple) NewSector(ctx context.Context, id abi.SectorID) error {
	return s.worker.NewSector(ctx, id)
}

func (s *Simple) AddPiece(ctx context.Context, id abi.SectorID, existingPieces []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r storage.Data) (abi.PieceInfo, error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.AddPiece(ctx, id, existingPieces, sz, r)
}

func (s *Simple) SealPreCommit1(ctx context.Context, id abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.SealPreCommit1(ctx, id, ticket, pieces)
}

func (s *Simple) SealPreCommit2(ctx context.Context, id abi.SectorID, phase1Out storage.PreCommit1Out) (cids storage.SectorCids, err error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.SealPreCommit2(ctx, id, phase1Out)
}

func (s *Simple) SealCommit1(ctx context.Context, id abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (output storage.Commit1Out, err error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.SealCommit1(ctx, id, ticket, seed, pieces, cids)
}

func (s *Simple) SealCommit2(ctx context.Context, id abi.SectorID, phase1Out storage.Commit1Out) (proof storage.Proof, err error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.SealCommit2(ctx, id, phase1Out)
}

func (s *Simple) FinalizeSector(ctx context.Context, id abi.SectorID) error {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.FinalizeSector(ctx, id)
}

func (s *Simple) GenerateEPostCandidates(ctx context.Context, miner abi.ActorID, sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]storage.PoStCandidateWithTicket, error) {
	return s.worker.GenerateEPostCandidates(ctx, miner, sectorInfo, challengeSeed, faults)
}

func (s *Simple) GenerateFallbackPoSt(ctx context.Context, miner abi.ActorID, sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) (storage.FallbackPostOut, error) {
	return s.worker.GenerateFallbackPoSt(ctx, miner, sectorInfo, challengeSeed, faults)
}

func (s *Simple) ComputeElectionPoSt(ctx context.Context, miner abi.ActorID, sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, winners []abi.PoStCandidate) ([]abi.PoStProof, error) {
	return s.worker.ComputeElectionPoSt(ctx, miner, sectorInfo, challengeSeed, winners)
}

func (s *Simple) ReadPieceFromSealedSector(context.Context, abi.SectorID, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error) {
	panic("todo")
}

var _ Manager = &Simple{}
