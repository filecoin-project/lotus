package sealing

import (
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-storage/storage"
)

type mutator interface {
	apply(state *SectorInfo)
}

// globalMutator is an event which can apply in every state
type globalMutator interface {
	// applyGlobal applies the event to the state. If if returns true,
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
	Pieces     []Piece
}

func (evt SectorStartCC) apply(state *SectorInfo) {
	state.SectorNumber = evt.ID
	state.Pieces = evt.Pieces
	state.SectorType = evt.SectorType
}

type SectorAddPiece struct {
	NewPiece Piece
}

func (evt SectorAddPiece) apply(state *SectorInfo) {
	state.Pieces = append(state.Pieces, evt.NewPiece)
}

type SectorStartPacking struct{}

func (evt SectorStartPacking) apply(*SectorInfo) {}

func (evt SectorStartPacking) Ignore() {}

type SectorPacked struct{ FillerPieces []abi.PieceInfo }

func (evt SectorPacked) apply(state *SectorInfo) {
	for idx := range evt.FillerPieces {
		state.Pieces = append(state.Pieces, Piece{
			Piece:    evt.FillerPieces[idx],
			DealInfo: nil, // filler pieces don't have deals associated with them
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
	PreCommit1Out storage.PreCommit1Out
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

type SectorPreCommitLanded struct {
	TipSet TipSetToken
}

func (evt SectorPreCommitLanded) apply(si *SectorInfo) {
	si.PreCommitTipSet = evt.TipSet
}

type SectorSealPreCommit1Failed struct{ error }

func (evt SectorSealPreCommit1Failed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorSealPreCommit1Failed) apply(si *SectorInfo) {
	si.InvalidProofs = 0 // reset counter
	si.PreCommit2Fails = 0
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
	state.PreCommitInfo = &evt.PreCommitInfo
}

type SectorSeedReady struct {
	SeedValue abi.InteractiveSealRandomness
	SeedEpoch abi.ChainEpoch
}

func (evt SectorSeedReady) apply(state *SectorInfo) {
	state.SeedEpoch = evt.SeedEpoch
	state.SeedValue = evt.SeedValue
}

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

type SectorCommitSubmitted struct {
	Message cid.Cid
}

func (evt SectorCommitSubmitted) apply(state *SectorInfo) {
	state.CommitMessage = &evt.Message
}

type SectorProving struct{}

func (evt SectorProving) apply(*SectorInfo) {}

type SectorFinalized struct{}

func (evt SectorFinalized) apply(*SectorInfo) {}

type SectorRetryFinalize struct{}

func (evt SectorRetryFinalize) apply(*SectorInfo) {}

type SectorFinalizeFailed struct{ error }

func (evt SectorFinalizeFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorFinalizeFailed) apply(*SectorInfo)                        {}

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
		state.Pieces[i].DealInfo.DealID = id
	}
}

// Faults

type SectorFaulty struct{}

func (evt SectorFaulty) apply(state *SectorInfo) {}

type SectorFaultReported struct{ reportMsg cid.Cid }

func (evt SectorFaultReported) apply(state *SectorInfo) {
	state.FaultReportMsg = &evt.reportMsg
}

type SectorFaultedFinal struct{}

// External events

type SectorRemove struct{}

func (evt SectorRemove) applyGlobal(state *SectorInfo) bool {
	state.State = Removing
	return true
}

type SectorRemoved struct{}

func (evt SectorRemoved) apply(state *SectorInfo) {}

type SectorRemoveFailed struct{ error }

func (evt SectorRemoveFailed) FormatError(xerrors.Printer) (next error) { return evt.error }
func (evt SectorRemoveFailed) apply(*SectorInfo)                        {}
