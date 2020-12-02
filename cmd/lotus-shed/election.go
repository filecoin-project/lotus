package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var electionCmd = &cli.Command{
	Name:  "election",
	Usage: "Commands related to leader election",
	Subcommands: []*cli.Command{
		electionRunDummy,
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
