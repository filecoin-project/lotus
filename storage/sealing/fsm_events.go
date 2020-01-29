package sealing

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
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

// Global events

type SectorRestart struct{}

func (evt SectorRestart) applyGlobal(*SectorInfo) bool { return false }

type SectorFatalError struct{ error }

func (evt SectorFatalError) applyGlobal(state *SectorInfo) bool {
	log.Errorf("Fatal error on sector %d: %+v", state.SectorID, evt.error)
	// TODO: Do we want to mark the state as unrecoverable?
	//  I feel like this should be a softer error, where the user would
	//  be able to send a retry event of some kind
	return true
}

type SectorForceState struct {
	state api.SectorState
}

func (evt SectorForceState) applyGlobal(state *SectorInfo) bool {
	state.State = evt.state
	return true
}

// Normal path

type SectorStart struct {
	id     uint64
	pieces []Piece
}

func (evt SectorStart) apply(state *SectorInfo) {
	state.SectorID = evt.id
	state.Pieces = evt.pieces
}

type SectorPacked struct{ pieces []Piece }

func (evt SectorPacked) apply(state *SectorInfo) {
	state.Pieces = append(state.Pieces, evt.pieces...)
}

type SectorPackingFailed struct{ error }

func (evt SectorPackingFailed) apply(*SectorInfo) {}

type SectorSealed struct {
	commR  []byte
	commD  []byte
	ticket SealTicket
}

func (evt SectorSealed) apply(state *SectorInfo) {
	state.CommD = evt.commD
	state.CommR = evt.commR
	state.Ticket = evt.ticket
}

type SectorSealFailed struct{ error }

func (evt SectorSealFailed) apply(*SectorInfo) {}

type SectorPreCommitFailed struct{ error }

func (evt SectorPreCommitFailed) apply(*SectorInfo) {}

type SectorPreCommitted struct {
	message cid.Cid
}

func (evt SectorPreCommitted) apply(state *SectorInfo) {
	state.PreCommitMessage = &evt.message
}

type SectorSeedReady struct {
	seed SealSeed
}

func (evt SectorSeedReady) apply(state *SectorInfo) {
	state.Seed = evt.seed
}

type SectorComputeProofFailed struct{ error }

type SectorCommitFailed struct{ error }

func (evt SectorCommitFailed) apply(*SectorInfo) {}

type SectorCommitted struct {
	message cid.Cid
	proof   []byte
}

func (evt SectorCommitted) apply(state *SectorInfo) {
	state.Proof = evt.proof
	state.CommitMessage = &evt.message
}

type SectorProving struct{}

func (evt SectorProving) apply(*SectorInfo) {}

type SectorFinalized struct{}

func (evt SectorFinalized) apply(*SectorInfo) {}

type SectorFinalizeFailed struct{ error }

func (evt SectorFinalizeFailed) apply(*SectorInfo) {}

// Failed state recovery

type SectorRetrySeal struct{}

func (evt SectorRetrySeal) apply(state *SectorInfo) {}

type SectorRetryPreCommit struct{}

func (evt SectorRetryPreCommit) apply(state *SectorInfo) {}

type SectorRetryWaitSeed struct{}

func (evt SectorRetryWaitSeed) apply(state *SectorInfo) {}

// Faults

type SectorFaulty struct{}

func (evt SectorFaulty) apply(state *SectorInfo) {}

type SectorFaultReported struct{ reportMsg cid.Cid }

func (evt SectorFaultReported) apply(state *SectorInfo) {
	state.FaultReportMsg = &evt.reportMsg
}

type SectorFaultedFinal struct{}
