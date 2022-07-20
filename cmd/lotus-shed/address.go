package main

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
)

var addressCmd = &cli.Command{
	Name:  "addr",
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
			return err
		}
		fmt.Printf("%s\n", a)
		return nil
	},
}
