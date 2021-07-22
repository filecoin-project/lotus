package cli

import (
	"encoding/hex"
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

var sendCmd = &cli.Command{
	Name:      "send",
	Usage:     "Send funds between accounts",
	ArgsUsage: "[targetAddress] [amount]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send funds from",
		},
		&cli.StringFlag{
			Name:  "gas-premium",
			Usage: "specify gas price to use in AttoFIL",
			Value: "0",
		},
		&cli.StringFlag{
			Name:  "gas-feecap",
			Usage: "specify gas fee cap to use in AttoFIL",
			Value: "0",
		},
		&cli.Int64Flag{
			Name:  "gas-limit",
			Usage: "specify gas limit",
			Value: 0,
		},
		&cli.Uint64Flag{
			Name:  "nonce",
			Usage: "specify the nonce to use",
			Value: 0,
		},
		&cli.Uint64Flag{
			Name:  "method",
			Usage: "specify method to invoke",
			Value: uint64(builtin.MethodSend),
		},
		&cli.StringFlag{
			Name:  "params-json",
			Usage: "specify invocation parameters in json",
		},
		&cli.StringFlag{
			Name:  "params-hex",
			Usage: "specify invocation parameters in hex",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Deprecated: use global 'force-send'",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("force") {
			fmt.Println("'force' flag is deprecated, use global flag 'force-send'")
		}

		if cctx.Args().Len() != 2 {
			return ShowHelp(cctx, fmt.Errorf("'send' expects two arguments, target and amount"))
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		ctx := ReqContext(cctx)
		var params SendParams

		params.To, err = address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return ShowHelp(cctx, fmt.Errorf("failed to parse target address: %w", err))
		}

		val, err := types.ParseFIL(cctx.Args().Get(1))
		if err != nil {
			return ShowHelp(cctx, fmt.Errorf("failed to parse amount: %w", err))
		}
		params.Val = abi.TokenAmount(val)

		if from := cctx.String("from"); from != "" {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			params.From = addr
		}

		if cctx.IsSet("gas-premium") {
			gp, err := types.BigFromString(cctx.String("gas-premium"))
			if err != nil {
				return err
			}
			params.GasPremium = &gp
		}

		if cctx.IsSet("gas-feecap") {
			gfc, err := types.BigFromString(cctx.String("gas-feecap"))
			if err != nil {
				return err
			}
			params.GasFeeCap = &gfc
		}

		if cctx.IsSet("gas-limit") {
			limit := cctx.Int64("gas-limit")
			params.GasLimit = &limit
		}

		params.Method = abi.MethodNum(cctx.Uint64("method"))

		if cctx.IsSet("params-json") {
			decparams, err := srv.DecodeTypedParamsFromJSON(ctx, params.To, params.Method, cctx.String("params-json"))
			if err != nil {
				return fmt.Errorf("failed to decode json params: %w", err)
			}
			params.Params = decparams
		}
		if cctx.IsSet("params-hex") {
			if params.Params != nil {
				return fmt.Errorf("can only specify one of 'params-json' and 'params-hex'")
			}
			decparams, err := hex.DecodeString(cctx.String("params-hex"))
			if err != nil {
				return fmt.Errorf("failed to decode hex params: %w", err)
			}
			params.Params = decparams
		}

		if cctx.IsSet("nonce") {
			n := cctx.Uint64("nonce")
			params.Nonce = &n
		}

		proto, err := srv.MessageForSend(ctx, params)
		if err != nil {
			return xerrors.Errorf("creating message prototype: %w", err)
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		fmt.Fprintf(cctx.App.Writer, "%s\n", sm.Cid())
		return nil
	},
}
