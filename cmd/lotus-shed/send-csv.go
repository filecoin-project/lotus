package main

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
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
			return lcli.IncorrectNumArgs(cctx)
		}

		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

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

		if strings.TrimSpace(records[0][0]) != "Recipient" ||
			strings.TrimSpace(records[0][1]) != "FIL" ||
			strings.TrimSpace(records[0][2]) != "Method" ||
			strings.TrimSpace(records[0][3]) != "Params" {
			return xerrors.Errorf("expected header row to be \"Recipient, FIL, Method, Params\"")
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

			method, err := strconv.Atoi(strings.TrimSpace(e[2]))
			if err != nil {
				return xerrors.Errorf("failed to parse method number: %w", err)
			}

			var params []byte
			if strings.TrimSpace(e[3]) != "nil" {
				params, err = hex.DecodeString(strings.TrimSpace(e[3]))
				if err != nil {
					return xerrors.Errorf("failed to parse hexparams: %w", err)
				}
			}

			msgs = append(msgs, &types.Message{
				To:     addr,
				From:   sender,
				Value:  abi.TokenAmount(value),
				Method: abi.MethodNum(method),
				Params: params,
			})
		}

		if len(msgs) == 0 {
			return nil
		}

		var msgCids []cid.Cid
		for i, msg := range msgs {
			smsg, err := api.MpoolPushMessage(ctx, msg, nil)
			if err != nil {
				fmt.Printf("%d, ERROR %s\n", i, err)
				continue
			}

			fmt.Printf("%d, %s\n", i, smsg.Cid())

			if i > 0 && i%100 == 0 {
				fmt.Printf("catching up until latest message lands")
				_, err := api.StateWaitMsg(ctx, smsg.Cid(), 1, lapi.LookbackNoLimit, true)
				if err != nil {
					return err
				}
			}

			msgCids = append(msgCids, smsg.Cid())
		}

		fmt.Println("waiting on messages...")

		for _, msgCid := range msgCids {
			ml, err := api.StateWaitMsg(ctx, msgCid, 5, lapi.LookbackNoLimit, true)
			if err != nil {
				return err
			}
			if ml.Receipt.ExitCode != exitcode.Ok {
				fmt.Printf("MSG %s NON-ZERO EXITCODE: %s\n", msgCid, ml.Receipt.ExitCode)
			}
		}

		fmt.Println("all sent messages succeeded")
		return nil
	},
}
