package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var electionCmd = &cli.Command{
	Name:  "election",
	Usage: "Commands related to leader election",
	Subcommands: []*cli.Command{
		electionRunDummy,
		electionEstimate,
		electionBacktest,
	},
}

var electionRunDummy = &cli.Command{
	Name:  "run-dummy",
	Usage: "Runs dummy elections with given power",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "network-power",
			Usage: "network storage power",
		},
		&cli.StringFlag{
			Name:  "miner-power",
			Usage: "miner storage power",
		},
		&cli.Uint64Flag{
			Name:  "seed",
			Usage: "rand number",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		minerPow, err := types.BigFromString(cctx.String("miner-power"))
		if err != nil {
			return xerrors.Errorf("decoding miner-power: %w", err)
		}
		networkPow, err := types.BigFromString(cctx.String("network-power"))
		if err != nil {
			return xerrors.Errorf("decoding network-power: %w", err)
		}

		ep := &types.ElectionProof{}
		ep.VRFProof = make([]byte, 32)
		seed := cctx.Uint64("seed")
		if seed == 0 {
			seed = rand.Uint64()
		}
		binary.BigEndian.PutUint64(ep.VRFProof, seed)

		i := uint64(0)
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			binary.BigEndian.PutUint64(ep.VRFProof[8:], i)
			j := ep.ComputeWinCount(minerPow, networkPow)
			_, err := fmt.Printf("%t, %d\n", j != 0, j)
			if err != nil {
				return err
			}
			i++
		}
	},
}

var electionEstimate = &cli.Command{
	Name:  "estimate",
	Usage: "Estimate elections with given power",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "network-power",
			Usage: "network storage power",
		},
		&cli.StringFlag{
			Name:  "miner-power",
			Usage: "miner storage power",
		},
		&cli.Uint64Flag{
			Name:  "seed",
			Usage: "rand number",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {
		minerPow, err := types.BigFromString(cctx.String("miner-power"))
		if err != nil {
			return xerrors.Errorf("decoding miner-power: %w", err)
		}
		networkPow, err := types.BigFromString(cctx.String("network-power"))
		if err != nil {
			return xerrors.Errorf("decoding network-power: %w", err)
		}

		ep := &types.ElectionProof{}
		ep.VRFProof = make([]byte, 32)
		seed := cctx.Uint64("seed")
		if seed == 0 {
			seed = rand.Uint64()
		}
		binary.BigEndian.PutUint64(ep.VRFProof, seed)

		winYear := int64(0)
		for i := 0; i < builtin2.EpochsInYear; i++ {
			binary.BigEndian.PutUint64(ep.VRFProof[8:], uint64(i))
			j := ep.ComputeWinCount(minerPow, networkPow)
			winYear += j
		}
		winHour := winYear * builtin2.EpochsInHour / builtin2.EpochsInYear
		winDay := winYear * builtin2.EpochsInDay / builtin2.EpochsInYear
		winMonth := winYear * builtin2.EpochsInDay * 30 / builtin2.EpochsInYear
		fmt.Println("winInHour, winInDay, winInMonth, winInYear")
		fmt.Printf("%d, %d, %d, %d\n", winHour, winDay, winMonth, winYear)
		return nil
	},
}

var electionBacktest = &cli.Command{
	Name:      "backtest",
	Usage:     "Backtest elections with given miner",
	ArgsUsage: "[minerAddress]",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "height",
			Usage: "blockchain head height",
		},
		&cli.IntFlag{
			Name:  "count",
			Usage: "number of won elections to look for",
			Value: 120,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("GetFullNodeAPI: %w", err)
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		var head *types.TipSet
		if cctx.IsSet("height") {
			head, err = api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Uint64("height")), types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("ChainGetTipSetByHeight: %w", err)
			}
		} else {
			head, err = api.ChainHead(ctx)
			if err != nil {
				return xerrors.Errorf("ChainHead: %w", err)
			}
		}

		miner, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("miner address: %w", err)
		}

		count := cctx.Int("count")
		if count < 1 {
			return xerrors.Errorf("count: %d", count)
		}

		fmt.Println("height, winCount")
		roundEnd := head.Height() + abi.ChainEpoch(1)
		for i := 0; i < count; {
			for round := head.Height() + abi.ChainEpoch(1); round <= roundEnd; round++ {
				i++
				win, err := backTestWinner(ctx, miner, round, head, api)
				if err == nil && win != nil {
					fmt.Printf("%d, %d\n", round, win.WinCount)
				}
			}

			roundEnd = head.Height()
			head, err = api.ChainGetTipSet(ctx, head.Parents())
			if err != nil {
				break
			}
		}
		return nil
	},
}

func backTestWinner(ctx context.Context, miner address.Address, round abi.ChainEpoch, ts *types.TipSet, api v0api.FullNode) (*types.ElectionProof, error) {
	mbi, err := api.MinerGetBaseInfo(ctx, miner, round, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get mining base info: %w", err)
	}
	if mbi == nil {
		return nil, nil
	}
	if !mbi.EligibleForMining {
		return nil, nil
	}

	brand := mbi.PrevBeaconEntry
	bvals := mbi.BeaconEntries
	if len(bvals) > 0 {
		brand = bvals[len(bvals)-1]
	}

	winner, err := gen.IsRoundWinner(ctx, round, miner, brand, mbi, api)
	if err != nil {
		return nil, xerrors.Errorf("failed to check if we win next round: %w", err)
	}

	return winner, nil
}
