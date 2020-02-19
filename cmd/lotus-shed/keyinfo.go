package main

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"gopkg.in/urfave/cli.v2"

	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

type walletInfo struct {
	Type      string
	Address   string
	PublicKey string
}

func (wi walletInfo) String() string {
	bs, _ := json.Marshal(wi)
	return string(bs)
}

var keyinfoCmd = &cli.Command{
	Name:        "keyinfo",
	Description: "decode a keyinfo",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "format",
			Value: "{{.Address}}",
			Usage: "Format to output",
		},
	},
	Action: func(cctx *cli.Context) error {
		format := cctx.String("format")

		var input io.Reader

		if cctx.Args().Len() == 0 {
			input = os.Stdin
		} else {
			input = strings.NewReader(cctx.Args().First())
		}

		bytes, err := ioutil.ReadAll(input)

		data, err := hex.DecodeString(strings.TrimSpace(string(bytes)))
		if err != nil {
			return err
		}

		var ki types.KeyInfo
		if err := json.Unmarshal(data, &ki); err != nil {
			return err
		}

		key, err := wallet.NewKey(ki)
		if err != nil {
			return err
		}

		bs, err := json.Marshal(key)
		if err != nil {
			return err
		}

		var wi walletInfo
		if err := json.Unmarshal(bs, &wi); err != nil {
			return err
		}

		tmpl, err := template.New("").Parse(format)
		if err != nil {
			return err
		}

		return tmpl.Execute(os.Stdout, wi)
	},
}
