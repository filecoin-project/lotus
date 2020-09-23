package types

import "github.com/ipfs/go-cid"

type StateRoot struct {
	// State root version. Versioned along with actors (for now).
	Version uint64
	// Actors tree. The structure depends on the state root version.
	Actors cid.Cid
	// Info. The structure depends on the state root version.
	Info cid.Cid
}

// TODO: version this.
type StateInfo struct{}
