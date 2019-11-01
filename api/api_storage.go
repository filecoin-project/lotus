package api

import (
	"context"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

type SectorState int

const (
	Undefined SectorState = iota

	Empty   // TODO: Is this useful
	Packing // sector not in sealStore, and not on chain

	Unsealed      // sealing / queued
	PreCommitting // on chain pre-commit
	PreCommitted  // waiting for seed
	Committing
)

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddress(context.Context) (address.Address, error)

	// Temp api for testing
	StoreGarbageData(context.Context) error

	// Get the status of a given sector by ID
	SectorsStatus(context.Context, uint64) (sectorbuilder.SectorSealingStatus, error)

	// List all staged sectors
	SectorsList(context.Context) ([]uint64, error)

	SectorsRefs(context.Context) (map[string][]SealedRef, error)
}

type SealedRef struct {
	Piece  string
	Offset uint64
	Size   uint32
}
