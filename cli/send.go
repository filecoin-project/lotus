package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	cid "github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
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
			Usage: "must be specified for the action to take effect if maybe SysErrInsufficientFunds etc",
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
		var params sendParams

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

		gp, err := types.BigFromString(cctx.String("gas-premium"))
		if err != nil {
			return err
		}
		params.GasPremium = gp

		gfc, err := types.BigFromString(cctx.String("gas-feecap"))
		if err != nil {
			return err
		}
		params.GasFeeCap = gfc
		params.GasLimit = cctx.Int64("gas-limit")

		params.Method = abi.MethodNum(cctx.Uint64("method"))

		if cctx.IsSet("params-json") {
			decparams, err := decodeTypedParams(ctx, api, params.To, params.Method, cctx.String("params-json"))
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
		params.Force = cctx.Bool("force")

		if cctx.IsSet("nonce") {
			params.Nonce.Set = true
			params.Nonce.N = cctx.Uint64("nonce")
		}

		msgCid, err := send(ctx, api, params)

		if err != nil {
			return xerrors.Errorf("executing send: %w", err)
		}

		fmt.Printf("%s\n", msgCid)
		return nil
	},
}

type sendAPIs interface {
	MpoolPush(context.Context, *types.SignedMessage) (cid.Cid, error)
	MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error)

	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletDefaultAddress(context.Context) (address.Address, error)
	WalletSignMessage(context.Context, address.Address, *types.Message) (*types.SignedMessage, error)
}

type sendParams struct {
	To   address.Address
	From address.Address
	Val  abi.TokenAmount

	GasPremium abi.TokenAmount
	GasFeeCap  abi.TokenAmount
	GasLimit   int64

	Nonce struct {
		N   uint64
		Set bool
	}
	Method abi.MethodNum
	Params []byte

	Force bool
}

func send(ctx context.Context, api sendAPIs, params sendParams) (cid.Cid, error) {
	if params.From == address.Undef {
		defaddr, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return cid.Undef, err
		}
		params.From = defaddr
	}

	msg := &types.Message{
		From:  params.From,
		To:    params.To,
		Value: params.Val,

		GasPremium: params.GasPremium,
		GasFeeCap:  params.GasFeeCap,
		GasLimit:   params.GasLimit,

		Method: params.Method,
		Params: params.Params,
	}

	if !params.Force {
		// Funds insufficient check
		fromBalance, err := api.WalletBalance(ctx, msg.From)
		if err != nil {
			return cid.Undef, err
		}
		totalCost := types.BigAdd(types.BigMul(msg.GasFeeCap, types.NewInt(uint64(msg.GasLimit))), msg.Value)

		if fromBalance.LessThan(totalCost) {
			fmt.Printf("WARNING: From balance %s less than total cost %s\n", types.FIL(fromBalance), types.FIL(totalCost))
			return cid.Undef, fmt.Errorf("--force must be specified for this action to have an effect; you have been warned")
		}
	}

	if params.Nonce.Set {
		msg.Nonce = params.Nonce.N
		sm, err := api.WalletSignMessage(ctx, params.From, msg)
		if err != nil {
			return cid.Undef, err
		}

		_, err = api.MpoolPush(ctx, sm)
		if err != nil {
			return cid.Undef, err
		}

		return sm.Cid(), nil
	}

	sm, err := api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, err
	}

	return sm.Cid(), nil
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
