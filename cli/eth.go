package cli

import (
	"context"
	"encoding/hex"
	"fmt"

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
			if ethAddr[:2] == "0x" {
				ethAddr = ethAddr[2:]
			}
			addr, err := hex.DecodeString(ethAddr)
			if err != nil {
				return err
			}
			copy(eaddr[:], addr)
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

		fromEthAddr, err := ethtypes.EthAddressFromHex(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		toEthAddr, err := ethtypes.EthAddressFromHex(cctx.Args().Get(1))
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

	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(faddr)
	if err != nil {
		return ethtypes.EthAddress{}, addr, err
	}

	return ethAddr, faddr, nil
}
