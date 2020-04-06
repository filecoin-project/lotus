package sealing

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
