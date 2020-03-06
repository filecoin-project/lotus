package sealmgr

import (
	"context"
	"io"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storedcounter"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"
)

type LocalWorker struct {
	sectorbuilder.Basic
}

var _ Worker = &LocalWorker{}

// Simple implements a very basic storage manager which has one local worker,
// running one thing locally
type Simple struct {
	sc    *storedcounter.StoredCounter
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

func NewSimpleManager(sc *storedcounter.StoredCounter, maddr address.Address, sb sectorbuilder.Basic) (*Simple, error) {
	w := &LocalWorker{
		sb,
	}

	return &Simple{
		sc:     sc,
		maddr:  maddr,
		worker: w,
	}, nil
}

func (s *Simple) NewSector() (abi.SectorNumber, error) {
	n, err := s.sc.Next()
	if err != nil {
		return 0, xerrors.Errorf("acquire sector number: %w", err)
	}

	return abi.SectorNumber(n), nil
}

func (s *Simple) AddPiece(ctx context.Context, sectorNum abi.SectorNumber, existingPieces []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r storage.Data) (abi.PieceInfo, error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.AddPiece(ctx, sectorNum, existingPieces, sz, r)
}

func (s *Simple) SealPreCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.SealPreCommit1(ctx, sectorNum, ticket, pieces)
}

func (s *Simple) SealPreCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out storage.PreCommit1Out) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.SealPreCommit2(ctx, sectorNum, phase1Out)
}

func (s *Simple) SealCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, sealedCID cid.Cid, unsealedCID cid.Cid) (output storage.Commit1Out, err error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.SealCommit1(ctx, sectorNum, ticket, seed, pieces, sealedCID, unsealedCID)
}

func (s *Simple) SealCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out storage.Commit1Out) (proof storage.Proof, err error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.SealCommit2(ctx, sectorNum, phase1Out)
}

func (s *Simple) FinalizeSector(ctx context.Context, sectorNum abi.SectorNumber) error {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.FinalizeSector(ctx, sectorNum)
}

func (s *Simple) GenerateEPostCandidates(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]storage.PoStCandidateWithTicket, error) {
	return s.worker.GenerateEPostCandidates(sectorInfo, challengeSeed, faults)
}

func (s *Simple) GenerateFallbackPoSt(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]storage.PoStCandidateWithTicket, []abi.PoStProof, error) {
	return s.worker.GenerateFallbackPoSt(sectorInfo, challengeSeed, faults)
}

func (s *Simple) ComputeElectionPoSt(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, winners []abi.PoStCandidate) ([]abi.PoStProof, error) {
	return s.worker.ComputeElectionPoSt(sectorInfo, challengeSeed, winners)
}

func (s *Simple) ReadPieceFromSealedSector(context.Context, abi.SectorNumber, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error) {
	panic("todo")
}

var _ Manager = &Simple{}
