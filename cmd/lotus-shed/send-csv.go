package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var sendCsvCmd = &cli.Command{
	Name:  "send-csv",
	Usage: "Utility for sending a batch of balance transfers",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "from",
			Usage:    "specify the account to send funds from",
			Required: true,
		},
	},
	ArgsUsage: "[csvfile]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return xerrors.New("must supply path to csv file")
		}

		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		sender, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return err
		}

		fileReader, err := os.Open(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("read csv: %w", err)
		}

		defer fileReader.Close() //nolint:errcheck
		r := csv.NewReader(fileReader)
		records, err := r.ReadAll()
		if err != nil {
			return xerrors.Errorf("read csv: %w", err)
		}

		if strings.TrimSpace(records[0][0]) != "Recipient" || strings.TrimSpace(records[0][1]) != "FIL" {
			return xerrors.Errorf("expected header row to be \"Recipient, FIL\"")
		}

		var msgs []*types.Message
		for i, e := range records[1:] {
			addr, err := address.NewFromString(e[0])
			if err != nil {
				return xerrors.Errorf("failed to parse address in row %d: %w", i, err)
			}

			value, err := types.ParseFIL(strings.TrimSpace(e[1]))
			if err != nil {
				return xerrors.Errorf("failed to parse value balance: %w", err)
			}

			msgs = append(msgs, &types.Message{
				To:    addr,
				From:  sender,
				Value: abi.TokenAmount(value),
			})
		}

		var msgCids []cid.Cid
		for i, msg := range msgs {
			smsg, err := api.MpoolPushMessage(ctx, msg, nil)
			if err != nil {
				return err
			}

			fmt.Printf("sending %s to %s in msg %s\n", msg.Value.String(), msg.To, smsg.Cid())

			if i > 0 && i%100 == 0 {
				fmt.Printf("catching up until latest message lands")
				_, err := api.StateWaitMsg(ctx, smsg.Cid(), 1, lapi.LookbackNoLimit, true)
				if err != nil {
					return err
				}
			}

			msgCids = append(msgCids, smsg.Cid())
		}

		fmt.Println("waiting on messages")

		for _, msgCid := range msgCids {
			ml, err := api.StateWaitMsg(ctx, msgCid, 5, lapi.LookbackNoLimit, true)
			if err != nil {
				return err
			}
			if ml.Receipt.ExitCode != exitcode.Ok {
				fmt.Printf("MSG %s NON-ZERO EXITCODE: %s\n", msgCid, ml.Receipt.ExitCode)
			}
		}

		return nil
	},
}
