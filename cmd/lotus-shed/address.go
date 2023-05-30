package main

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
)

var addressCmd = &cli.Command{
	Name:  "addr",
	Usage: "interact with filecoin addresses",
	Subcommands: cli.Commands{
		addrDecode,
		addrEncode,
	},
}

var addrDecode = &cli.Command{
	Name:  "decode",
	Usage: "decode hex bytes into address",
	Action: func(cctx *cli.Context) error {
		addrHex := cctx.Args().First()
		bs, err := hex.DecodeString(addrHex)
		if err != nil {
			return err
		}
		// first try cbor
		var a address.Address
		err = a.UnmarshalCBOR((bytes.NewReader(bs)))
		if err != nil {
			fmt.Printf("failed to unmarshal as CBOR, trying raw\n")
		} else {
			fmt.Printf("%s\n", a)
			return nil
		}

		// next try raw payload
		a, err = address.NewFromBytes(bs)
		if err != nil {
			return xerrors.New("could not decode as CBOR or raw payload, failing")
		}
		fmt.Printf("%s\n", a)
		return nil
	},
}

var addrEncode = &cli.Command{
	Name:  "encode",
	Usage: "encode address to hex bytes",
	Action: func(cctx *cli.Context) error {
		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		fmt.Printf("%x\n", addr.Bytes())
		return nil
	},
}
