package filter

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
)

type LogFilter struct {
	minHeight abi.ChainEpoch // minimum epoch to apply filter or -1 if no minimum
	maxHeight abi.ChainEpoch // maximum epoch to apply filter or -1 if no maximum
	tipsetCid cid.Cid
	addresses []address.Address
}

// MatchTipset reports whether this filter matches the given tipset
func (f *LogFilter) MatchTipset(ts *types.TipSet) bool {
	if f.tipsetCid != cid.Undef {
		tsCid, err := ts.Key().Cid()
		if err != nil {
			return false
		}
		return f.tipsetCid.Equals(tsCid)
	}

	if f.minHeight >= 0 && f.minHeight > ts.Height() {
		return false
	}
	if f.maxHeight >= 0 && f.maxHeight < ts.Height() {
		return false
	}
	return true
}

// MatchMessage reports whether this filter matches the given message
func (f *LogFilter) MatchMessage(ts *types.TipSet, msg *types.Message, rcpt *types.MessageReceipt) bool {
	if f.minHeight >= 0 && f.minHeight > ts.Height() {
		return false
	}
	if f.maxHeight >= 0 && f.maxHeight < ts.Height() {
		return false
	}
	if !f.matchAddress(msg.From) {
		return false
	}

	return true
}

func (f *LogFilter) matchAddress(o address.Address) bool {
	// Assume short lists of addresses
	// TODO: binary search for longer lists
	for _, a := range f.addresses {
		if a == o {
			return true
		}
	}
	return false
}
