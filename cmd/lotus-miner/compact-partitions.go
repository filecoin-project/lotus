package main

import (
	"fmt"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/v7/actors/builtin"
	"strconv"
	"strings"
)

var sectorsCompactPartitions = &cli.Command{
	Name:  "compact-partitions",
	Usage: "Compact partitions",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "deadline",
			Usage:    "compact deadline",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "partitions",
			Usage:    "compact partitions split for '-' , eg:0-1",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)
		maddr, err := getActorAddress(ctx, cctx)
		api, nCloser, err := lcli.GetFullNodeAPI(cctx)
		defer nCloser()

		deadline := cctx.Uint64("deadline")
		partitionString := cctx.String("partitions")
		partArr := strings.Split(partitionString, "-")
		partId := make([]uint64, 0)
		for _, id := range partArr {
			parseId, err := strconv.ParseUint(id, 10, 64)
			if err != nil {
				log.Infof("parse partitions error %s", err)
				return err
			}
			partId = append(partId, parseId)
		}

		partitions := bitfield.NewFromSet(partId)
		param := miner.CompactPartitionsParams{Deadline: deadline, Partitions: partitions}
		sp, err := actors.SerializeParams(&param)

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		msg := &types.Message{
			From:   mi.Worker,
			To:     maddr,
			Method: builtin.MethodsMiner.CompactPartitions,
			Value:  big.Zero(),
			Params: sp,
		}

		sm, err := api.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return err
		}

		fmt.Printf("push compact-partitions daedline: %d cid: %s\n", cctx.Uint64("deadline"), sm.Cid())

		return nil
	},
}
