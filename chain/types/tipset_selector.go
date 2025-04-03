package types

import (
	"encoding/json"

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
// At most, one such criterion can be specified at a time. Otherwise, the
// criterion is considered to be invalid. See Validate.
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
		return nil, xerrors.Errorf("unknown tipset slection criterion: %T", criterion)
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
		if err := tss.Height.Validate(); err != nil {
			return xerrors.Errorf("validating tipset height: %w", err)
		}
	}
	if criteria > 1 {
		return xerrors.Errorf("only one tipset selection criteria must be specified, found: %v", criteria)
	}
	return nil
}

// TipSetHeight is a criterion that selects a tipset At given height anchored to
// a given parent tipset.
//
// In a case where the tipset at given height is null, and Previous is true,
// it'll select the previous non-null tipset instead. Otherwise, it returns the
// null tipset at the given height.
//
// The Anchor may optionally be specified as TipSetTag, or TipSetKey. If
// specified, the selected tipset is guaranteed to be a child of the tipset
// specified by the anchor at the given height. Otherwise, the "latest" TipSetTag
// is used as the Anchor.
type TipSetHeight struct {
	At       abi.ChainEpoch `json:"at,omitempty"`
	Previous bool           `json:"previous,omitempty"`
	Anchor   *TipSetAnchor  `json:"anchor,omitempty"`
}

// Validate ensures that the TipSetHeight is valid. It checks that the height is
// not negative and the Anchor is valid.
//
// A nil or a zero-valued height is considered to be valid.
func (tsh *TipSetHeight) Validate() error {
	if tsh == nil {
		// An unspecified height is valid, because it's an optional field in TipSetCriterion.
		return nil
	}
	if tsh.At < 0 {
		return xerrors.New("invalid tipset height: epoch cannot be less than zero")
	}
	return tsh.Anchor.Validate()
}

// TipSetAnchor represents a tipset in the chain that can be used as an anchor
// for selecting a tipset. The anchor may be specified as a TipSetTag or a
// TipSetKey but not both. Defaults to TipSetTag "latest" if neither are
// specified.
//
// See TipSetHeight.
type TipSetAnchor struct {

	// TODO: We might want to rename the term "anchor" to "parent" if they're
	//       conceptually interchangeable. Because, it is easier to reuse a term that
	//       already exist compared to teaching people a new one. For now we'll keep it as
	//       "anchor" to keep consistent with the internal API design discussions. We will
	//       revisit the terminology here as the new API groups are added, namely
	//       StateSearchMsg.

	Key *TipSetKey `json:"key,omitempty"`
	Tag *TipSetTag `json:"tag,omitempty"`
}

// Validate ensures that the TipSetAnchor is valid. It checks that at most one
// of TipSetKey or TipSetTag is specified. Otherwise, it returns an error.
//
// Note that a nil or a zero-valued anchor is valid, and is considered to be
// equivalent to the default anchor, which is the tipset tagged as "latest".
func (tsa *TipSetAnchor) Validate() error {
	if tsa == nil {
		// An unspecified Anchor is valid, because it's an optional field, and falls back
		// to whatever the API decides the default to be.
		return nil
	}
	if tsa.Key != nil && tsa.Tag != nil {
		return xerrors.New("invalid tipset anchor: at most one of key or tag must be specified")
	}
	// Zero-valued anchor is valid, and considered to be an equivalent to whatever
	// the API decides the default to be.
	return nil
}
