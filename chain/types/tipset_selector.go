package types

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
)

var (
	// TipSetTags represents the predefined set of tags for tipsets. The supported
	// tags are:
	//   - Latest: the most recent tipset in the chain with the heaviest weight.
	//   - Finalized: the most recent tipset considered final by the node.
	//   - Safe: the most recent tipset between Finalized and Latest - build.SafeHeightDistance.
	//          If the tipset at the safe height is null, the first non-nil parent tipset is returned.
	//
	// See TipSetTag.
	TipSetTags = struct {
		Latest    TipSetTag
		Finalized TipSetTag
		Safe      TipSetTag
	}{
		Latest:    TipSetTag("latest"),
		Finalized: TipSetTag("finalized"),
		Safe:      TipSetTag("safe"),
	}

	// TipSetSelectors represents the predefined set of selectors for tipsets.
	//
	// See TipSetSelector.
	TipSetSelectors = struct {
		Latest    TipSetSelector
		Finalized TipSetSelector
		Safe      TipSetSelector
		Height    func(abi.ChainEpoch, bool, *TipSetAnchor) TipSetSelector
		Key       func(TipSetKey) TipSetSelector
	}{
		Latest:    TipSetSelector{Tag: &TipSetTags.Latest},
		Finalized: TipSetSelector{Tag: &TipSetTags.Finalized},
		Safe:      TipSetSelector{Tag: &TipSetTags.Safe},
		Height: func(height abi.ChainEpoch, previous bool, anchor *TipSetAnchor) TipSetSelector {
			return TipSetSelector{Height: &TipSetHeight{At: &height, Previous: previous, Anchor: anchor}}
		},
		Key: func(key TipSetKey) TipSetSelector { return TipSetSelector{Key: &key} },
	}

	// TipSetAnchors represents the predefined set of anchors for tipsets.
	//
	// See TipSetAnchor.
	TipSetAnchors = struct {
		Latest    *TipSetAnchor
		Finalized *TipSetAnchor
		Safe      *TipSetAnchor
		Key       func(TipSetKey) *TipSetAnchor
	}{
		Latest:    &TipSetAnchor{Tag: &TipSetTags.Latest},
		Finalized: &TipSetAnchor{Tag: &TipSetTags.Finalized},
		Safe:      &TipSetAnchor{Tag: &TipSetTags.Safe},
		Key:       func(key TipSetKey) *TipSetAnchor { return &TipSetAnchor{Key: &key} },
	}
)

// TipSetTag is a string that represents a pointer to a tipset.
// See TipSetSelector.
type TipSetTag string

// TipSetSelector captures the selection criteria for a tipset.
//
// The supported criterion for selection is one of the following:
//   - Key: the tipset key, see TipSetKey.
//   - Height: the tipset height with an optional fallback to non-null parent, see TipSetHeight.
//   - Tag: the tipset tag, either "latest" or "finalized", see TipSetTags.
//
// At most, one such criterion can be specified at a time. Otherwise, the
// criterion is considered to be invalid. See Validate.
//
// Experimental: This API is experimental and may change without notice.
type TipSetSelector struct {
	Key    *TipSetKey    `json:"key,omitempty"`
	Height *TipSetHeight `json:"height,omitempty"`
	Tag    *TipSetTag    `json:"tag,omitempty"`
}

// Validate ensures that the TipSetSelector is valid. It checks that only one of
// the selection criteria is specified. If no criteria are specified, it returns
// nil, indicating that the default selection criteria should be used as defined
// by the Lotus API Specification.
func (tss TipSetSelector) Validate() error {
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
	if criteria != 1 {
		return xerrors.Errorf("exactly one tipset selection criteria must be specified, found: %v", criteria)
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
// specified by the anchor at the given height. Otherwise, the "finalized" TipSetTag
// is used as the Anchor.
//
// Experimental: This API is experimental and may change without notice.
type TipSetHeight struct {
	At       *abi.ChainEpoch `json:"at,omitempty"`
	Previous bool            `json:"previous,omitempty"`
	Anchor   *TipSetAnchor   `json:"anchor,omitempty"`
}

// Validate ensures that the TipSetHeight is valid. It checks that the height is
// not negative and the Anchor is valid.
//
// A nil or a zero-valued height is considered to be valid.
func (tsh TipSetHeight) Validate() error {
	if tsh.At == nil {
		return xerrors.New("invalid tipset height: at epoch must be specified")
	}
	if *tsh.At < 0 {
		return xerrors.New("invalid tipset height: epoch cannot be less than zero")
	}
	return tsh.Anchor.Validate()
}

// TipSetAnchor represents a tipset in the chain that can be used as an anchor
// for selecting a tipset. The anchor may be specified as a TipSetTag or a
// TipSetKey but not both. Defaults to TipSetTag "finalized" if neither are
// specified.
//
// See TipSetHeight.
//
// Experimental: This API is experimental and may change without notice.
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
// equivalent to the default anchor, which is the tipset tagged as "finalized".
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
