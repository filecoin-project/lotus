package sealing

type SectorState string

const (
	UndefinedSectorState SectorState = ""

	// happy path
	Empty          SectorState = "Empty"
	Packing        SectorState = "Packing"       // sector not in sealStore, and not on chain
	PreCommit1     SectorState = "PreCommit1"    // do PreCommit1
	PreCommit2     SectorState = "PreCommit2"    // do PreCommit1
	PreCommitting  SectorState = "PreCommitting" // on chain pre-commit
	PreCommitWait  SectorState = "PreCommitWait" // waiting for precommit to land on chain
	WaitSeed       SectorState = "WaitSeed"      // waiting for seed
	Committing     SectorState = "Committing"
	CommitWait     SectorState = "CommitWait" // waiting for message to land on chain
	FinalizeSector SectorState = "FinalizeSector"
	Proving        SectorState = "Proving"
	// error modes
	FailedUnrecoverable SectorState = "FailedUnrecoverable"
	SealFailed          SectorState = "SealFailed"
	PreCommitFailed     SectorState = "PreCommitFailed"
	ComputeProofFailed  SectorState = "ComputeProofFailed"
	CommitFailed        SectorState = "CommitFailed"
	PackingFailed       SectorState = "PackingFailed"
	FinalizeFailed      SectorState = "FinalizeFailed"

	Faulty        SectorState = "Faulty"        // sector is corrupted or gone for some reason
	FaultReported SectorState = "FaultReported" // sector has been declared as a fault on chain
	FaultedFinal  SectorState = "FaultedFinal"  // fault declared on chain
)
