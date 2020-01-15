package sealing

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
)

type SectorStart struct {
	id     uint64
	pieces []Piece
}
type SectorRestart struct{}

type SectorFatalError struct{ error }

type SectorPacked struct{ pieces []Piece }

type SectorSealed struct {
	commR  []byte
	commD  []byte
	ticket SealTicket
}
type SectorSealFailed struct{ error }

type SectorPreCommitFailed struct{ error }
type SectorPreCommitted struct {
	message cid.Cid
}

type SectorSeedReady struct {
	seed SealSeed
}

type SectorSealCommitFailed struct{ error }
type SectorCommitFailed struct{ error }
type SectorCommitted struct {
	message cid.Cid
	proof   []byte
}

type SectorProving struct{}

type SectorFaultReported struct{ reportMsg cid.Cid }
type SectorFaultedFinal struct{}

type SectorForceState struct {
	state api.SectorState
}
