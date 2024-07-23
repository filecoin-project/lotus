package sealing

import (
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type mutator interface {
	apply(state *SectorInfo)
}

// globalMutator is an event which can apply in every state
type globalMutator interface {
	// applyGlobal applies the event to the state. If it returns true,
	//  event processing should be interrupted
	applyGlobal(state *SectorInfo) bool
}

type Ignorable interface {
	Ignore()
}

// Global events

type SectorRestart struct{}

func (evt SectorRestart) applyGlobal(*SectorInfo) bool { return false }

type SectorFatalError struct{ error }

func (evt SectorFatalError) FormatError(xerrors.Printer) (next error) { return evt.error }

func (evt SectorFatalError) applyGlobal(state *SectorInfo) bool {
	log.Errorf("Fatal error on sector %d: %+v", state.SectorNumber, evt.error)
	// TODO: Do we want to mark the state as unrecoverable?
	//  I feel like this should be a softer error, where the user would
	//  be able to send a retry event of some kind
	return true
}

type SectorForceState struct {
	State SectorState
}

func (evt SectorForceState) applyGlobal(state *SectorInfo) bool {
	state.State = evt.State
	return true
}

// Normal path

type SectorStart struct {
	ID         abi.SectorNumber
	SectorType abi.RegisteredSealProof
}

func (evt SectorStart) apply(state *SectorInfo) {
	state.SectorNumber = evt.ID
	state.SectorType = evt.SectorType
}

type SectorStartCC struct {
	ID         abi.SectorNumber
	SectorType abi.RegisteredSealProof
}

func (evt SectorStartCC) apply(state *SectorInfo) {
	state.SectorNumber = evt.ID
	state.SectorType = evt.SectorType
}

type SectorAddPiece struct{}

func (evt SectorAddPiece) apply(state *SectorInfo) {
	if state.CreationTime == 0 {
		state.CreationTime = time.Now().Unix()
	}
}

type SectorPieceAdded struct {
	NewPieces []SafeSectorPiece
}

func (evt SectorPieceAdded) apply(state *SectorInfo) {
	state.Pieces = append(state.Pieces, evt.NewPieces...)
}

type SectorAddPieceFailed struct{ error }

func (evt SectorAddPieceFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorAddPieceFailed) apply(si *SectorInfo)                     {}

type SectorRetryWaitDeals struct{}

func (evt SectorRetryWaitDeals) apply(si *SectorInfo) {}

type SectorStartPacking struct{}

func (evt SectorStartPacking) apply(*SectorInfo) {}

func (evt SectorStartPacking) Ignore() {}

type SectorPacked struct{ FillerPieces []abi.PieceInfo }

func (evt SectorPacked) apply(state *SectorInfo) {
	for idx := range evt.FillerPieces {
		state.Pieces = append(state.Pieces, SafeSectorPiece{
			real: api.SectorPiece{
				Piece:    evt.FillerPieces[idx],
				DealInfo: nil, // filler pieces don't have deals associated with them
			},
		})
	}
}

type SectorTicket struct {
	TicketValue abi.SealRandomness
	TicketEpoch abi.ChainEpoch
}

func (evt SectorTicket) apply(state *SectorInfo) {
	state.TicketEpoch = evt.TicketEpoch
	state.TicketValue = evt.TicketValue
}

type SectorOldTicket struct{}

func (evt SectorOldTicket) apply(*SectorInfo) {}

type SectorPreCommit1 struct {
	PreCommit1Out storiface.PreCommit1Out
}

func (evt SectorPreCommit1) apply(state *SectorInfo) {
	state.PreCommit1Out = evt.PreCommit1Out
	state.PreCommit2Fails = 0
}

type SectorPreCommit2 struct {
	Sealed   cid.Cid
	Unsealed cid.Cid
}

func (evt SectorPreCommit2) apply(state *SectorInfo) {
	commd := evt.Unsealed
	state.CommD = &commd
	commr := evt.Sealed
	state.CommR = &commr
}

type SectorPreCommitBatch struct{}

func (evt SectorPreCommitBatch) apply(*SectorInfo) {}

type SectorPreCommitBatchSent struct {
	Message cid.Cid
}

func (evt SectorPreCommitBatchSent) apply(state *SectorInfo) {
	state.PreCommitMessage = &evt.Message
}

type SectorPreCommitLanded struct {
	TipSet types.TipSetKey
}

func (evt SectorPreCommitLanded) apply(si *SectorInfo) {
	si.PreCommitTipSet = evt.TipSet
}

type SectorSealPreCommit1Failed struct{ error }

func (evt SectorSealPreCommit1Failed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorSealPreCommit1Failed) apply(si *SectorInfo) {
	si.InvalidProofs = 0 // reset counter
	si.PreCommit2Fails = 0

	si.PreCommit1Fails++
}

type SectorSealPreCommit2Failed struct{ error }

func (evt SectorSealPreCommit2Failed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorSealPreCommit2Failed) apply(si *SectorInfo) {
	si.InvalidProofs = 0 // reset counter
	si.PreCommit2Fails++
}

type SectorChainPreCommitFailed struct{ error }

func (evt SectorChainPreCommitFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorChainPreCommitFailed) apply(*SectorInfo)                        {}

type SectorPreCommitted struct {
	Message          cid.Cid
	PreCommitDeposit big.Int
	PreCommitInfo    miner.SectorPreCommitInfo
}

func (evt SectorPreCommitted) apply(state *SectorInfo) {
	state.PreCommitMessage = &evt.Message
	state.PreCommitDeposit = evt.PreCommitDeposit
}

type SectorSeedReady struct {
	SeedValue abi.InteractiveSealRandomness
	SeedEpoch abi.ChainEpoch
}

func (evt SectorSeedReady) apply(state *SectorInfo) {
	state.SeedEpoch = evt.SeedEpoch
	state.SeedValue = evt.SeedValue
}

type SectorRemoteCommit1Failed struct{ error }

func (evt SectorRemoteCommit1Failed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorRemoteCommit1Failed) apply(*SectorInfo)                        {}

type SectorRemoteCommit2Failed struct{ error }

func (evt SectorRemoteCommit2Failed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorRemoteCommit2Failed) apply(*SectorInfo)                        {}

type SectorComputeProofFailed struct{ error }

func (evt SectorComputeProofFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorComputeProofFailed) apply(*SectorInfo)                        {}

type SectorCommitFailed struct{ error }

func (evt SectorCommitFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorCommitFailed) apply(*SectorInfo)                        {}

type SectorRetrySubmitCommit struct{}

func (evt SectorRetrySubmitCommit) apply(*SectorInfo) {}

type SectorDealsExpired struct{ error }

func (evt SectorDealsExpired) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorDealsExpired) apply(*SectorInfo)                        {}

type SectorTicketExpired struct{ error }

func (evt SectorTicketExpired) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorTicketExpired) apply(*SectorInfo)                        {}

type SectorCommitted struct {
	Proof []byte
}

func (evt SectorCommitted) apply(state *SectorInfo) {
	state.Proof = evt.Proof
}

// SectorProofReady is like SectorCommitted, but finalizes before sending the proof to the chain
type SectorProofReady struct {
	Proof []byte
}

func (evt SectorProofReady) apply(state *SectorInfo) {
	state.Proof = evt.Proof
}

type SectorSubmitCommitAggregate struct{}

func (evt SectorSubmitCommitAggregate) apply(*SectorInfo) {}

type SectorCommitSubmitted struct {
	Message cid.Cid
}

func (evt SectorCommitSubmitted) apply(state *SectorInfo) {
	state.CommitMessage = &evt.Message
}

type SectorCommitAggregateSent struct {
	Message cid.Cid
}

func (evt SectorCommitAggregateSent) apply(state *SectorInfo) {
	state.CommitMessage = &evt.Message
}

type SectorProving struct{}

func (evt SectorProving) apply(*SectorInfo) {}

type SectorFinalized struct{}

func (evt SectorFinalized) apply(*SectorInfo) {}

type SectorFinalizedAvailable struct{}

func (evt SectorFinalizedAvailable) apply(*SectorInfo) {}

type SectorRetryFinalize struct{}

func (evt SectorRetryFinalize) apply(*SectorInfo) {}

type SectorFinalizeFailed struct{ error }

func (evt SectorFinalizeFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorFinalizeFailed) apply(*SectorInfo)                        {}

// Snap deals // CC update path

type SectorMarkForUpdate struct{}

func (evt SectorMarkForUpdate) apply(state *SectorInfo) {}

type SectorStartCCUpdate struct{}

func (evt SectorStartCCUpdate) apply(state *SectorInfo) {
	state.CCUpdate = true
	// Clear filler piece but remember in case of abort
	state.CCPieces = state.Pieces
	state.Pieces = nil

	// Clear CreationTime in case this sector was accepting piece data previously
	state.CreationTime = 0
}

type SectorReplicaUpdate struct {
	Out storiface.ReplicaUpdateOut
}

func (evt SectorReplicaUpdate) apply(state *SectorInfo) {
	state.UpdateSealed = &evt.Out.NewSealed
	state.UpdateUnsealed = &evt.Out.NewUnsealed
}

type SectorProveReplicaUpdate struct {
	Proof storiface.ReplicaUpdateProof
}

func (evt SectorProveReplicaUpdate) apply(state *SectorInfo) {
	state.ReplicaUpdateProof = evt.Proof
}

type SectorReplicaUpdateSubmitted struct {
	Message cid.Cid
}

func (evt SectorReplicaUpdateSubmitted) apply(state *SectorInfo) {
	state.ReplicaUpdateMessage = &evt.Message
}

type SectorReplicaUpdateLanded struct{}

func (evt SectorReplicaUpdateLanded) apply(state *SectorInfo) {}

type SectorUpdateActive struct{}

func (evt SectorUpdateActive) apply(state *SectorInfo) {}

type SectorKeyReleased struct{}

func (evt SectorKeyReleased) apply(state *SectorInfo) {}

// Failed state recovery

type SectorRetrySealPreCommit1 struct{}

func (evt SectorRetrySealPreCommit1) apply(state *SectorInfo) {}

type SectorRetrySealPreCommit2 struct{}

func (evt SectorRetrySealPreCommit2) apply(state *SectorInfo) {}

type SectorRetryPreCommit struct{}

func (evt SectorRetryPreCommit) apply(state *SectorInfo) {}

type SectorRetryWaitSeed struct{}

func (evt SectorRetryWaitSeed) apply(state *SectorInfo) {}

type SectorRetryPreCommitWait struct{}

func (evt SectorRetryPreCommitWait) apply(state *SectorInfo) {}

type SectorRetryComputeProof struct{}

func (evt SectorRetryComputeProof) apply(state *SectorInfo) {
	state.InvalidProofs++
}

type SectorRetryInvalidProof struct{}

func (evt SectorRetryInvalidProof) apply(state *SectorInfo) {
	state.InvalidProofs++
}

type SectorRetryCommitWait struct{}

func (evt SectorRetryCommitWait) apply(state *SectorInfo) {}

type SectorInvalidDealIDs struct {
	Return ReturnState
}

func (evt SectorInvalidDealIDs) apply(state *SectorInfo) {
	state.Return = evt.Return
}

type SectorUpdateDealIDs struct {
	Updates map[int]abi.DealID
}

func (evt SectorUpdateDealIDs) apply(state *SectorInfo) {
	for i, id := range evt.Updates {
		// NOTE: all update deals are builtin-market deals
		state.Pieces[i].real.DealInfo.DealID = id
	}
}

// Snap Deals failure and recovery

type SectorRetryReplicaUpdate struct{}

func (evt SectorRetryReplicaUpdate) apply(state *SectorInfo) {}

type SectorRetryProveReplicaUpdate struct{}

func (evt SectorRetryProveReplicaUpdate) apply(state *SectorInfo) {}

type SectorUpdateReplicaFailed struct{ error }

func (evt SectorUpdateReplicaFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorUpdateReplicaFailed) apply(state *SectorInfo)                  {}

type SectorProveReplicaUpdateFailed struct{ error }

func (evt SectorProveReplicaUpdateFailed) FormatError(xerrors.Printer) (next error) {
	return evt.error
}
func (evt SectorProveReplicaUpdateFailed) apply(state *SectorInfo) {}

type SectorAbortUpgrade struct{ error }

func (evt SectorAbortUpgrade) apply(state *SectorInfo) {}
func (evt SectorAbortUpgrade) FormatError(xerrors.Printer) (next error) {
	return evt.error
}

type SectorRevertUpgradeToProving struct{}

func (evt SectorRevertUpgradeToProving) apply(state *SectorInfo) {
	// cleanup sector state so that it is back in proving
	state.CCUpdate = false
	state.UpdateSealed = nil
	state.UpdateUnsealed = nil
	state.ReplicaUpdateProof = nil
	state.ReplicaUpdateMessage = nil
	state.Pieces = state.CCPieces
	state.CCPieces = nil
	state.CreationTime = 0
}

type SectorRetrySubmitReplicaUpdateWait struct{}

func (evt SectorRetrySubmitReplicaUpdateWait) apply(state *SectorInfo) {}

type SectorRetrySubmitReplicaUpdate struct{}

func (evt SectorRetrySubmitReplicaUpdate) apply(state *SectorInfo) {}

type SectorSubmitReplicaUpdateFailed struct{}

func (evt SectorSubmitReplicaUpdateFailed) apply(state *SectorInfo) {}

type SectorDeadlineImmutable struct{}

func (evt SectorDeadlineImmutable) apply(state *SectorInfo) {}

type SectorDeadlineMutable struct{}

func (evt SectorDeadlineMutable) apply(state *SectorInfo) {}

type SectorReleaseKeyFailed struct{ error }

func (evt SectorReleaseKeyFailed) FormatError(xerrors.Printer) (next error) {
	return evt.error
}
func (evt SectorReleaseKeyFailed) apply(state *SectorInfo) {}

// Faults

type SectorFaulty struct{}

func (evt SectorFaulty) apply(state *SectorInfo) {}

type SectorFaultReported struct{ reportMsg cid.Cid }

func (evt SectorFaultReported) apply(state *SectorInfo) {
	state.FaultReportMsg = &evt.reportMsg
}

type SectorFaultedFinal struct{}

// Terminating

type SectorTerminate struct{}

func (evt SectorTerminate) applyGlobal(state *SectorInfo) bool {
	state.State = Terminating
	return true
}

type SectorTerminating struct{ Message *cid.Cid }

func (evt SectorTerminating) apply(state *SectorInfo) {
	state.TerminateMessage = evt.Message
}

type SectorTerminated struct{ TerminatedAt abi.ChainEpoch }

func (evt SectorTerminated) apply(state *SectorInfo) {
	state.TerminatedAt = evt.TerminatedAt
}

type SectorTerminateFailed struct{ error }

func (evt SectorTerminateFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorTerminateFailed) apply(*SectorInfo)                        {}

// External events

type SectorRemove struct{}

func (evt SectorRemove) applyGlobal(state *SectorInfo) bool {
	// because this event is global we need to send the notification here instead through an fsm callback
	maybeNotifyRemoteDone(false, "Removing")(state)

	state.State = Removing
	return true
}

type SectorRemoved struct{}

func (evt SectorRemoved) apply(state *SectorInfo) {}

type SectorRemoveFailed struct{ error }

func (evt SectorRemoveFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorRemoveFailed) apply(*SectorInfo)                        {}

type SectorReceive struct {
	State SectorInfo
}

func (evt SectorReceive) apply(state *SectorInfo) {
	*state = evt.State
}

type SectorReceived struct{}

func (evt SectorReceived) apply(state *SectorInfo) {}
