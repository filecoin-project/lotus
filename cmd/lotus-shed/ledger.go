package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	ledgerwallet "github.com/filecoin-project/lotus/chain/wallet/ledger"
	"github.com/urfave/cli/v2"
	ledgerfil "github.com/whyrusleeping/ledger-filecoin-go"
)

var ledgerCmd = &cli.Command{
	Name:  "ledger",
	Usage: "Ledger interactions",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		ledgerListAddressesCmd,
		ledgerKeyInfoCmd,
		ledgerSignTestCmd,
	},
}

var ledgerListAddressesCmd = &cli.Command{
	Name: "list",
	Action: func(cctx *cli.Context) error {

		fl, err := ledgerfil.FindLedgerFilecoinApp()
		if err != nil {
			return err
		}

		for i := 0; i < 20; i++ {
			p := []uint32{0x80000000 + 44, 0x80000000 + 461, 0x80000000, 0, uint32(i)}
			pubk, err := fl.GetPublicKeySECP256K1(p)
			if err != nil {
				return err
			}

			addr, err := address.NewSecp256k1Address(pubk)
			if err != nil {
				return err
			}

			fmt.Printf("%s: %s\n", addr, printHDPath(p))
		}

		return nil
	},
}

func parseHDPath(s string) ([]uint32, error) {
	parts := strings.Split(s, "/")
	if parts[0] != "m" {
		return nil, fmt.Errorf("expected HD path to start with 'm'")
	}

	var out []uint32
	for _, p := range parts[1:] {
		var hard bool
		if strings.HasSuffix(p, "'") {
			p = p[:len(p)-1]
			hard = true
		}

		v, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, err
		}
		if v >= 0x80000000 {
			return nil, fmt.Errorf("path element %s too large", p)
		}

		if hard {
			v += 0x80000000
		}
		out = append(out, uint32(v))
	}
	return out, nil
}

func printHDPath(pth []uint32) string {
	s := "m/"
	for _, p := range pth {
		var hard bool
		if p >= 0x80000000 {
			p -= 0x80000000
		}
		s += fmt.Sprint(p)
		if hard {
			s += "'"
		}
		s += "/"
	}

	return strings.TrimRight(s, "/")
}

func numlist(p []uint32) string {
	var out []string
	for _, v := range p {
		out = append(out, fmt.Sprint(v))
	}
	return strings.Join(out, ",")
}

var ledgerKeyInfoCmd = &cli.Command{
	Name: "key-info",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return cli.ShowCommandHelp(cctx, cctx.Command.Name)
		}

		fl, err := ledgerfil.FindLedgerFilecoinApp()
		if err != nil {
			return err
		}

		p, err := parseHDPath(cctx.Args().First())
		if err != nil {
			return err
		}

		pubk, _, addr, err := fl.GetAddressPubKeySECP256K1(p)
		if err != nil {
			return err
		}
		fmt.Println(addr)
		fmt.Println(pubk)

		a, err := address.NewFromString(addr)
		if err != nil {
			return err
		}

		var pd ledgerwallet.LedgerKeyInfo
		pd.Address = a
		pd.Path = p

		b, err := json.Marshal(pd)
		if err != nil {
			return err
		}

		var ki types.KeyInfo
		ki.Type = wallet.KTSecp256k1
		ki.PrivateKey = b

		out, err := json.Marshal(ki)
		if err != nil {
			return err
		}

		fmt.Println(string(out))

		return nil
	},
}

var ledgerSignTestCmd = &cli.Command{
	Name: "sign",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return cli.ShowCommandHelp(cctx, cctx.Command.Name)
		}

		fl, err := ledgerfil.FindLedgerFilecoinApp()
		if err != nil {
			return err
		}

		p, err := parseHDPath(cctx.Args().First())
		if err != nil {
			return err
		}

		addr, err := address.NewFromString("f1xc3hws5n6y5m3m44gzb3gyjzhups6wzmhe663ji")
		if err != nil {
			return err
		}

		m := &types.Message{
			To:   addr,
			From: addr,
		}

		b, err := m.ToStorageBlock()
		if err != nil {
			return err
		}

		sig, err := fl.SignSECP256K1(p, b.RawData())
		if err != nil {
			return err
		}

		fmt.Println(sig.SignatureBytes())

		return nil
	},
}
