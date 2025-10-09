package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var EvmCmd = &cli.Command{
	Name:  "evm",
	Usage: "Commands related to the Filecoin EVM runtime",
	Subcommands: []*cli.Command{
		EvmDeployCmd,
		EvmInvokeCmd,
		EvmGetInfoCmd,
		EvmCallSimulateCmd,
		EvmGetContractAddress,
		EvmGetBytecode,
	},
}

var EvmGetInfoCmd = &cli.Command{
	Name:      "stat",
	Usage:     "Print eth/filecoin addrs and code cid",
	ArgsUsage: "address",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		addrString := cctx.Args().Get(0)

		var faddr address.Address
		var eaddr ethtypes.EthAddress
		addr, err := address.NewFromString(addrString)
		if err != nil { // This isn't a filecoin address
			eaddr, err = ethtypes.ParseEthAddress(addrString)
			if err != nil { // This isn't an Eth address either
				return xerrors.Errorf("address is not a filecoin or eth address")
			}
			faddr, err = eaddr.ToFilecoinAddress()
			if err != nil {
				return err
			}
		} else {
			eaddr, faddr, err = ethAddrFromFilecoinAddress(ctx, addr, api)
			if err != nil {
				return err
			}
		}

		actor, err := api.StateGetActor(ctx, faddr, types.EmptyTSK)
		fmt.Println("Filecoin address: ", faddr)
		fmt.Println("Eth address: ", eaddr)
		if err != nil {
			fmt.Printf("Actor lookup failed for faddr %s with error: %s\n", faddr, err)
		} else {
			idAddr, err := api.StateLookupID(ctx, faddr, types.EmptyTSK)
			if err == nil {
				fmt.Println("ID address: ", idAddr)
				fmt.Println("Code cid: ", actor.Code.String())
				fmt.Println("Actor Type: ", builtin.ActorNameByCode(actor.Code))
			}
		}

		return nil
	},
}

var EvmCallSimulateCmd = &cli.Command{
	Name:      "call",
	Usage:     "Simulate an eth contract call",
	ArgsUsage: "[from] [to] [params]",
	Action: func(cctx *cli.Context) error {

		if cctx.NArg() != 3 {
			return IncorrectNumArgs(cctx)
		}

		fromEthAddr, err := ethtypes.ParseEthAddress(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		toEthAddr, err := ethtypes.ParseEthAddress(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		params, err := ethtypes.DecodeHexStringTrimSpace(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		res, err := api.EthCall(ctx, ethtypes.EthCall{
			From: &fromEthAddr,
			To:   &toEthAddr,
			Data: params,
		}, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		if err != nil {
			fmt.Println("Eth call fails, return val: ", res)
			return err
		}

		fmt.Println("Result: ", res)

		return nil

	},
}

var EvmGetContractAddress = &cli.Command{
	Name:      "contract-address",
	Usage:     "Generate contract address from smart contract code",
	ArgsUsage: "[senderEthAddr] [salt] [contractHexPath]",
	Action: func(cctx *cli.Context) error {

		if cctx.NArg() != 3 {
			return IncorrectNumArgs(cctx)
		}

		sender, err := ethtypes.ParseEthAddress(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		salt, err := ethtypes.DecodeHexStringTrimSpace(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("Could not decode salt: %w", err)
		}
		if len(salt) > 32 {
			return xerrors.Errorf("Len of salt bytes greater than 32")
		}
		var fsalt [32]byte
		copy(fsalt[:], salt[:])

		contractBin := cctx.Args().Get(2)
		if err != nil {
			return err
		}
		contractHex, err := os.ReadFile(contractBin)
		if err != nil {

			return err
		}
		contract, err := ethtypes.DecodeHexStringTrimSpace(string(contractHex))
		if err != nil {
			return xerrors.Errorf("Could not decode contract file: %w", err)
		}

		contractAddr, err := ethtypes.GetContractEthAddressFromCode(sender, fsalt, contract)
		if err != nil {
			return err
		}

		fmt.Println("Contract Eth address: ", contractAddr)

		return nil
	},
}

var EvmDeployCmd = &cli.Command{
	Name:      "deploy",
	Usage:     "Deploy an EVM smart contract and return its address",
	ArgsUsage: "contract",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to use for sending the creation message",
		},
		&cli.BoolFlag{
			Name:  "hex",
			Usage: "use when input contract is in hex",
		},
		&cli.BoolFlag{
			Name:  "wait",
			Usage: "wait for message execution before returning (default: true)",
			Value: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		afmt := NewAppFmt(cctx.App)

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if argc := cctx.Args().Len(); argc != 1 {
			return xerrors.Errorf("must pass the contract init code")
		}

		contract, err := os.ReadFile(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("failed to read contract: %w", err)
		}
		if cctx.Bool("hex") {
			contract, err = ethtypes.DecodeHexStringTrimSpace(string(contract))
			if err != nil {
				return xerrors.Errorf("failed to decode contract: %w", err)
			}
		}

		var fromAddr address.Address
		if from := cctx.String("from"); from == "" {
			fromAddr, err = api.WalletDefaultAddress(ctx)
		} else {
			fromAddr, err = address.NewFromString(from)
		}
		if err != nil {
			return err
		}

		initcode := abi.CborBytes(contract)
		params, err := actors.SerializeParams(&initcode)
		if err != nil {
			return fmt.Errorf("failed to serialize Create params: %w", err)
		}

		msg := &types.Message{
			To:     builtintypes.EthereumAddressManagerActorAddr,
			From:   fromAddr,
			Value:  big.Zero(),
			Method: builtintypes.MethodsEAM.CreateExternal,
			Params: params,
		}

		// TODO: On Jan 11th, we decided to add an `EAM#create_external` method
		//  that uses the nonce of the caller instead of taking a user-supplied nonce.
		//  Track: https://github.com/filecoin-project/ref-fvm/issues/1255
		//  When that's implemented, we should migrate the CLI to use that,
		//  as `EAM#create` will be reserved for the EVM runtime actor.
		// TODO: this is very racy. It may assign a _different_ nonce than the expected one.
		afmt.Println("sending message...")
		smsg, err := api.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return xerrors.Errorf("failed to push message: %w", err)
		}

		afmt.Printf("Message CID: %s\n", smsg.Cid())

		if cctx.Bool("wait") {
			afmt.Println("waiting for message to execute...")
			wait, err := api.StateWaitMsg(ctx, smsg.Cid(), 0)
			if err != nil {
				return xerrors.Errorf("error waiting for message: %w", err)
			}

			afmt.Printf("Exit Code: %d\n", wait.Receipt.ExitCode)
			afmt.Printf("Gas Used: %d\n", wait.Receipt.GasUsed)

			if wait.Receipt.ExitCode != 0 {
				return xerrors.Errorf("actor execution failed")
			}

			var result eam.CreateReturn
			r := bytes.NewReader(wait.Receipt.Return)
			if err := result.UnmarshalCBOR(r); err != nil {
				return xerrors.Errorf("error unmarshaling return value: %w", err)
			}

			addr, err := address.NewIDAddress(result.ActorID)
			if err != nil {
				return err
			}

			afmt.Printf("Actor ID: %d\n", result.ActorID)
			afmt.Printf("ID Address: %s\n", addr)
			afmt.Printf("Robust Address: %s\n", result.RobustAddress)
			afmt.Printf("Eth Address: %s\n", "0x"+hex.EncodeToString(result.EthAddress[:]))

			ea, err := ethtypes.CastEthAddress(result.EthAddress[:])
			if err != nil {
				return fmt.Errorf("failed to create ethereum address: %w", err)
			}

			delegated, err := ea.ToFilecoinAddress()
			if err != nil {
				return fmt.Errorf("failed to calculate f4 address: %w", err)
			}

			afmt.Printf("f4 Address: %s\n", delegated)

			if len(wait.Receipt.Return) > 0 {
				result := base64.StdEncoding.EncodeToString(wait.Receipt.Return)
				afmt.Printf("Return: %s\n", result)
			}
		}

		return nil
	},
}

var EvmInvokeCmd = &cli.Command{
	Name:      "invoke",
	Usage:     "Invoke an EVM smart contract using the specified CALLDATA",
	ArgsUsage: "address calldata",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to use for sending the exec message",
		}, &cli.IntFlag{
			Name:  "value",
			Usage: "optionally specify the value to be sent with the invocation message",
		},
	},
	Action: func(cctx *cli.Context) error {
		afmt := NewAppFmt(cctx.App)

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if argc := cctx.Args().Len(); argc != 2 {
			return xerrors.Errorf("must pass the address and calldata")
		}

		addr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("failed to decode address: %w", err)
		}

		var calldata []byte
		calldata, err = ethtypes.DecodeHexStringTrimSpace(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("decoding hex input data: %w", err)
		}

		var buffer bytes.Buffer
		if err := cbg.WriteByteArray(&buffer, calldata); err != nil {
			return xerrors.Errorf("failed to encode evm params as cbor: %w", err)
		}
		calldata = buffer.Bytes()

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

		val := abi.NewTokenAmount(cctx.Int64("value"))
		msg := &types.Message{
			To:     addr,
			From:   fromAddr,
			Value:  val,
			Method: builtintypes.MethodsEVM.InvokeContract,
			Params: calldata,
		}

		afmt.Println("sending message...")
		smsg, err := api.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return xerrors.Errorf("failed to push message: %w", err)
		}

		afmt.Println("waiting for message to execute...")
		wait, err := api.StateWaitMsg(ctx, smsg.Cid(), 0)
		if err != nil {
			return xerrors.Errorf("error waiting for message: %w", err)
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			return xerrors.Errorf("actor execution failed")
		}

		afmt.Println("Gas used: ", wait.Receipt.GasUsed)
		result, err := cbg.ReadByteArray(bytes.NewBuffer(wait.Receipt.Return), uint64(len(wait.Receipt.Return)))
		if err != nil {
			return xerrors.Errorf("evm result not correctly encoded: %w", err)
		}

		if len(result) > 0 {
			afmt.Println(hex.EncodeToString(result))
		} else {
			afmt.Println("OK")
		}

		if eventsRoot := wait.Receipt.EventsRoot; eventsRoot != nil {
			afmt.Println("Events emitted:")

			s := &apiIpldStore{ctx, api}
			amt, err := amt4.LoadAMT(ctx, s, *eventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
			if err != nil {
				return err
			}

			var evt types.Event
			err = amt.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
				fmt.Printf("%x\n", deferred.Raw)
				if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
					return err
				}
				if err != nil {
					return err
				}
				fmt.Printf("\tEmitter ID: %s\n", evt.Emitter)
				for _, e := range evt.Entries {
					value, err := cbg.ReadByteArray(bytes.NewBuffer(e.Value), uint64(len(e.Value)))
					if err != nil {
						return err
					}
					fmt.Printf("\t\tKey: %s, Value: 0x%x, Flags: b%b\n", e.Key, value, e.Flags)
				}
				return nil

			})
		}
		if err != nil {
			return err
		}

		return nil
	},
}

func ethAddrFromFilecoinAddress(ctx context.Context, addr address.Address, fnapi v0api.FullNode) (ethtypes.EthAddress, address.Address, error) {
	var faddr address.Address
	var err error

	switch addr.Protocol() {
	case address.BLS, address.SECP256K1:
		faddr, err = fnapi.StateLookupID(ctx, addr, types.EmptyTSK)
		if err != nil {
			return ethtypes.EthAddress{}, addr, err
		}
	case address.Actor, address.ID:
		faddr, err = fnapi.StateLookupID(ctx, addr, types.EmptyTSK)
		if err != nil {
			return ethtypes.EthAddress{}, addr, err
		}
		fAct, err := fnapi.StateGetActor(ctx, faddr, types.EmptyTSK)
		if err != nil {
			return ethtypes.EthAddress{}, addr, err
		}
		if fAct.DelegatedAddress != nil && (*fAct.DelegatedAddress).Protocol() == address.Delegated {
			faddr = *fAct.DelegatedAddress
		}
	case address.Delegated:
		faddr = addr
	default:
		return ethtypes.EthAddress{}, addr, xerrors.Errorf("Filecoin address doesn't match known protocols")
	}

	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(faddr)
	if err != nil {
		return ethtypes.EthAddress{}, addr, err
	}

	return ethAddr, faddr, nil
}

var EvmGetBytecode = &cli.Command{
	Name:      "bytecode",
	Usage:     "Write the bytecode of a smart contract to a file",
	ArgsUsage: "[contract-address] [file-name]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "bin",
			Usage: "write the bytecode as raw binary and don't hex-encode",
		},
	},
	Action: func(cctx *cli.Context) error {

		if cctx.NArg() != 2 {
			return IncorrectNumArgs(cctx)
		}

		contractAddr, err := ethtypes.ParseEthAddress(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		fileName := cctx.Args().Get(1)

		api, closer, err := GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		code, err := api.EthGetCode(ctx, contractAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		if err != nil {
			return err
		}
		if !cctx.Bool("bin") {
			newCode := make([]byte, hex.EncodedLen(len(code)))
			hex.Encode(newCode, code)
			code = newCode
		}
		if err := os.WriteFile(fileName, code, 0o666); err != nil {
			return xerrors.Errorf("failed to write bytecode to file %s: %w", fileName, err)
		}

		fmt.Printf("Code for %s written to %s\n", contractAddr, fileName)
		return nil
	},
}
