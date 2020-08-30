package cli

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"sort"

	"github.com/filecoin-project/lotus/paychmgr"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
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
		paychSettleCmd,
		paychCloseCmd,
	},
}

var paychGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "Create a new payment channel or get existing one and add amount to it",
	ArgsUsage: "[fromAddress toAddress amount]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return ShowHelp(cctx, fmt.Errorf("must pass three arguments: <from> <to> <available funds>"))
		}

		from, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return ShowHelp(cctx, fmt.Errorf("failed to parse from address: %s", err))
		}

		to, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return ShowHelp(cctx, fmt.Errorf("failed to parse to address: %s", err))
		}

		amt, err := types.ParseFIL(cctx.Args().Get(2))
		if err != nil {
			return ShowHelp(cctx, fmt.Errorf("parsing amount failed: %s", err))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		// Send a message to chain to create channel / add funds to existing
		// channel
		info, err := api.PaychGet(ctx, from, to, types.BigInt(amt))
		if err != nil {
			return err
		}

		// Wait for the message to be confirmed
		chAddr, err := api.PaychGetWaitReady(ctx, info.WaitSentinel)
		if err != nil {
			return err
		}

		fmt.Fprintln(cctx.App.Writer, chAddr)
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
			fmt.Fprintln(cctx.App.Writer, v.String())
		}
		return nil
	},
}

var paychSettleCmd = &cli.Command{
	Name:      "settle",
	Usage:     "Settle a payment channel",
	ArgsUsage: "[channelAddress]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return fmt.Errorf("must pass payment channel address")
		}

		ch, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse payment channel address: %s", err)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		mcid, err := api.PaychSettle(ctx, ch)
		if err != nil {
			return err
		}

		mwait, err := api.StateWaitMsg(ctx, mcid, build.MessageConfidence)
		if err != nil {
			return nil
		}
		if mwait.Receipt.ExitCode != 0 {
			return fmt.Errorf("settle message execution failed (exit code %d)", mwait.Receipt.ExitCode)
		}

		fmt.Fprintf(cctx.App.Writer, "Settled channel %s\n", ch)
		return nil
	},
}

var paychCloseCmd = &cli.Command{
	Name:      "collect",
	Usage:     "Collect funds for a payment channel",
	ArgsUsage: "[channelAddress]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return fmt.Errorf("must pass payment channel address")
		}

		ch, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse payment channel address: %s", err)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		mcid, err := api.PaychCollect(ctx, ch)
		if err != nil {
			return err
		}

		mwait, err := api.StateWaitMsg(ctx, mcid, build.MessageConfidence)
		if err != nil {
			return nil
		}
		if mwait.Receipt.ExitCode != 0 {
			return fmt.Errorf("collect message execution failed (exit code %d)", mwait.Receipt.ExitCode)
		}

		fmt.Fprintf(cctx.App.Writer, "Collected funds for channel %s\n", ch)
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
			return ShowHelp(cctx, fmt.Errorf("must pass two arguments: <channel> <amount>"))
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

		fmt.Fprintln(cctx.App.Writer, enc)
		return nil
	},
}

var paychVoucherCheckCmd = &cli.Command{
	Name:      "check",
	Usage:     "Check validity of payment channel voucher",
	ArgsUsage: "[channelAddress voucher]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass payment channel address and voucher to validate"))
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

		fmt.Fprintln(cctx.App.Writer, "voucher is valid")
		return nil
	},
}

var paychVoucherAddCmd = &cli.Command{
	Name:      "add",
	Usage:     "Add payment channel voucher to local datastore",
	ArgsUsage: "[channelAddress voucher]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass payment channel address and voucher"))
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
			Usage: "Print voucher as serialized string",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return ShowHelp(cctx, fmt.Errorf("must pass payment channel address"))
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

		for _, v := range sortVouchers(vouchers) {
			export := cctx.Bool("export")
			err := outputVoucher(cctx.App.Writer, v, export)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

var paychVoucherBestSpendableCmd = &cli.Command{
	Name:      "best-spendable",
	Usage:     "Print vouchers with highest value that is currently spendable for each lane",
	ArgsUsage: "[channelAddress]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "export",
			Usage: "Print voucher as serialized string",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return ShowHelp(cctx, fmt.Errorf("must pass payment channel address"))
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

		vouchersByLane, err := paychmgr.BestSpendableByLane(ctx, api, ch)
		if err != nil {
			return err
		}

		var vouchers []*paych.SignedVoucher
		for _, vchr := range vouchersByLane {
			vouchers = append(vouchers, vchr)
		}
		for _, best := range sortVouchers(vouchers) {
			export := cctx.Bool("export")
			err := outputVoucher(cctx.App.Writer, best, export)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

func sortVouchers(vouchers []*paych.SignedVoucher) []*paych.SignedVoucher {
	sort.Slice(vouchers, func(i, j int) bool {
		if vouchers[i].Lane == vouchers[j].Lane {
			return vouchers[i].Nonce < vouchers[j].Nonce
		}
		return vouchers[i].Lane < vouchers[j].Lane
	})
	return vouchers
}

func outputVoucher(w io.Writer, v *paych.SignedVoucher, export bool) error {
	var enc string
	if export {
		var err error
		enc, err = EncodedString(v)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(w, "Lane %d, Nonce %d: %s", v.Lane, v.Nonce, v.Amount.String())
	if export {
		fmt.Fprintf(w, "; %s", enc)
	}
	fmt.Fprintln(w)
	return nil
}

var paychVoucherSubmitCmd = &cli.Command{
	Name:      "submit",
	Usage:     "Submit voucher to chain to update payment channel state",
	ArgsUsage: "[channelAddress voucher]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass payment channel address and voucher"))
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

		mcid, err := api.PaychVoucherSubmit(ctx, ch, sv, nil, nil)
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

		fmt.Fprintln(cctx.App.Writer, "channel updated successfully")

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
