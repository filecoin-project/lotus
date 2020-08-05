package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/lotus"
)

var execLotusFlags struct {
	file string
}

var execLotusCmd = &cli.Command{
	Name:        "exec-lotus",
	Description: "execute a test vector against Lotus",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "file",
			Usage:       "input file",
			Required:    true,
			Destination: &execLotusFlags.file,
		},
	},
	Action: runExecLotus,
}

func runExecLotus(_ *cli.Context) error {
	if execLotusFlags.file == "" {
		return fmt.Errorf("test vector file cannot be empty")
	}

	file, err := os.Open(execLotusFlags.file)
	if err != nil {
		return fmt.Errorf("failed to open test vector: %w", err)
	}

	var (
		dec = json.NewDecoder(file)
		tv  TestVector
	)

	if err = dec.Decode(&tv); err != nil {
		return fmt.Errorf("failed to decode test vector: %w", err)
	}

	switch tv.Class {
	case "message":
		var (
			ctx     = context.Background()
			epoch   = tv.Pre.Epoch
			preroot = tv.Pre.StateTree.RootCID
		)

		bs := blockstore.NewTemporary()

		buf := bytes.NewReader(tv.CAR)
		gr, err := gzip.NewReader(buf)
		if err != nil {
			return err
		}
		defer gr.Close()

		header, err := car.LoadCar(bs, gr)
		if err != nil {
			return fmt.Errorf("failed to load state tree car from test vector: %w", err)
		}

		fmt.Println("roots: ", header.Roots)

		driver := lotus.NewDriver(ctx)

		for i, m := range tv.ApplyMessages {
			fmt.Printf("decoding message %v\n", i)
			msg, err := types.DecodeMessage(m)
			if err != nil {
				return err
			}

			fmt.Printf("executing message %v\n", i)
			var applyRet *vm.ApplyRet
			applyRet, preroot, err = driver.ExecuteMessage(msg, preroot, bs, epoch)
			if err != nil {
				return err
			}
			spew.Dump(applyRet)
		}

		return nil

	default:
		return fmt.Errorf("test vector class not supported")
	}
}
