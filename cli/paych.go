package cli

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/filecoin-project/lotus/build"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/urfave/cli/v2"

	types "github.com/filecoin-project/lotus/chain/types"
)

var paychCmd = &cli.Command{
	Name:  "paych",
	Usage: "Manage payment channels",
	Subcommands: []*cli.Command{
		paychGetCmd,
		paychListCmd,
		paychVoucherCmd,
	},
}

var paychGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "Create a new payment channel or get existing one",
	ArgsUsage: "[fromAddress toAddress amount]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return fmt.Errorf("must pass three arguments: <from> <to> <available funds in FIL>")
		}

		from, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse from address: %s", err)
		}

		to, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return fmt.Errorf("failed to parse to address: %s", err)
		}

		amt, err := types.ParseFIL(cctx.Args().Get(2))
		if err != nil {
			return fmt.Errorf("parsing amount as whole FIL failed: %s", err)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		info, err := api.PaychGet(ctx, from, to, types.BigInt(amt))
		if err != nil {
			return err
		}

		fmt.Println(info.Channel.String())
		return nil
	},
}

var paychListCmd = &cli.Command{
	Name:  "list",
	Usage: "List all locally registered payment channels",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		chs, err := api.PaychList(ctx)
		if err != nil {
			return err
		}

		for _, v := range chs {
			fmt.Println(v.String())
		}
		return nil
	},
}

var paychVoucherCmd = &cli.Command{
	Name:  "voucher",
	Usage: "Interact with payment channel vouchers",
	Subcommands: []*cli.Command{
		paychVoucherCreateCmd,
		paychVoucherCheckCmd,
		paychVoucherAddCmd,
		paychVoucherListCmd,
		paychVoucherBestSpendableCmd,
		paychVoucherSubmitCmd,
	},
}

var paychVoucherCreateCmd = &cli.Command{
	Name:      "create",
	Usage:     "Create a signed payment channel voucher",
	ArgsUsage: "[channelAddress amount]",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "lane",
			Value: 0,
			Usage: "specify payment channel lane to use",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return fmt.Errorf("must pass two arguments: <channel> <amount>")
		}

		ch, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		amt, err := types.BigFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		lane := cctx.Int("lane")

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		sv, err := api.PaychVoucherCreate(ctx, ch, amt, uint64(lane))
		if err != nil {
			return err
		}

		enc, err := EncodedString(sv)
		if err != nil {
			return err
		}

		fmt.Println(enc)
		return nil
	},
}

var paychVoucherCheckCmd = &cli.Command{
	Name:      "check",
	Usage:     "Check validity of payment channel voucher",
	ArgsUsage: "[channelAddress voucher]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return fmt.Errorf("must pass payment channel address and voucher to validate")
		}

		ch, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		sv, err := types.DecodeSignedVoucher(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if err := api.PaychVoucherCheckValid(ctx, ch, sv); err != nil {
			return err
		}

		fmt.Println("voucher is valid")
		return nil
	},
}

var paychVoucherAddCmd = &cli.Command{
	Name:      "add",
	Usage:     "Add payment channel voucher to local datastore",
	ArgsUsage: "[channelAddress voucher]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return fmt.Errorf("must pass payment channel address and voucher")
		}

		ch, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		sv, err := types.DecodeSignedVoucher(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		// TODO: allow passing proof bytes
		if _, err := api.PaychVoucherAdd(ctx, ch, sv, nil, types.NewInt(0)); err != nil {
			return err
		}

		return nil
	},
}

var paychVoucherListCmd = &cli.Command{
	Name:      "list",
	Usage:     "List stored vouchers for a given payment channel",
	ArgsUsage: "[channelAddress]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "export",
			Usage: "Print export strings",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return fmt.Errorf("must pass payment channel address")
		}

		ch, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		vouchers, err := api.PaychVoucherList(ctx, ch)
		if err != nil {
			return err
		}

		for _, v := range vouchers {
			if cctx.Bool("export") {
				enc, err := EncodedString(v)
				if err != nil {
					return err
				}

				fmt.Printf("Lane %d, Nonce %d: %s; %s\n", v.Lane, v.Nonce, v.Amount.String(), enc)
			} else {
				fmt.Printf("Lane %d, Nonce %d: %s\n", v.Lane, v.Nonce, v.Amount.String())
			}
		}

		return nil
	},
}

var paychVoucherBestSpendableCmd = &cli.Command{
	Name:      "best-spendable",
	Usage:     "Print voucher with highest value that is currently spendable",
	ArgsUsage: "[channelAddress]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return fmt.Errorf("must pass payment channel address")
		}

		ch, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		vouchers, err := api.PaychVoucherList(ctx, ch)
		if err != nil {
			return err
		}

		var best *paych.SignedVoucher
		for _, v := range vouchers {
			spendable, err := api.PaychVoucherCheckSpendable(ctx, ch, v, nil, nil)
			if err != nil {
				return err
			}
			if spendable {
				if best == nil || v.Amount.GreaterThan(best.Amount) {
					best = v
				}
			}
		}

		if best == nil {
			return fmt.Errorf("No spendable vouchers for that channel")
		}

		enc, err := EncodedString(best)
		if err != nil {
			return err
		}

		fmt.Println(enc)
		fmt.Printf("Amount: %s\n", best.Amount)
		return nil
	},
}

var paychVoucherSubmitCmd = &cli.Command{
	Name:      "submit",
	Usage:     "Submit voucher to chain to update payment channel state",
	ArgsUsage: "[channelAddress voucher]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return fmt.Errorf("must pass payment channel address and voucher")
		}

		ch, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		sv, err := types.DecodeSignedVoucher(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		mcid, err := api.PaychVoucherSubmit(ctx, ch, sv)
		if err != nil {
			return err
		}

		mwait, err := api.StateWaitMsg(ctx, mcid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if mwait.Receipt.ExitCode != 0 {
			return fmt.Errorf("message execution failed (exit code %d)", mwait.Receipt.ExitCode)
		}

		fmt.Println("channel updated successfully")

		return nil
	},
}

func EncodedString(sv *paych.SignedVoucher) (string, error) {
	buf := new(bytes.Buffer)
	if err := sv.MarshalCBOR(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}
