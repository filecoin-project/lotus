package types

import "github.com/ipfs/go-cid"

// StateTreeVersion is the version of the state tree itself, independent of the
// network version or the actors version.
type StateTreeVersion uint64

const (
	// StateTreeVersion0 corresponds to actors < v2.
	StateTreeVersion0 StateTreeVersion = iota
	// StateTreeVersion1 corresponds to actors v2
	StateTreeVersion1
	// StateTreeVersion2 corresponds to actors v3.
	StateTreeVersion2
	// StateTreeVersion3 corresponds to actors v4.
	StateTreeVersion3
	// StateTreeVersion4 corresponds to actors v5, v6.
	StateTreeVersion4
)

type StateRoot struct {
	// State tree version.
	Version StateTreeVersion
	// Actors tree. The structure depends on the state root version.
	Actors cid.Cid
	// Info. The structure depends on the state root version.
	Info cid.Cid
}

// TODO: version this.
type StateInfo0 struct{}
