package main

import (
	"encoding/hex"
	"encoding/json"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-cid"
)

// Class represents the type of test this instance is.
type Class string

var (
	// ClassMessage tests the VM transition over a single message
	ClassMessage Class = "message"
	// ClassBlock tests the VM transition over a block of messages
	ClassBlock Class = "block"
	// ClassTipset tests the VM transition on a tipset update
	ClassTipset Class = "tipset"
	// ClassChain tests the VM transition across a chain segment
	ClassChain Class = "chain"
)

// Selector provides a filter to indicate what implementations this test is relevant for
type Selector string

// Metadata provides information on the generation of this test case
type Metadata struct {
	ID      string         `json:"id"`
	Version string         `json:"version"`
	Gen     GenerationData `json:"gen"`
}

// GenerationData tags the source of this test case
type GenerationData struct {
	Source  string `json:"source"`
	Version string `json:"version"`
}

// StateTree represents a state tree within preconditions and postconditions.
type StateTree struct {
	RootCID cid.Cid `json:"root_cid"`
}

// HexEncodedBytes is a hex-encoded binary value.
//
// TODO may switch to base64 or base85 for efficiency.
type HexEncodedBytes []byte

// Preconditions contain a representation of VM state at the beginning of the test
type Preconditions struct {
	Epoch     abi.ChainEpoch `json:"epoch"`
	StateTree *StateTree     `json:"state_tree"`
}

// Postconditions contain a representation of VM state at th end of the test
type Postconditions struct {
	StateTree *StateTree `json:"state_tree"`
}

// MarshalJSON implements json.Marshal for HexEncodedBytes
func (heb HexEncodedBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(heb))
}

// UnmarshalJSON implements json.Unmarshal for HexEncodedBytes
func (heb *HexEncodedBytes) UnmarshalJSON(v []byte) error {
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return err
	}

	bytes, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	*heb = bytes
	return nil
}

// TestVector is a single test case
type TestVector struct {
	Class    `json:"class"`
	Selector `json:"selector"`
	Meta     *Metadata `json:"_meta"`

	// CAR binary data to be loaded into the test environment, usually a CAR
	// containing multiple state trees, addressed by root CID from the relevant
	// objects.
	CAR HexEncodedBytes `json:"car_hex"`

	Pre           *Preconditions    `json:"preconditions"`
	ApplyMessages []HexEncodedBytes `json:"apply_messages"`
	Post          *Postconditions   `json:"postconditions"`
}
