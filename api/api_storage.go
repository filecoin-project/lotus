package api

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

// alias because cbor-gen doesn't like non-alias types
type SectorState = uint64

const (
	UndefinedSectorState SectorState = iota

	Empty   // TODO: Is this useful
	Packing // sector not in sealStore, and not on chain

	Unsealed      // sealing / queued
	PreCommitting // on chain pre-commit
	PreCommitted  // waiting for seed
	Committing
	Proving

	SectorNoUpdate = UndefinedSectorState
)

func SectorStateStr(s SectorState) string {
	switch s {
	case UndefinedSectorState:
		return "UndefinedSectorState"
	case Empty:
		return "Empty"
	case Packing:
		return "Packing"
	case Unsealed:
		return "Unsealed"
	case PreCommitting:
		return "PreCommitting"
	case PreCommitted:
		return "PreCommitted"
	case Committing:
		return "Committing"
	case Proving:
		return "Proving"
	}
	return fmt.Sprintf("<Unknown %d>", s)
}

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddress(context.Context) (address.Address, error)

	// Temp api for testing
	StoreGarbageData(context.Context) error

	// Get the status of a given sector by ID
	SectorsStatus(context.Context, uint64) (SectorInfo, error)

	// List all staged sectors
	SectorsList(context.Context) ([]uint64, error)

	SectorsRefs(context.Context) (map[string][]SealedRef, error)

	WorkerStats(context.Context) (WorkerStats, error)

	// WorkerQueue registers a remote worker
	WorkerQueue(context.Context) (<-chan sectorbuilder.WorkerTask, error)

	WorkerDone(ctx context.Context, task uint64, res sectorbuilder.SealRes) error
}

type WorkerStats struct {
	Free     int
	Reserved int // for PoSt
	Total    int
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
}

type SealedRef struct {
	Piece  string
	Offset uint64
	Size   uint64
}

type SealedRefs struct {
	Refs []SealedRef
}
