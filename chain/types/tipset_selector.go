package types

import (
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
)

// TipSetTag is a string that represents a pointer to a tipset.
// See TipSetCriterion.
type TipSetTag string

// TipSetTags represents the predefined set of tags for tipsets. The supported
// tags are:
//   - Latest: the most recent tipset in the chain with the heaviest weight.
//   - Finalized: the most recent tipset considered final by the node.
//
// See TipSetTag.
var TipSetTags = struct {
	Latest    TipSetTag
	Finalized TipSetTag
}{
	Latest:    TipSetTag("latest"),
	Finalized: TipSetTag("finalized"),
}

// TipSetSelector is a JSON-RPC parameter type that can be used to select a tipset.
// See NewTipSetSelector, TipSetCriterion.
type TipSetSelector = jsonrpc.RawParams

// TipSetCriterion captures the selection criteria for a tipset.
//
// The supported criterion for selection is one of the following:
//   - Key: the tipset key, see TipSetKey.
//   - Height: the tipset height with an optional fallback to non-null parent, see TipSetHeight.
//   - Tag: the tipset tag, either "latest" or "finalized", see TipSetTags.
//
// At most, only one such criterion can be specified at a time. Otherwise, the
// criterion is considered. See Validate.
type TipSetCriterion struct {
	Key    *TipSetKey    `json:"key,omitempty"`
	Height *TipSetHeight `json:"height,omitempty"`
	Tag    *TipSetTag    `json:"tag,omitempty"`
}

// TipSetCriteria is a union of all possible criteria for selecting a tipset.
type TipSetCriteria interface {
	TipSetTag | TipSetHeight | TipSetKey
}

// NewTipSetSelector creates a new TipSetSelector from the given criterion. The
// criterion must conform to one of the supported types: TipSetTag, TipSetHeight,
// or TipSetKey.
//
// See TipSetCriteria type constraint.
func NewTipSetSelector[T TipSetCriteria](t T) (TipSetSelector, error) {
	switch criterion := any(t).(type) {
	case TipSetTag:
		return json.Marshal(TipSetCriterion{Tag: &criterion})
	case TipSetHeight:
		return json.Marshal(TipSetCriterion{Height: &criterion})
	case TipSetKey:
		return json.Marshal(TipSetCriterion{Key: &criterion})
	default:
		// Dear Golang, this should be impossible because of the type constraint being
		// evaluated at compile time; yet, here we are. I would panic, but then we are
		// friends and this function returns errors anyhow.
		return nil, fmt.Errorf("unknown tipset slection criterion: %T", criterion)
	}
}

// DecodeTipSetCriterion extracts a TipSetCriterion from the given
// TipSetSelector. The returned criterion is validated before returning. If the
// selector is empty, a nil criterion is returned.
func DecodeTipSetCriterion(tss TipSetSelector) (*TipSetCriterion, error) {
	if len(tss) == 0 || string(tss) == "null" {
		return nil, nil
	}

	var criterion TipSetCriterion
	if err := json.Unmarshal(tss, &criterion); err != nil {
		return nil, xerrors.Errorf("decoding tipset selector: %w", err)
	}
	if err := criterion.Validate(); err != nil {
		return nil, xerrors.Errorf("validating tipset criterion: %w", err)
	}
	return &criterion, nil
}

// Validate ensures that the TipSetCriterion is valid. It checks that only one of
// the selection criteria is specified. If no criteria are specified, it returns
// nil, indicating that the default selection criteria should be used as defined
// by the Lotus API Specification.
func (tss *TipSetCriterion) Validate() error {
	if tss == nil {
		// Empty TipSetSelector is valid and implies whatever the default is dictated to
		// be by the API, which happens to be the tipset tagged as "latest".
		return nil
	}
	var criteria int
	if tss.Key != nil {
		criteria++
	}
	if tss.Tag != nil {
		criteria++
	}
	if tss.Height != nil {
		criteria++
	}
	if criteria > 1 {
		return xerrors.Errorf("only one tipset selection criteria must be specified, found: %v", criteria)
	}
	return nil
}

// TipSetHeight is a criterion that selects a tipset At given height.
//
// In a case where the tipset at given height is null, and Previous is true,
// it'll select the previous non-null tipset instead. Otherwise, it returns the
// null tipset at the given height.
type TipSetHeight struct {
	At       abi.ChainEpoch `json:"at,omitempty"`
	Previous bool           `json:"previous,omitempty"`
}
