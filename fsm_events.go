package sealing

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
)

type mutator interface {
	apply(state *SectorInfo)
}

type SectorStart struct {
	id     uint64
	pieces []Piece
}
func (evt SectorStart) apply(state *SectorInfo) {
	state.SectorID = evt.id
	state.Pieces = evt.pieces
}

type SectorRestart struct{}

type SectorFatalError struct{ error }

type SectorPacked struct{ pieces []Piece }
func (evt SectorPacked) apply(state *SectorInfo) {
	state.Pieces = append(state.Pieces, evt.pieces...)
}

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

type SectorPreCommitFailed struct{ error }

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

type SectorSealCommitFailed struct{ error }
type SectorCommitFailed struct{ error }
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

type SectorFaultReported struct{ reportMsg cid.Cid }
type SectorFaultedFinal struct{}

type SectorForceState struct {
	state api.SectorState
}
