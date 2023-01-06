package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var EthCmd = &cli.Command{
	Name:  "eth",
	Usage: "Query eth contract state",
	Subcommands: []*cli.Command{
		EthGetInfoCmd,
		EthCallSimulateCmd,
		EthGetContractAddress,
	},
}

var EthGetInfoCmd = &cli.Command{
	Name:  "stat",
	Usage: "Print eth/filecoin addrs and code cid",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "filAddr",
			Usage: "Filecoin address",
		},
		&cli.StringFlag{
			Name:  "ethAddr",
			Usage: "Ethereum address",
		},
	},
	Action: func(cctx *cli.Context) error {

		filAddr := cctx.String("filAddr")
		ethAddr := cctx.String("ethAddr")

		var faddr address.Address
		var eaddr ethtypes.EthAddress

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if filAddr != "" {
			addr, err := address.NewFromString(filAddr)
			if err != nil {
				return err
			}
			eaddr, faddr, err = ethAddrFromFilecoinAddress(ctx, addr, api)
			if err != nil {
				return err
			}
		} else if ethAddr != "" {
			eaddr, err = ethtypes.NewEthAddressFromHex(ethAddr)
			if err != nil {
				return err
			}
			faddr, err = eaddr.ToFilecoinAddress()
			if err != nil {
				return err
			}
		} else {
			return xerrors.Errorf("Neither filAddr nor ethAddr specified")
		}

		actor, err := api.StateGetActor(ctx, faddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		fmt.Println("Filecoin address: ", faddr)
		fmt.Println("Eth address: ", eaddr)
		fmt.Println("Code cid: ", actor.Code.String())

		return nil

	},
}

var EthCallSimulateCmd = &cli.Command{
	Name:      "call",
	Usage:     "Simulate an eth contract call",
	ArgsUsage: "[from] [to] [params]",
	Action: func(cctx *cli.Context) error {

		if cctx.NArg() != 3 {
			return IncorrectNumArgs(cctx)
		}

		fromEthAddr, err := ethtypes.NewEthAddressFromHex(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		toEthAddr, err := ethtypes.NewEthAddressFromHex(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		params, err := hex.DecodeString(cctx.Args().Get(2))
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
		}, "")
		if err != nil {
			fmt.Println("Eth call fails, return val: ", res)
			return err
		}

		fmt.Println("Result: ", res)

		return nil

	},
}

var EthGetContractAddress = &cli.Command{
	Name:      "contract-address",
	Usage:     "Generate contract address from smart contract code",
	ArgsUsage: "[senderEthAddr] [salt] [contractHexPath]",
	Action: func(cctx *cli.Context) error {

		if cctx.NArg() != 3 {
			return IncorrectNumArgs(cctx)
		}

		sender, err := ethtypes.NewEthAddressFromHex(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		salt, err := hex.DecodeString(cctx.Args().Get(1))
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
		contract, err := hex.DecodeString(string(contractHex))
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
		if fAct.Address != nil && (*fAct.Address).Protocol() == address.Delegated {
			faddr = *fAct.Address
		}
	case address.Delegated:
		faddr = addr
	default:
		return ethtypes.EthAddress{}, addr, xerrors.Errorf("Filecoin address doesn't match known protocols")
	}

	ethAddr, err := ethtypes.NewEthAddressFromFilecoinAddress(faddr)
	if err != nil {
		return ethtypes.EthAddress{}, addr, err
	}

	return ethAddr, faddr, nil
}
