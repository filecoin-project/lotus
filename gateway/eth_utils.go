package gateway

// Block tag resolution utilities for gateway-level range checking.
//
// The "finalized" tag is not resolved here because its true height depends on
// F3 consensus and/or the EC probability calculator, which cannot be determined
// from head height alone. A static estimate like head-ChainFinality would
// undercount the range when "finalized" is a toBlock (actual finalized height
// is typically much closer to head). If trace_filter gains "finalized" support,
// the gateway will need to query the node for the real finalized height to
// perform accurate range checks.

import (
	"context"
	"math"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// checkEthTraceFilterBlockRange checks whether the block range in the given
// trace filter criteria exceeds the gateway's configured maximum.
func (gw *Node) checkEthTraceFilterBlockRange(headHeight abi.ChainEpoch, filter ethtypes.EthTraceFilterCriteria) error {
	if gw.ethTraceFilterMaxBlockRange <= 0 {
		return nil
	}
	fromBlk := ethtypes.BlockTagLatest
	if filter.FromBlock != nil {
		fromBlk = *filter.FromBlock
	}
	toBlk := ethtypes.BlockTagLatest
	if filter.ToBlock != nil {
		toBlk = *filter.ToBlock
	}
	// Default for omitted fromBlock/toBlock is "latest", matching trace_filter
	// and eth_getLogs semantics (OpenEthereum/Erigon trace_filter spec).
	from, fromOk := resolveTraceFilterBlockTag(fromBlk, headHeight)
	to, toOk := resolveTraceFilterBlockTag(toBlk, headHeight)
	// If either tag couldn't be resolved (e.g. "finalized"), skip the range
	// check and let the node handle validation. If from >= to the node's
	// iteration is a no-op.
	maxRange := uint64(gw.ethTraceFilterMaxBlockRange)
	if fromOk && toOk && to > from && uint64(to-from) > maxRange {
		return api.NewErrBlockRangeExceeded(maxRange, uint64(to-from))
	}
	return nil
}

type chainHeadHeightFunc func(context.Context) (abi.ChainEpoch, error)

// checkEthEventFilterBlockRange checks the height range of an Ethereum event
// filter.
func (gw *Node) checkEthEventFilterBlockRange(ctx context.Context, filter *ethtypes.EthFilterSpec, getHeadHeight chainHeadHeightFunc) error {
	if gw.eventFilterMaxHeightRange <= 0 || filter == nil || filter.BlockHash != nil {
		return nil
	}

	fromTag := ethtypes.BlockTagLatest
	if filter.FromBlock != nil {
		fromTag = *filter.FromBlock
	}
	toTag := ethtypes.BlockTagLatest
	if filter.ToBlock != nil {
		toTag = *filter.ToBlock
	}
	if isEthEventHeadTag(fromTag) && isEthEventHeadTag(toTag) {
		return nil
	}

	var headHeight abi.ChainEpoch
	if isEthEventHeadTag(fromTag) || isEthEventHeadTag(toTag) {
		var err error
		headHeight, err = getHeadHeight(ctx)
		if err != nil {
			return err
		}
	}

	from, fromOK := resolveEthEventFilterBlockTag(fromTag, headHeight)
	to, toOK := resolveEthEventFilterBlockTag(toTag, headHeight)
	if !fromOK || !toOK {
		// Let the full node return the authoritative error for unsupported or
		// malformed block tags.
		return nil
	}
	return gw.checkEventFilterHeightRange(from, to)
}

func (gw *Node) checkActorEventFilterHeightRange(ctx context.Context, filter *types.ActorEventFilter, getHeadHeight chainHeadHeightFunc) error {
	if gw.eventFilterMaxHeightRange <= 0 || filter == nil {
		return nil
	}
	if filter.TipSetKey != nil && !filter.TipSetKey.IsEmpty() {
		return nil
	}
	if filter.FromHeight == nil && filter.ToHeight == nil {
		return nil
	}
	if filter.FromHeight != nil && *filter.FromHeight < 0 {
		return nil
	}
	if filter.ToHeight != nil && *filter.ToHeight < 0 {
		return nil
	}

	var (
		from abi.ChainEpoch
		to   abi.ChainEpoch
	)
	if filter.FromHeight != nil && filter.ToHeight != nil {
		from = *filter.FromHeight
		to = *filter.ToHeight
	} else {
		headHeight, err := getHeadHeight(ctx)
		if err != nil {
			return err
		}
		if filter.FromHeight != nil {
			from = *filter.FromHeight
			if headHeight > 0 {
				to = headHeight - 1
			}
		} else {
			from = headHeight
			to = *filter.ToHeight
		}
	}
	return gw.checkEventFilterHeightRange(from, to)
}

func (gw *Node) checkEventFilterHeightRange(from, to abi.ChainEpoch) error {
	if gw.eventFilterMaxHeightRange <= 0 || to <= from {
		return nil
	}
	heightRange := to - from
	if heightRange > gw.eventFilterMaxHeightRange {
		return api.NewErrBlockRangeExceeded(uint64(gw.eventFilterMaxHeightRange), uint64(heightRange))
	}
	return nil
}

func isEthEventHeadTag(tag string) bool {
	return tag == "" || tag == ethtypes.BlockTagLatest
}

func resolveEthEventFilterBlockTag(tag string, headHeight abi.ChainEpoch) (abi.ChainEpoch, bool) {
	switch {
	case isEthEventHeadTag(tag):
		if headHeight > 0 {
			return headHeight - 1, true
		}
		return 0, true
	case tag == ethtypes.BlockTagEarliest:
		return 0, true
	default:
		var num ethtypes.EthUint64
		if err := num.UnmarshalJSON([]byte(`"` + tag + `"`)); err != nil || num > math.MaxInt64 {
			return 0, false
		}
		return abi.ChainEpoch(num), true
	}
}

// resolveTraceFilterBlockTag resolves the block tags supported by trace_filter
// to a numeric height. Returns (0, false) for unsupported or unparseable tags.
func resolveTraceFilterBlockTag(tag string, headHeight abi.ChainEpoch) (ethtypes.EthUint64, bool) {
	switch tag {
	case ethtypes.BlockTagPending:
		return ethtypes.EthUint64(headHeight), true
	case ethtypes.BlockTagLatest:
		if headHeight > 0 {
			return ethtypes.EthUint64(headHeight - 1), true
		}
		return 0, true
	case ethtypes.BlockTagSafe:
		// Matches trace.go's getEthBlockNumberFromString which uses (head-1)-SafeEpochDelay.
		// Note: the authoritative TipSetResolver uses head-SafeEpochDelay (no -1); if this
		// function is reused beyond trace_filter, revisit this.
		if headHeight > ethtypes.SafeEpochDelay+1 {
			return ethtypes.EthUint64(headHeight - 1 - ethtypes.SafeEpochDelay), true
		}
		return 0, true
	default:
		var num ethtypes.EthUint64
		if err := num.UnmarshalJSON([]byte(`"` + tag + `"`)); err != nil {
			return 0, false
		}
		return num, true
	}
}
