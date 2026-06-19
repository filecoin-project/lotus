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
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
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
