package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var EthCmd = &cli.Command{
	Name:  "eth",
	Usage: "Query eth contract state",
	Subcommands: []*cli.Command{
		EthGetAddressCmd,
		EthCallSimulateCmd,
	},
}

func ethAddrFromFilecoinAddress(ctx context.Context, addr address.Address, fnapi v0api.FullNode) (ethtypes.EthAddress, address.Address, error) {
	var faddr address.Address
	var err error

	switch addr.Protocol() {
	case address.BLS, address.SECP256K1:
		faddr, err = fnapi.StateLookupID(ctx, addr, types.EmptyTSK)
	case address.Actor, address.ID:
		f0addr, err := fnapi.StateLookupID(ctx, addr, types.EmptyTSK)
		if err != nil {
			return ethtypes.EthAddress{}, addr, err
		}
		faddr, err = fnapi.StateAccountKey(ctx, f0addr, types.EmptyTSK)
		if err != nil {
			return ethtypes.EthAddress{}, addr, err
		}
		if faddr.Protocol() == address.BLS || addr.Protocol() == address.SECP256K1 {
			faddr = f0addr
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

var EthGetAddressCmd = &cli.Command{
	Name:      "stat",
	Usage:     "Print eth/filecoin addrs and code cid",
	ArgsUsage: "[faddr]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "filaddr",
			Value: "",
			Usage: "Filecoin address",
		},
		&cli.StringFlag{
			Name:  "ethaddr",
			Value: "",
			Usage: "Ethereum address",
		},
	},
	Action: func(cctx *cli.Context) error {

		filaddr := cctx.String("filaddr")
		ethaddr := cctx.String("ethaddr")

		var faddr address.Address
		var eaddr ethtypes.EthAddress

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if filaddr != "" {
			addr, err := address.NewFromString(filaddr)
			if err != nil {
				return err
			}
			eaddr, faddr, err = ethAddrFromFilecoinAddress(ctx, addr, api)
		} else if ethaddr != "" {
			addr, err := hex.DecodeString(ethaddr)
			if err != nil {
				return err
			}
			copy(eaddr[:], addr)
			faddr, err = eaddr.ToFilecoinAddress()
			if err != nil {
				return err
			}
		} else {
			return xerrors.Errorf("Neither filaddr or ethaddr specified")
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

		var fromEthAddr ethtypes.EthAddress
		fromAddr, err := hex.DecodeString(cctx.Args().Get(0))
		if err != nil {
			return err
		}
		copy(fromEthAddr[:], fromAddr)

		var toEthAddr ethtypes.EthAddress
		toAddr, err := hex.DecodeString(cctx.Args().Get(1))
		if err != nil {
			return err
		}
		copy(toEthAddr[:], toAddr)

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
