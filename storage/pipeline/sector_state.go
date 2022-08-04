package sealing

import (
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
)

type SectorState string

type stateMeta struct {
	stat statSectorState

	sealDurationEstimate time.Duration
}

var ValidSectorStateList = map[SectorState]stateMeta{}

// this function should only be used to initialize new sector states
// DO NOT USE THIS TO cast strings to SectorState
func makeSectorState(name string, statState statSectorState, opts ...func(meta *stateMeta)) SectorState {
	st := SectorState(name)
	m := stateMeta{
		stat: statState,
	}

	for _, opt := range opts {
		opt(&m)
	}

	ValidSectorStateList[st] = m
	return st
}

func sealDurationEstimate(d time.Duration) func(meta *stateMeta) {
	return func(meta *stateMeta) {
		meta.sealDurationEstimate = d
	}
}

// rough seal timing estimates, used for estimating future pipeline state in pipeline stats
var (
	sealTimePC1      = 3*60*time.Minute + sealTimePC2
	sealTimePC2      = 30*time.Minute + sealTimeWaitSeed
	sealTimeWaitSeed = time.Duration(uint64(policy.GetPreCommitChallengeDelay())*build.BlockDelaySecs)*time.Second + sealTimeCommit
	sealTimeCommit   = 30*time.Minute + sealTimeFinalize
	sealTimeFinalize = 10 * time.Minute

	snapTime = 40 * time.Minute
)

// cmd/lotus-miner/info.go defines CLI colors/order corresponding to these states
// update files there when adding new states
var (
	UndefinedSectorState = makeSectorState("", sstStaging)

	// happy path
	Empty         = makeSectorState("Empty", sstStaging)                                                 // deprecated
	WaitDeals     = makeSectorState("WaitDeals", sstStaging)                                             // waiting for more pieces (deals) to be added to the sector
	AddPiece      = makeSectorState("AddPiece", sstStaging)                                              // put deal data (and padding if required) into the sector
	Packing       = makeSectorState("Packing", sstSealing, sealDurationEstimate(sealTimePC1))            // sector not in sealStore, and not on chain
	GetTicket     = makeSectorState("GetTicket", sstSealing, sealDurationEstimate(sealTimePC1))          // generate ticket
	PreCommit1    = makeSectorState("PreCommit1", sstSealing, sealDurationEstimate(sealTimePC1))         // do PreCommit1
	PreCommit2    = makeSectorState("PreCommit2", sstSealing, sealDurationEstimate(sealTimePC2))         // do PreCommit2
	PreCommitting = makeSectorState("PreCommitting", sstSealing, sealDurationEstimate(sealTimeWaitSeed)) // on chain pre-commit

	PreCommitWait        = makeSectorState("PreCommitWait", sstSealing, sealDurationEstimate(sealTimeWaitSeed)) // waiting for precommit to land on chain
	SubmitPreCommitBatch = makeSectorState("SubmitPreCommitBatch", sstSealing, sealDurationEstimate(sealTimeWaitSeed))

	PreCommitBatchWait = makeSectorState("PreCommitBatchWait", sstSealing, sealDurationEstimate(sealTimeWaitSeed))
	WaitSeed           = makeSectorState("WaitSeed", sstSealing, sealDurationEstimate(sealTimeWaitSeed)) // waiting for seed

	Committing           = makeSectorState("Committing", sstSealing, sealDurationEstimate(sealTimeCommit))       // compute PoRep
	CommitFinalize       = makeSectorState("CommitFinalize", sstSealing, sealDurationEstimate(sealTimeFinalize)) // cleanup sector metadata before submitting the proof (early finalize)
	CommitFinalizeFailed = makeSectorState("CommitFinalizeFailed", sstFailed)

	// single commit
	SubmitCommit = makeSectorState("SubmitCommit", sstLateSeal) // send commit message to the chain
	CommitWait   = makeSectorState("CommitWait", sstLateSeal)   // wait for the commit message to land on chain

	SubmitCommitAggregate = makeSectorState("SubmitCommitAggregate", sstLateSeal)
	CommitAggregateWait   = makeSectorState("CommitAggregateWait", sstLateSeal)

	FinalizeSector = makeSectorState("FinalizeSector", sstSealing, sealDurationEstimate(sealTimeFinalize))
	Proving        = makeSectorState("Proving", sstProving)
	Available      = makeSectorState("Available", sstProving) // proving CC available for SnapDeals

	// snap deals / cc update
	SnapDealsWaitDeals    = makeSectorState("SnapDealsWaitDeals", sstStaging)
	SnapDealsAddPiece     = makeSectorState("SnapDealsAddPiece", sstStaging)
	SnapDealsPacking      = makeSectorState("SnapDealsPacking", sstSealing, sealDurationEstimate(snapTime))
	UpdateReplica         = makeSectorState("UpdateReplica", sstSealing, sealDurationEstimate(snapTime))
	ProveReplicaUpdate    = makeSectorState("ProveReplicaUpdate", sstSealing, sealDurationEstimate(snapTime))
	SubmitReplicaUpdate   = makeSectorState("SubmitReplicaUpdate", sstLateSeal)
	ReplicaUpdateWait     = makeSectorState("ReplicaUpdateWait", sstLateSeal)
	FinalizeReplicaUpdate = makeSectorState("FinalizeReplicaUpdate", sstSealing, sealDurationEstimate(snapTime))
	UpdateActivating      = makeSectorState("UpdateActivating", sstProving)
	ReleaseSectorKey      = makeSectorState("ReleaseSectorKey", sstProving)

	// error modes
	FailedUnrecoverable  = makeSectorState("FailedUnrecoverable", sstFailed)
	AddPieceFailed       = makeSectorState("AddPieceFailed", sstStaging) // yes, we still consider this sector as staging for stats
	SealPreCommit1Failed = makeSectorState("SealPreCommit1Failed", sstFailed)
	SealPreCommit2Failed = makeSectorState("SealPreCommit2Failed", sstFailed)
	PreCommitFailed      = makeSectorState("PreCommitFailed", sstFailed)
	ComputeProofFailed   = makeSectorState("ComputeProofFailed", sstFailed)
	CommitFailed         = makeSectorState("CommitFailed", sstFailed)
	PackingFailed        = makeSectorState("PackingFailed", sstFailed) // TODO: deprecated, remove
	FinalizeFailed       = makeSectorState("FinalizeFailed", sstFailed)
	DealsExpired         = makeSectorState("DealsExpired", sstFailed)
	RecoverDealIDs       = makeSectorState("RecoverDealIDs", sstFailed)

	// snap deals error modes
	SnapDealsAddPieceFailed     = makeSectorState("SnapDealsAddPieceFailed", sstFailed)
	SnapDealsDealsExpired       = makeSectorState("SnapDealsDealsExpired", sstFailed)
	SnapDealsRecoverDealIDs     = makeSectorState("SnapDealsRecoverDealIDs", sstFailed)
	AbortUpgrade                = makeSectorState("AbortUpgrade", sstFailed)
	ReplicaUpdateFailed         = makeSectorState("ReplicaUpdateFailed", sstFailed)
	ReleaseSectorKeyFailed      = makeSectorState("ReleaseSectorKeyFailed", sstFailed)
	FinalizeReplicaUpdateFailed = makeSectorState("FinalizeReplicaUpdateFailed", sstFailed)

	Faulty            = makeSectorState("Faulty", sstFailed)        // sector is corrupted or gone for some reason
	FaultReported     = makeSectorState("FaultReported", sstFailed) // sector has been declared as a fault on chain
	FaultedFinal      = makeSectorState("FaultedFinal", sstFailed)  // fault declared on chain
	Terminating       = makeSectorState("Terminating", sstProving)
	TerminateWait     = makeSectorState("TerminateWait", sstProving)
	TerminateFinality = makeSectorState("TerminateFinality", sstProving)
	TerminateFailed   = makeSectorState("TerminateFailed", sstProving)

	Removing     = makeSectorState("Removing", sstProving)
	RemoveFailed = makeSectorState("RemoveFailed", sstFailed)
	Removed      = makeSectorState("Removed", sstProving)
)

func toStatState(st SectorState, finEarly bool) statSectorState {
	statState := ValidSectorStateList[st].stat

	if finEarly && statState == sstLateSeal {
		return sstProving
	} else if statState == sstLateSeal {
		statState = sstSealing
	}

	return statState
}

func IsUpgradeState(st SectorState) bool {
	switch st {
	case SnapDealsWaitDeals,
		SnapDealsAddPiece,
		SnapDealsPacking,
		UpdateReplica,
		ProveReplicaUpdate,
		SubmitReplicaUpdate,

		SnapDealsAddPieceFailed,
		SnapDealsDealsExpired,
		SnapDealsRecoverDealIDs,
		AbortUpgrade,
		ReplicaUpdateFailed,
		ReleaseSectorKeyFailed,
		FinalizeReplicaUpdateFailed:
		return true
	default:
		return false
	}
}
