package api

import (
	"bytes"
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/chain/types"
)

// alias because cbor-gen doesn't like non-alias types
type SectorState = uint64

const (
	UndefinedSectorState SectorState = iota

	// happy path
	Empty
	Packing // sector not in sealStore, and not on chain

	Unsealed      // sealing / queued
	PreCommitting // on chain pre-commit
	WaitSeed      // waiting for seed
	Committing
	CommitWait // waiting for message to land on chain
	FinalizeSector
	Proving
	_ // reserved
	_
	_

	// recovery handling
	// Reseal
	_
	_
	_
	_
	_
	_
	_

	// error modes
	FailedUnrecoverable

	SealFailed
	PreCommitFailed
	SealCommitFailed
	CommitFailed
	PackingFailed
	_
	_
	_

	Faulty        // sector is corrupted or gone for some reason
	FaultReported // sector has been declared as a fault on chain
	FaultedFinal  // fault declared on chain
)

var SectorStates = []string{
	UndefinedSectorState: "UndefinedSectorState",
	Empty:                "Empty",
	Packing:              "Packing",
	Unsealed:             "Unsealed",
	PreCommitting:        "PreCommitting",
	WaitSeed:             "WaitSeed",
	Committing:           "Committing",
	CommitWait:           "CommitWait",
	FinalizeSector:       "FinalizeSector",
	Proving:              "Proving",

	SealFailed:       "SealFailed",
	PreCommitFailed:  "PreCommitFailed",
	SealCommitFailed: "SealCommitFailed",
	CommitFailed:     "CommitFailed",
	PackingFailed:    "PackingFailed",

	FailedUnrecoverable: "FailedUnrecoverable",

	Faulty:        "Faulty",
	FaultReported: "FaultReported",
	FaultedFinal:  "FaultedFinal",
}

// StorageMiner is a low-level interface to the Filecoin network storage miner node
type StorageMiner interface {
	Common

	ActorAddress(context.Context) (address.Address, error)

	ActorSectorSize(context.Context, address.Address) (abi.SectorSize, error)

	// Temp api for testing
	PledgeSector(context.Context) error

	// Get the status of a given sector by ID
	SectorsStatus(context.Context, abi.SectorNumber) (SectorInfo, error)

	// List all staged sectors
	SectorsList(context.Context) ([]abi.SectorNumber, error)

	SectorsRefs(context.Context) (map[string][]SealedRef, error)

	SectorsUpdate(context.Context, abi.SectorNumber, SectorState) error

	/*WorkerStats(context.Context) (sealsched.WorkerStats, error)*/

	/*// WorkerQueue registers a remote worker
	WorkerQueue(context.Context, WorkerCfg) (<-chan WorkerTask, error)

	// WorkerQueue registers a remote worker
	WorkerQueue(context.Context, sectorbuilder.WorkerCfg) (<-chan sectorbuilder.WorkerTask, error)

	WorkerDone(ctx context.Context, task uint64, res sectorbuilder.SealRes) error
	*/
	MarketImportDealData(ctx context.Context, propcid cid.Cid, path string) error
	MarketListDeals(ctx context.Context) ([]storagemarket.StorageDeal, error)
	MarketListIncompleteDeals(ctx context.Context) ([]storagemarket.MinerDeal, error)
	SetPrice(context.Context, types.BigInt) error

	DealsImportData(ctx context.Context, dealPropCid cid.Cid, file string) error
	DealsList(ctx context.Context) ([]storagemarket.StorageDeal, error)

	StorageAddLocal(ctx context.Context, path string) error
}

type SealRes struct {
	Err   string
	GoErr error `json:"-"`

	Proof []byte
}

type SectorLog struct {
	Kind      string
	Timestamp uint64

	Trace string

	Message string
}

type SectorInfo struct {
	SectorID abi.SectorNumber
	State    SectorState
	CommD    *cid.Cid
	CommR    *cid.Cid
	Proof    []byte
	Deals    []abi.DealID
	Ticket   SealTicket
	Seed     SealSeed
	Retries  uint64

	LastErr string

	Log []SectorLog
}

type SealedRef struct {
	SectorID abi.SectorNumber
	Offset   uint64
	Size     abi.UnpaddedPieceSize
}

type SealedRefs struct {
	Refs []SealedRef
}

type SealTicket struct {
	Value abi.SealRandomness
	Epoch abi.ChainEpoch
}

type SealSeed struct {
	Value abi.InteractiveSealRandomness
	Epoch abi.ChainEpoch
}

func (st *SealTicket) Equals(ost *SealTicket) bool {
	return bytes.Equal(st.Value, ost.Value) && st.Epoch == ost.Epoch
}

func (st *SealSeed) Equals(ost *SealSeed) bool {
	return bytes.Equal(st.Value, ost.Value) && st.Epoch == ost.Epoch
}
