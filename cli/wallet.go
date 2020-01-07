package cli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/filecoin-project/go-address"
	types "github.com/filecoin-project/lotus/chain/types"
	"gopkg.in/urfave/cli.v2"
)

var walletCmd = &cli.Command{
	Name:  "wallet",
	Usage: "Manage wallet",
	Subcommands: []*cli.Command{
		walletNew,
		walletList,
		walletBalance,
		walletExport,
		walletImport,
		walletGetDefault,
		walletSetDefault,
	},
}

var walletNew = &cli.Command{
	Name:      "new",
	Usage:     "Generate a new key of the given type",
	ArgsUsage: "[bls|secp256k1]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
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
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
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
	Name:      "balance",
	Usage:     "Get account balance",
	ArgsUsage: "[account address]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
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

		fmt.Printf("%s\n", types.FIL(balance))
		return nil
	},
}

var walletGetDefault = &cli.Command{
	Name:  "default",
	Usage: "Get default wallet address",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		addr, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", addr.String())
		return nil
	},
}

var walletSetDefault = &cli.Command{
	Name:  "set-default",
	Usage: "Set default wallet address",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address to set as default")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		return api.WalletSetDefault(ctx, addr)
	},
}

var walletExport = &cli.Command{
	Name:  "export",
	Usage: "export keys",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify key to export")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ki, err := api.WalletExport(ctx, addr)
		if err != nil {
			return err
		}

		b, err := json.Marshal(ki)
		if err != nil {
			return err
		}

		fmt.Println(hex.EncodeToString(b))
		return nil
	},
}

var walletImport = &cli.Command{
	Name:  "import",
	Usage: "import keys",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var hexdata []byte
		if !cctx.Args().Present() || cctx.Args().First() == "-" {
			indata, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			hexdata = indata

		} else {
			fdata, err := ioutil.ReadFile(cctx.Args().First())
			if err != nil {
				return err
			}
			hexdata = fdata
		}
		data, err := hex.DecodeString(strings.TrimSpace(string(hexdata)))
		if err != nil {
			return err
		}

		var ki types.KeyInfo
		if err := json.Unmarshal(data, &ki); err != nil {
			return err
		}

		addr, err := api.WalletImport(ctx, &ki)
		if err != nil {
			return err
		}

		fmt.Printf("imported key %s successfully!\n", addr)
		return nil
	},
}
