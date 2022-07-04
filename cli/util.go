package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/hako/durafmt"
	"github.com/ipfs/go-cid"
	"github.com/mattn/go-isatty"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
)

// Set the global default, to be overridden by individual cli flags in order
func init() {
	color.NoColor = os.Getenv("GOLOG_LOG_FMT") != "color" &&
		!isatty.IsTerminal(os.Stdout.Fd()) &&
		!isatty.IsCygwinTerminal(os.Stdout.Fd())
}

func parseTipSet(ctx context.Context, api v0api.FullNode, vals []string) (*types.TipSet, error) {
	var headers []*types.BlockHeader
	for _, c := range vals {
		blkc, err := cid.Decode(c)
		if err != nil {
			return nil, err
		}

		bh, err := api.ChainGetBlock(ctx, blkc)
		if err != nil {
			return nil, err
		}

		headers = append(headers, bh)
	}

	return types.NewTipSet(headers)
}

func EpochTime(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}

// EpochTimeTs is like EpochTime, but also outputs absolute time. `ts` is only
// used to provide a timestamp at some epoch to calculate time from. It can be
// a genesis tipset.
//
// Example output: `1944975 (01 Jul 22 08:07 CEST, 10 hours 29 minutes ago)`
func EpochTimeTs(curr, e abi.ChainEpoch, ts *types.TipSet) string {
	timeStr := time.Unix(int64(ts.MinTimestamp()+(uint64(e-ts.Height())*build.BlockDelaySecs)), 0).Format(time.RFC822)

	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s, %s ago)", e, timeStr, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (%s, now)", e, timeStr)
	case curr < e:
		return fmt.Sprintf("%d (%s, in %s)", e, timeStr, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}
