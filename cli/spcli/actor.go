package spcli

import (
	"bytes"
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/impl"
)

func ActorWithdrawCmd(getActor ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "withdraw",
		Usage:     "withdraw available balance to beneficiary",
		ArgsUsage: "[amount (FIL)]",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "confidence",
				Usage: "number of block confirmations to wait for",
				Value: int(build.MessageConfidence),
			},
			&cli.BoolFlag{
				Name:  "beneficiary",
				Usage: "send withdraw message from the beneficiary address",
			},
		},
		Action: func(cctx *cli.Context) error {
			amount := abi.NewTokenAmount(0)

			if cctx.Args().Present() {
				f, err := types.ParseFIL(cctx.Args().First())
				if err != nil {
					return xerrors.Errorf("parsing 'amount' argument: %w", err)
				}

				amount = abi.TokenAmount(f)
			}

			api, acloser, err := lcli.GetFullNodeAPIV1(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			maddr, err := getActor(cctx)
			if err != nil {
				return err
			}

			res, err := impl.WithdrawBalance(ctx, api, maddr, amount, !cctx.IsSet("beneficiary"))
			if err != nil {
				return err
			}

			fmt.Printf("Requested withdrawal in message %s\nwaiting for it to be included in a block..\n", res)

			// wait for it to get mined into a block
			wait, err := api.StateWaitMsg(ctx, res, uint64(cctx.Int("confidence")), lapi.LookbackNoLimit, true)
			if err != nil {
				return xerrors.Errorf("Timeout waiting for withdrawal message %s", res)
			}

			if wait.Receipt.ExitCode.IsError() {
				return xerrors.Errorf("Failed to execute withdrawal message %s: %w", wait.Message, wait.Receipt.ExitCode.Error())
			}

			nv, err := api.StateNetworkVersion(ctx, wait.TipSet)
			if err != nil {
				return err
			}

			if nv >= network.Version14 {
				var withdrawn abi.TokenAmount
				if err := withdrawn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
					return err
				}

				fmt.Printf("Successfully withdrew %s \n", types.FIL(withdrawn))
				if withdrawn.LessThan(amount) {
					fmt.Printf("Note that this is less than the requested amount of %s\n", types.FIL(amount))
				}
			}

			return nil
		},
	}
}
