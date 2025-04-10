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
	//
	// See TipSetTag.
	TipSetTags = struct {
		Latest    TipSetTag
		Finalized TipSetTag
	}{
		Latest:    TipSetTag("latest"),
		Finalized: TipSetTag("finalized"),
	}

	// TipSetSelectors represents the predefined set of selectors for tipsets.
	//
	// See TipSetSelector.
	TipSetSelectors = struct {
		Latest    TipSetSelector
		Finalized TipSetSelector
		Height    func(abi.ChainEpoch, bool, *TipSetAnchor) TipSetSelector
		Key       func(TipSetKey) TipSetSelector
	}{
		Latest:    TipSetSelector{Tag: &TipSetTags.Latest},
		Finalized: TipSetSelector{Tag: &TipSetTags.Finalized},
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
		Key       func(TipSetKey) *TipSetAnchor
	}{
		Latest:    &TipSetAnchor{Tag: &TipSetTags.Latest},
		Finalized: &TipSetAnchor{Tag: &TipSetTags.Finalized},
		Key:       func(key TipSetKey) *TipSetAnchor { return &TipSetAnchor{Key: &key} },
	}

	// TipSetLimits provides convenient constructor functions for creating different types
	// of TipSetLimit instances.
	//
	// It contains:
	//   - Unlimited: A pre-initialized TipSetLimit with no effective height limit
	//   - Height: A function to create a TipSetLimit with an absolute height
	//   - Distance: A function to create a TipSetLimit with a relative distance
	TipSetLimits = struct {
		// Unlimited is a pre-initialized TipSetLimit representing an unbounded height limit.
		// This is represented internally using a negative value that would otherwise be invalid.
		Unlimited TipSetLimit

		// Height returns a TipSetLimit constrained to a specific absolute chain epoch.
		//
		// Parameters:
		//   - at: The absolute chain epoch to limit to
		Height func(abi.ChainEpoch) TipSetLimit

		// Distance returns a TipSetLimit constrained to a relative distance from a reference point.
		// The actual height will be calculated when HeightRelativeTo is called.
		//
		// Parameters:
		//   - distance: The number of epochs relative to some reference point
		Distance func(uint64) TipSetLimit
	}{
		Height:   func(at abi.ChainEpoch) TipSetLimit { return TipSetLimit{Height: &at} },
		Distance: func(distance uint64) TipSetLimit { return TipSetLimit{Distance: &distance} },
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

// TipSetLimit represents a limit on tipsets, either by absolute height or
// by relative distance from a reference point.
//
// Either Height or Distance must be set, but not both. If Height is set,
// it specifies an absolute chain epoch. If Distance is set, it specifies
// a relative distance from a reference point. A special unlimited value
// is available through TipSetLimits.Unlimited.
type TipSetLimit struct {
	// Height represents an absolute chain epoch height.
	// When set, Distance must be nil.
	Height *abi.ChainEpoch `json:"height,omitempty"`

	// Distance represents a relative distance from a reference point in epochs.
	// When set, Height must be nil.
	Distance *uint64 `json:"distance,omitempty"`
}

// Validate checks if the TipSetLimit is properly configured.
//
// Returns an error if both Height and Distance are set,
// or if Height is set to a negative value. Note that the special unlimited
// value TipSetLimits.Unlimited bypasses the negative value check as it uses
// a negative height internally.
func (tsl TipSetLimit) Validate() error {
	heightSet := tsl.Height != nil
	distanceSet := tsl.Distance != nil
	if distanceSet && heightSet {
		return xerrors.New("either distance or height must be set, but not both")
	}
	if heightSet && *tsl.Height < 0 {
		return xerrors.New("height must be greater than or equal to zero")
	}
	return nil
}

// HeightRelativeTo calculates the absolute chain epoch based on the TipSetLimit
// configuration and a reference epoch.
//
// If Height is set, it returns the absolute height regardless of the reference
// epoch. If Distance is set, it returns the reference epoch plus the distance.
// If neither is set, it returns -1.
//
// Note that this function does not validate the TipSetLimit configuration.
func (tsl TipSetLimit) HeightRelativeTo(relative abi.ChainEpoch) abi.ChainEpoch {
	if tsl.Height != nil {
		// Ignore relative height, and return the absolute height specified in limit.
		return *tsl.Height
	}
	if tsl.Distance != nil {
		return relative + abi.ChainEpoch(*tsl.Distance)
	}
	return -1
}
