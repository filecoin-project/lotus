package api

import (
	"context"

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
	CommitWait // waiting for message to land on chain
	Proving

	SealFailed
	PreCommitFailed
	SealCommitFailed
	CommitFailed

	FailedUnrecoverable
)

var SectorStates = []string{
	UndefinedSectorState: "UndefinedSectorState",
	Empty:                "Empty",
	Packing:              "Packing",
	Unsealed:             "Unsealed",
	PreCommitting:        "PreCommitting",
	PreCommitted:         "PreCommitted",
	Committing:           "Committing",
	Proving:              "Proving",

	SealFailed:       "SealFailed",
	PreCommitFailed:  "PreCommitFailed",
	SealCommitFailed: "SealCommitFailed",
	CommitFailed:     "CommitFailed",

	FailedUnrecoverable: "FailedUnrecoverable",
}

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddress(context.Context) (address.Address, error)

	ActorSectorSize(context.Context, address.Address) (uint64, error)

	// Temp api for testing
	StoreGarbageData(context.Context) error

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

type SectorInfo struct {
	SectorID uint64
	State    SectorState
	CommD    []byte
	CommR    []byte
	Proof    []byte
	Deals    []uint64
	Ticket   sectorbuilder.SealTicket
	Seed     sectorbuilder.SealSeed
	LastErr  string
}

type SealedRef struct {
	SectorID uint64
	Offset   uint64
	Size     uint64
}

type SealedRefs struct {
	Refs []SealedRef
}
