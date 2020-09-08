package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/stmgr"
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
		&cli.Int64Flag{
			Name:  "nonce",
			Usage: "specify the nonce to use",
			Value: -1,
		},
		&cli.Uint64Flag{
			Name:  "method",
			Usage: "specify method to invoke",
			Value: 0,
		},
		&cli.StringFlag{
			Name:  "params-json",
			Usage: "specify invocation parameters in json",
		},
		&cli.StringFlag{
			Name:  "params-hex",
			Usage: "specify invocation parameters in hex",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return ShowHelp(cctx, fmt.Errorf("'send' expects two arguments, target and amount"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		toAddr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return ShowHelp(cctx, fmt.Errorf("failed to parse target address: %w", err))
		}

		val, err := types.ParseFIL(cctx.Args().Get(1))
		if err != nil {
			return ShowHelp(cctx, fmt.Errorf("failed to parse amount: %w", err))
		}

		var fromAddr address.Address
		if from := cctx.String("from"); from == "" {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}

			fromAddr = defaddr
		} else {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			fromAddr = addr
		}

		gp, err := types.BigFromString(cctx.String("gas-premium"))
		if err != nil {
			return err
		}
		gfc, err := types.BigFromString(cctx.String("gas-feecap"))
		if err != nil {
			return err
		}

		method := abi.MethodNum(cctx.Uint64("method"))

		var params []byte
		if cctx.IsSet("params-json") {
			decparams, err := decodeTypedParams(ctx, api, toAddr, method, cctx.String("params-json"))
			if err != nil {
				return fmt.Errorf("failed to decode json params: %w", err)
			}
			params = decparams
		}
		if cctx.IsSet("params-hex") {
			if params != nil {
				return fmt.Errorf("can only specify one of 'params-json' and 'params-hex'")
			}
			decparams, err := hex.DecodeString(cctx.String("params-hex"))
			if err != nil {
				return fmt.Errorf("failed to decode hex params: %w", err)
			}
			params = decparams
		}

		msg := &types.Message{
			From:       fromAddr,
			To:         toAddr,
			Value:      types.BigInt(val),
			GasPremium: gp,
			GasFeeCap:  gfc,
			GasLimit:   cctx.Int64("gas-limit"),
			Method:     method,
			Params:     params,
		}

		if cctx.Int64("nonce") > 0 {
			msg.Nonce = uint64(cctx.Int64("nonce"))
			sm, err := api.WalletSignMessage(ctx, fromAddr, msg)
			if err != nil {
				return err
			}

			_, err = api.MpoolPush(ctx, sm)
			if err != nil {
				return err
			}
			fmt.Println(sm.Cid())
		} else {
			sm, err := api.MpoolPushMessage(ctx, msg, nil)
			if err != nil {
				return err
			}
			fmt.Println(sm.Cid())
		}

		return nil
	},
}

func decodeTypedParams(ctx context.Context, fapi api.FullNode, to address.Address, method abi.MethodNum, paramstr string) ([]byte, error) {
	act, err := fapi.StateGetActor(ctx, to, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	methodMeta, found := stmgr.MethodsMap[act.Code][method]
	if !found {
		return nil, fmt.Errorf("method %d not found on actor %s", method, act.Code)
	}

	p := reflect.New(methodMeta.Params.Elem()).Interface().(cbg.CBORMarshaler)

	if err := json.Unmarshal([]byte(paramstr), p); err != nil {
		return nil, fmt.Errorf("unmarshaling input into params type: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := p.MarshalCBOR(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
