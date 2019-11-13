package smm

import (
    "github.com/filecoin-project/lotus/api"
    "github.com/ipfs/go-cid"
)

type Address string
type BitField map[uint64]struct{}

type SectorState int
const (
    Empty SectorState = iota // new sector
    Packing                  // deals can be added to the sector
    Unsealed                 // sector is full, waiting for sealing
    PreCommitting            // first step in sealing
    PreCommitted             // pre-commit to ticket complete - waiting for seed
    Committing               // second step in sealing
    Proving                  // sector is sealed with no faults
    NotProving               // sector is sealed with fault(s)
    Deleted                  // unrecoverable fault(s) or deals expired
)

type SectorID uint64

type SectorStateInfo struct {
    State    SectorState
    Error    error
    Progress uint
    MaxBytes uint64 // the max. quantity of bit-padded bytes that this sector can contain
    RemBytes uint64 // the quantity of additional bit-padded bytes that can fit into this sector
}

type Sector struct {
    SectorId uint64
    Deals    []StorageDealInfo
    State    SectorState
}

type StorageMiningState struct {
    //ModuleMiningState StorageMiningModuleState
    Sectors           []Sector
}

type StorageDealInfo struct {
    DealID   cid.Cid    // on-chain deal ID
    CommP    cid.Cid    // merkle root of preprocessed piece-data
    Size     uint64    // number of bytes in piece
    Path     string    // locator of piece data in filesystem
    Expiry   uint64    // unix epoch
}

type Epoch uint64    // aka “height” or “round number”
type StateKey string // an opaque unique state identifier (the state root CID)
type SealSeed []byte
type Proof []byte

type ProvingPeriod struct {
    Start          Epoch  // First epoch in the period
    End            Epoch  // Last epoch in the period
}

type MinerChainState struct {
    // From StorageMinerActor
    Address                Address
    PreCommittedSectors    map[uint64]api.ChainSectorInfo
    Sectors                map[uint64]api.ChainSectorInfo
    StagedCommittedSectors map[uint64]api.ChainSectorInfo
    ProvingSet             BitField

    // From StoragePowerActor
    Power                  uint64
}

type MiningStateEvents interface {
    OnSectorChanged(stateInfo SectorStateInfo)
    OnProvingPeriodChanged(provingPeriod ProvingPeriod)
}

// move out functions, transform to struct
type StorageMiningModule interface {
    // Starts the mining state machine
    StartMining() (StorageMiningState, error)
    // Stops the state machine, pausing any in-process seal
    StopMining() error

    // Register listener
    RegisterStorageMiningListener(MiningStateEvents) uint64
    // Unregister listener
    UnregisterStorageMiningListener(uint64)

    // Provides a new deal to be staged and sealed
    AddStorageDeal(deal StorageDealInfo) error
    // Exposes state for operator information
    GetState() StorageMiningState

    // TODO: expose information about the deals that don’t appear
    // in staged sectors yet, local state vs on-chain state.

    // Informs the module that a sector is unavailable.
    // The module will forward this information to the chain.
    SectorFailed(SectorID) error

    // Informs the module that a failed sector is now available.
    // The module will forward this information to the chain.
    SectorRecovered(SectorID) error

    // Un-seals a sector and writes all piece payloads to the
    // filesystem under path/hex(CommP).
    UnsealSector(id SectorID, path string)

}
