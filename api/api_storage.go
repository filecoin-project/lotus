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

	// Temp api for testing
	StoreGarbageData(context.Context) error

	// Get the status of a given sector by ID
	SectorsStatus(context.Context, uint64) (SectorInfo, error)

	// List all staged sectors
	SectorsList(context.Context) ([]uint64, error)

	SectorsRefs(context.Context) (map[string][]SealedRef, error)

	WorkerStats(context.Context) (WorkerStats, error)
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
