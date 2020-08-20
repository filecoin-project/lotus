package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/filecoin-project/lotus/chain/wallet"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/urfave/cli/v2"
)

func main() {

	app := cli.NewApp()
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "type",
			Aliases: []string{"t"},
			Value:   "bls",
			Usage:   "specify key type to generate (bls or secp256k1)",
		},
	}
	app.Action = func(cctx *cli.Context) error {
		memks := wallet.NewMemKeyStore()
		w, err := wallet.NewWallet(memks)
		if err != nil {
			return err
		}

		var kt crypto.SigType
		switch cctx.String("type") {
		case "bls":
			kt = crypto.SigTypeBLS
		case "secp256k1":
			kt = crypto.SigTypeSecp256k1
		default:
			return fmt.Errorf("unrecognized key type: %q", cctx.String("type"))
		}

		kaddr, err := w.GenerateKey(kt)
		if err != nil {
			return err
		}

		ki, err := w.Export(kaddr)
		if err != nil {
			return err
		}

		fi, err := os.Create(fmt.Sprintf("%s.key", kaddr))
		if err != nil {
			return err
		}
		defer fi.Close()

		b, err := json.Marshal(ki)
		if err != nil {
			return err
		}

		if _, err := fi.Write(b); err != nil {
			return fmt.Errorf("failed to write key info to file: %w", err)
		}

		fmt.Println("Generated new key: ", kaddr)
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
