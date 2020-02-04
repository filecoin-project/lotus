package api

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	s2 "github.com/filecoin-project/go-storage-miner"
)

type SectorState = s2.SectorState

var SectorStates = s2.SectorStates

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddress(context.Context) (address.Address, error)

	ActorSectorSize(context.Context, address.Address) (uint64, error)

	// Temp api for testing
	PledgeSector(context.Context) error

	// Get the status of a given sector by ID
	SectorsStatus(context.Context, uint64) (SectorInfo, error)

	// List all staged sectors
	SectorsList(context.Context) ([]uint64, error)

	SectorsRefs(context.Context) (map[string][]SealedRef, error)

	SectorsUpdate(context.Context, uint64, SectorState) error

	WorkerStats(context.Context) (sectorbuilder.WorkerStats, error)

	// WorkerQueue registers a remote worker
	WorkerQueue(context.Context, sectorbuilder.WorkerCfg) (<-chan sectorbuilder.WorkerTask, error)

	WorkerDone(ctx context.Context, task uint64, res sectorbuilder.SealRes) error
}

type SectorLog struct {
	Kind      string
	Timestamp uint64

	Trace string

	Message string
}

type SectorInfo struct {
	SectorID uint64
	State    SectorState
	CommD    []byte
	CommR    []byte
	Proof    []byte
	Deals    []uint64
	Ticket   sectorbuilder.SealTicket
	Seed     sectorbuilder.SealSeed
	Retries  uint64

	LastErr string

	Log []SectorLog
}

type SealedRef struct {
	SectorID uint64
	Offset   uint64
	Size     uint64
}

type SealedRefs struct {
	Refs []SealedRef
}
