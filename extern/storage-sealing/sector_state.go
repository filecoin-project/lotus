package sealing

type SectorState string

var ExistSectorStateList = make(map[SectorState]struct{})

const (
	UndefinedSectorState SectorState = ""

	// happy path
	Empty          SectorState = "Empty"
	WaitDeals      SectorState = "WaitDeals"     // waiting for more pieces (deals) to be added to the sector
	Packing        SectorState = "Packing"       // sector not in sealStore, and not on chain
	PreCommit1     SectorState = "PreCommit1"    // do PreCommit1
	PreCommit2     SectorState = "PreCommit2"    // do PreCommit2
	PreCommitting  SectorState = "PreCommitting" // on chain pre-commit
	PreCommitWait  SectorState = "PreCommitWait" // waiting for precommit to land on chain
	WaitSeed       SectorState = "WaitSeed"      // waiting for seed
	Committing     SectorState = "Committing"    // compute PoRep
	SubmitCommit   SectorState = "SubmitCommit"  // send commit message to the chain
	CommitWait     SectorState = "CommitWait"    // wait for the commit message to land on chain
	FinalizeSector SectorState = "FinalizeSector"
	Proving        SectorState = "Proving"
	// error modes
	FailedUnrecoverable  SectorState = "FailedUnrecoverable"
	SealPreCommit1Failed SectorState = "SealPreCommit1Failed"
	SealPreCommit2Failed SectorState = "SealPreCommit2Failed"
	PreCommitFailed      SectorState = "PreCommitFailed"
	ComputeProofFailed   SectorState = "ComputeProofFailed"
	CommitFailed         SectorState = "CommitFailed"
	PackingFailed        SectorState = "PackingFailed" // TODO: deprecated, remove
	FinalizeFailed       SectorState = "FinalizeFailed"
	DealsExpired         SectorState = "DealsExpired"
	RecoverDealIDs       SectorState = "RecoverDealIDs"

	Faulty        SectorState = "Faulty"        // sector is corrupted or gone for some reason
	FaultReported SectorState = "FaultReported" // sector has been declared as a fault on chain
	FaultedFinal  SectorState = "FaultedFinal"  // fault declared on chain

	Removing     SectorState = "Removing"
	RemoveFailed SectorState = "RemoveFailed"
	Removed      SectorState = "Removed"
)
func init() {
	ExistSectorStateList[Empty] = struct{}{}
	ExistSectorStateList[WaitDeals] = struct{}{}
	ExistSectorStateList[Packing] = struct{}{}
	ExistSectorStateList[PreCommit1] = struct{}{}
	ExistSectorStateList[PreCommit2] = struct{}{}
	ExistSectorStateList[PreCommitting] = struct{}{}
	ExistSectorStateList[PreCommitWait] = struct{}{}
	ExistSectorStateList[WaitSeed] = struct{}{}
	ExistSectorStateList[Committing] = struct{}{}
	ExistSectorStateList[SubmitCommit] = struct{}{}
	ExistSectorStateList[CommitWait] = struct{}{}
	ExistSectorStateList[FinalizeSector] = struct{}{}
	ExistSectorStateList[Proving] = struct{}{}
	ExistSectorStateList[FailedUnrecoverable] = struct{}{}
	ExistSectorStateList[SealPreCommit1Failed] = struct{}{}
	ExistSectorStateList[SealPreCommit2Failed] = struct{}{}
	ExistSectorStateList[PreCommitFailed] = struct{}{}
	ExistSectorStateList[ComputeProofFailed] = struct{}{}
	ExistSectorStateList[CommitFailed] = struct{}{}
	ExistSectorStateList[PackingFailed] = struct{}{}
	ExistSectorStateList[FinalizeFailed] = struct{}{}
	ExistSectorStateList[DealsExpired] = struct{}{}
	ExistSectorStateList[RecoverDealIDs] = struct{}{}
	ExistSectorStateList[Faulty] = struct{}{}
	ExistSectorStateList[FaultReported] = struct{}{}
	ExistSectorStateList[FaultedFinal] = struct{}{}
	ExistSectorStateList[Removing] = struct{}{}
	ExistSectorStateList[RemoveFailed] = struct{}{}
	ExistSectorStateList[Removed] = struct{}{}
}

func toStatState(st SectorState) statSectorState {
	switch st {
	case Empty, WaitDeals, Packing, PreCommit1, PreCommit2, PreCommitting, PreCommitWait, WaitSeed, Committing, CommitWait, FinalizeSector:
		return sstSealing
	case Proving, Removed, Removing:
		return sstProving
	}

	return sstFailed
}
