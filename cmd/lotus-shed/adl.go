package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	adt13 "github.com/filecoin-project/go-state-types/builtin/v13/util/adt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var adlCmd = &cli.Command{
	Name:  "adl",
	Usage: "adl manipulation commands",
	Subcommands: []*cli.Command{
		adlAmtCmd,
	},
}

var adlAmtCmd = &cli.Command{
	Name:  "amt",
	Usage: "AMT manipulation commands",
	Subcommands: []*cli.Command{
		adlAmtGetCmd,
	},
}

var adlAmtGetCmd = &cli.Command{
	Name:  "get",
	Usage: "Get an element from an AMT",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "car-file",
			Usage: "write a car file with two hamts (use lotus-shed export-car)",
		},
		&cli.IntFlag{
			Name:  "bitwidth",
			Usage: "bitwidth of the HAMT",
			Value: 5,
		},
		&cli.StringFlag{
			Name:  "root",
			Usage: "root cid of the HAMT",
		},
		&cli.Int64Flag{
			Name:  "key",
			Usage: "key to get",
		},
	},
	Action: func(cctx *cli.Context) error {
		bs := blockstore.NewMemorySync()

		f, err := os.Open(cctx.String("car-file"))
		if err != nil {
			return err
		}
		defer func(f *os.File) {
			_ = f.Close()
		}(f)

		cr, err := car.NewCarReader(f)
		if err != nil {
			return err
		}

		for {
			blk, err := cr.Next()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			if err := bs.Put(cctx.Context, blk); err != nil {
				return err
			}
		}

		root, err := cid.Parse(cctx.String("root"))
		if err != nil {
			return err
		}

		m, err := adt13.AsArray(adt.WrapStore(cctx.Context, cbor.NewCborStore(bs)), root, cctx.Int("bitwidth"))
		if err != nil {
			return err
		}

		var out cbg.Deferred
		ok, err := m.Get(cctx.Uint64("key"), &out)
		if err != nil {
			return err
		}
		if !ok {
			return xerrors.Errorf("no such element")
		}

		fmt.Printf("RAW: %x\n", out.Raw)
		fmt.Println("----")

		var i interface{}
		if err := cbor.DecodeInto(out.Raw, &i); err == nil {
			ij, err := json.MarshalIndent(i, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(ij))
		}

		return nil
	},
}
