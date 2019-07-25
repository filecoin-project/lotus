package cli

import (
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/address"
	"gopkg.in/urfave/cli.v2"
)

var walletCmd = &cli.Command{
	Name:  "wallet",
	Usage: "Manage wallet",
	Subcommands: []*cli.Command{
		walletNew,
		walletList,
		walletBalance,
	},
}

var walletNew = &cli.Command{
	Name:  "new",
	Usage: "Generate a new key of the given type (bls or secp256k1)",
	Action: func(cctx *cli.Context) error {
		api, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		ctx := ReqContext(cctx)

		t := cctx.Args().First()
		if t == "" {
			t = "bls"
		}

		nk, err := api.WalletNew(ctx, t)
		if err != nil {
			return err
		}

		fmt.Println(nk.String())

		return nil
	},
}

var walletList = &cli.Command{
	Name:  "list",
	Usage: "List wallet address",
	Action: func(cctx *cli.Context) error {
		api, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		ctx := ReqContext(cctx)

		addrs, err := api.WalletList(ctx)
		if err != nil {
			return err
		}

		for _, addr := range addrs {
			fmt.Println(addr.String())
		}
		return nil
	},
}

var walletBalance = &cli.Command{
	Name:  "balance",
	Usage: "get account balance",
	Action: func(cctx *cli.Context) error {
		api, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		ctx := ReqContext(cctx)

		var addr address.Address
		if cctx.Args().First() != "" {
			addr, err = address.NewFromString(cctx.Args().First())
		} else {
			addr, err = api.WalletDefaultAddress(ctx)
		}
		if err != nil {
			return err
		}

		balance, err := api.WalletBalance(ctx, addr)
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", balance.String())
		return nil
	},
}
