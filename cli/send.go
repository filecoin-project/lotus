package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var SendCmd = &cli.Command{
	Name:      "send",
	Usage:     "Send funds between accounts",
	ArgsUsage: "[targetAddress] [amount]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send funds from",
		},
		&cli.StringFlag{
			Name:  "from-eth-addr",
			Usage: "optionally specify the eth addr to send funds from",
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
		&cli.StringFlag{
			Name:  "csv",
			Usage: "send multiple transactions from a CSV file (format: Recipient,FIL,Method,Params)",
		},
	},
	Action: func(cctx *cli.Context) error {
		if csvFile := cctx.String("csv"); csvFile != "" {
			return handleCSVSend(cctx, csvFile)
		}

		if cctx.IsSet("force") {
			fmt.Println("'force' flag is deprecated, use global flag 'force-send'")
		}

		if cctx.NArg() != 2 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		ctx := ReqContext(cctx)
		var params SendParams

		is0xRecipient := false
		params.To, err = address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			// could be an ETH address
			ea, err := ethtypes.ParseEthAddress(cctx.Args().Get(0))
			if err != nil {
				return ShowHelp(cctx, fmt.Errorf("failed to parse target address; address must be a valid FIL address or an ETH address: %w", err))
			}
			is0xRecipient = true
			// this will be either "f410f..." or "f0..."
			params.To, err = ea.ToFilecoinAddress()
			if err != nil {
				return ShowHelp(cctx, fmt.Errorf("failed to convert ETH address to FIL address: %w", err))
			}
			// ideally, this should never happen
			if !(params.To.Protocol() == address.ID || params.To.Protocol() == address.Delegated) {
				return ShowHelp(cctx, fmt.Errorf("ETH addresses can only map to a FIL addresses starting with f410f or f0"))
			}
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
		} else if from := cctx.String("from-eth-addr"); from != "" {
			eaddr, err := ethtypes.ParseEthAddress(from)
			if err != nil {
				return err
			}
			faddr, err := eaddr.ToFilecoinAddress()
			if err != nil {
				fmt.Println("error on conversion to faddr")
				return err
			}
			fmt.Println("f4 addr: ", faddr)
			params.From = faddr
		} else {
			defaddr, err := srv.FullNodeAPI().WalletDefaultAddress(ctx)
			if err != nil {
				return fmt.Errorf("failed to get default address: %w", err)
			}
			params.From = defaddr
		}

		if cctx.IsSet("params-hex") {
			decparams, err := hex.DecodeString(cctx.String("params-hex"))
			if err != nil {
				return fmt.Errorf("failed to decode hex params: %w", err)
			}
			params.Params = decparams
		}

		if ethtypes.IsEthAddress(params.From) || is0xRecipient {
			// Method numbers don't make sense from eth accounts.
			if cctx.IsSet("method") {
				return xerrors.Errorf("messages from f410f addresses may not specify a method number")
			}

			// Now, figure out the correct method number from the recipient.
			if params.To == builtintypes.EthereumAddressManagerActorAddr {
				params.Method = builtintypes.MethodsEAM.CreateExternal
			} else {
				params.Method = builtintypes.MethodsEVM.InvokeContract
			}

			if cctx.IsSet("params-json") {
				return xerrors.Errorf("may not call with json parameters from an eth account")
			}

			// And format the parameters, if present.
			if len(params.Params) > 0 {
				var buf bytes.Buffer
				if err := cbg.WriteByteArray(&buf, params.Params); err != nil {
					return xerrors.Errorf("failed to marshal EVM parameters")
				}
				params.Params = buf.Bytes()
			}

			// We can only send to an f410f or f0 address.
			if !(params.To.Protocol() == address.ID || params.To.Protocol() == address.Delegated) {
				api := srv.FullNodeAPI()
				// Resolve id addr if possible.
				params.To, err = api.StateLookupID(ctx, params.To, types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("addresses starting with f410f can only send to other addresses starting with f410f, or id addresses. could not find id address for %s", params.To.String())
				}
			}
		} else {
			params.Method = abi.MethodNum(cctx.Uint64("method"))
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "Sending message from: %s\nSending message to: %s\nUsing Method: %d\n", params.From.String(), params.To.String(), params.Method)

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

		if cctx.IsSet("params-json") {
			if params.Params != nil {
				return fmt.Errorf("can only specify one of 'params-json' and 'params-hex'")
			}
			decparams, err := srv.DecodeTypedParamsFromJSON(ctx, params.To, params.Method, cctx.String("params-json"))
			if err != nil {
				return fmt.Errorf("failed to decode json params: %w", err)
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
			if strings.Contains(err.Error(), "no current EF") {
				return xerrors.Errorf("transaction rejected on ledger: %w", err)
			}
			return err
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "%s\n", sm.Cid())
		return nil
	},
}

func handleCSVSend(cctx *cli.Context, csvFile string) error {
	srv, err := GetFullNodeServices(cctx)
	if err != nil {
		return err
	}
	defer srv.Close() //nolint:errcheck

	ctx := ReqContext(cctx)

	var fromAddr address.Address
	if from := cctx.String("from"); from != "" {
		addr, err := address.NewFromString(from)
		if err != nil {
			return err
		}
		fromAddr = addr
	} else {
		defaddr, err := srv.FullNodeAPI().WalletDefaultAddress(ctx)
		if err != nil {
			return fmt.Errorf("failed to get default address: %w", err)
		}
		fromAddr = defaddr
	}

	// Print sending address
	_, _ = fmt.Fprintf(cctx.App.Writer, "Sending messages from: %s\n", fromAddr.String())

	fileReader, err := os.Open(csvFile)
	if err != nil {
		return xerrors.Errorf("read csv: %w", err)
	}
	defer func() {
		if err := fileReader.Close(); err != nil {
			log.Errorf("failed to close csv file: %v", err)
		}
	}()

	r := csv.NewReader(fileReader)
	records, err := r.ReadAll()
	if err != nil {
		return xerrors.Errorf("read csv: %w", err)
	}

	// Validate header
	if len(records) == 0 ||
		len(records[0]) != 4 ||
		strings.TrimSpace(records[0][0]) != "Recipient" ||
		strings.TrimSpace(records[0][1]) != "FIL" ||
		strings.TrimSpace(records[0][2]) != "Method" ||
		strings.TrimSpace(records[0][3]) != "Params" {
		return xerrors.Errorf("expected header row to be \"Recipient,FIL,Method,Params\"")
	}

	// First pass: validate and build params
	var sendParams []SendParams
	totalAmount := abi.NewTokenAmount(0)

	for i, e := range records[1:] {
		if len(e) != 4 {
			return xerrors.Errorf("row %d has %d fields, expected 4", i, len(e))
		}

		var params SendParams
		params.From = fromAddr

		// Parse recipient
		var err error
		params.To, err = address.NewFromString(e[0])
		if err != nil {
			return xerrors.Errorf("failed to parse address in row %d: %w", i, err)
		}

		// Parse value
		val, err := types.ParseFIL(e[1])
		if err != nil {
			return xerrors.Errorf("failed to parse amount in row %d: %w", i, err)
		}
		params.Val = abi.TokenAmount(val)
		totalAmount = types.BigAdd(totalAmount, params.Val)

		// Parse method
		method, err := strconv.Atoi(strings.TrimSpace(e[2]))
		if err != nil {
			return xerrors.Errorf("failed to parse method number in row %d: %w", i, err)
		}
		params.Method = abi.MethodNum(method)

		// Parse params
		if strings.TrimSpace(e[3]) != "nil" {
			params.Params, err = hex.DecodeString(strings.TrimSpace(e[3]))
			if err != nil {
				return xerrors.Errorf("failed to parse hex params in row %d: %w", i, err)
			}
		}

		sendParams = append(sendParams, params)
	}

	// Check sender balance
	senderBalance, err := srv.FullNodeAPI().WalletBalance(ctx, fromAddr)
	if err != nil {
		return xerrors.Errorf("failed to get sender balance: %w", err)
	}

	if senderBalance.LessThan(totalAmount) {
		return xerrors.Errorf("insufficient funds: need %s FIL, have %s FIL",
			types.FIL(totalAmount), types.FIL(senderBalance))
	}

	// Second pass: perform sends
	for i, params := range sendParams {
		proto, err := srv.MessageForSend(ctx, params)
		if err != nil {
			return xerrors.Errorf("creating message prototype for row %d: %w", i, err)
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return xerrors.Errorf("sending message for row %d: %w", i, err)
		}

		fmt.Printf("Sent message %d: %s\n", i, sm.Cid())
	}

	return nil
}
