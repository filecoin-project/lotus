package cliutil

import (
	"fmt"
	"time"

	"github.com/hako/durafmt"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

func EpochTime(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(buildconstants.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(buildconstants.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}

// EpochTimeTs is like EpochTime, but also outputs absolute time. `ts` is only
// used to provide a timestamp at some epoch to calculate time from. It can be
// a genesis tipset.
//
// Example output: `1944975 (01 Jul 22 08:07 CEST, 10 hours 29 minutes ago)`
func EpochTimeTs(curr, e abi.ChainEpoch, ts *types.TipSet) string {
	timeStr := time.Unix(int64(ts.MinTimestamp()+(uint64(e-ts.Height())*buildconstants.BlockDelaySecs)), 0).Format(time.RFC822)

	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s, %s ago)", e, timeStr, durafmt.Parse(time.Second*time.Duration(int64(buildconstants.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (%s, now)", e, timeStr)
	case curr < e:
		return fmt.Sprintf("%d (%s, in %s)", e, timeStr, durafmt.Parse(time.Second*time.Duration(int64(buildconstants.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}
