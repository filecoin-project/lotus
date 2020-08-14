package schema

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
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
	Desc    string         `json:"description"`
	Comment string         `json:"comment"`
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

// Base64EncodedBytes is a base64-encoded binary value.
type Base64EncodedBytes []byte

// Preconditions contain a representation of VM state at the beginning of the test
type Preconditions struct {
	Epoch     abi.ChainEpoch `json:"epoch"`
	StateTree *StateTree     `json:"state_tree"`
}

// Receipt represents a receipt to match against.
type Receipt struct {
	ExitCode    exitcode.ExitCode  `json:"exit_code"`
	ReturnValue Base64EncodedBytes `json:"return"`
	GasUsed     int64              `json:"gas_used"`
}

// Postconditions contain a representation of VM state at th end of the test
type Postconditions struct {
	StateTree *StateTree `json:"state_tree"`
	Receipts  []*Receipt `json:"receipts"`
}

// MarshalJSON implements json.Marshal for Base64EncodedBytes
func (beb Base64EncodedBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.StdEncoding.EncodeToString(beb))
}

// UnmarshalJSON implements json.Unmarshal for Base64EncodedBytes
func (beb *Base64EncodedBytes) UnmarshalJSON(v []byte) error {
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return err
	}

	bytes, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	*beb = bytes
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
	CAR Base64EncodedBytes `json:"car"`

	Pre           *Preconditions  `json:"preconditions"`
	ApplyMessages []Message       `json:"apply_messages"`
	Post          *Postconditions `json:"postconditions"`
}

type Message struct {
	Bytes Base64EncodedBytes `json:"bytes"`
	Epoch *abi.ChainEpoch    `json:"epoch,omitempty"`
}

// Validate validates this test vector against the JSON schema, and applies
// further validation rules that cannot be enforced through JSON Schema.
func (tv TestVector) Validate() error {
	// TODO validate against JSON Schema.
	if tv.Class == ClassMessage {
		if len(tv.Post.Receipts) != len(tv.ApplyMessages) {
			return fmt.Errorf("length of postcondition receipts must match length of messages to apply")
		}
	}
	return nil
}

// MustMarshalJSON encodes the test vector to JSON and panics if it errors.
func (tv TestVector) MustMarshalJSON() []byte {
	b, err := json.Marshal(&tv)
	if err != nil {
		panic(err)
	}
	return b
}
