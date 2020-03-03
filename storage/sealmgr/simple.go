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

	ffi "github.com/filecoin-project/filecoin-ffi"
)

// Simple implements a very basic storage manager which has one local worker,
// running one thing locally
type Simple struct {
	sc    *storedcounter.StoredCounter
	maddr address.Address

	rateLimiter sync.Mutex
	worker      Worker
}

func NewSimpleManager(sc *storedcounter.StoredCounter, maddr address.Address, sb sectorbuilder.Basic) (*Simple, error) {
	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return nil, xerrors.Errorf("get miner id: %w", err)
	}

	w := &LocalWorker{
		sealer: sb,
		mid:    abi.ActorID(mid),
	}

	return &Simple{
		sc:     sc,
		maddr:  maddr,
		worker: w,
	}, nil
}

func (s *Simple) NewSector() (SectorInfo, error) {
	n, err := s.sc.Next()
	if err != nil {
		return SectorInfo{}, xerrors.Errorf("acquire sector number: %w", err)
	}

	mid, err := address.IDFromAddress(s.maddr)
	if err != nil {
		return SectorInfo{}, xerrors.Errorf("get miner id: %w", err)
	}

	return SectorInfo{
		ID: abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: abi.SectorNumber(n),
		},
	}, nil
}

func (s *Simple) AddPiece(ctx context.Context, si SectorInfo, sz abi.UnpaddedPieceSize, r io.Reader) (cid.Cid, SectorInfo, error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.AddPiece(ctx, si, sz, r)
}

func (s *Simple) RunSeal(ctx context.Context, task TaskType, si SectorInfo) (SectorInfo, error) {
	s.rateLimiter.Lock()
	defer s.rateLimiter.Unlock()

	return s.worker.Run(ctx, task, si)
}

func (s *Simple) GenerateEPostCandidates(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]ffi.PoStCandidateWithTicket, error) {
	return s.worker.(*LocalWorker).sealer.GenerateEPostCandidates(sectorInfo, challengeSeed, faults)
}

func (s *Simple) GenerateFallbackPoSt(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, faults []abi.SectorNumber) ([]ffi.PoStCandidateWithTicket, []abi.PoStProof, error) {
	return s.worker.(*LocalWorker).sealer.GenerateFallbackPoSt(sectorInfo, challengeSeed, faults)
}

func (s *Simple) ComputeElectionPoSt(sectorInfo []abi.SectorInfo, challengeSeed abi.PoStRandomness, winners []abi.PoStCandidate) ([]abi.PoStProof, error) {
	return s.worker.(*LocalWorker).sealer.ComputeElectionPoSt(sectorInfo, challengeSeed, winners)
}

var _ Manager = &Simple{}
