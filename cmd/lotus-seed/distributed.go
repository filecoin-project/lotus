package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
)

var runCmd = &cli.Command{
	Name: "run",
	Description: "run preseal batch",
	ArgsUsage: "[batch json]",
	Action: func(c *cli.Context) error {
		sdir := c.String("sectorbuilder-dir")
		sbroot, err := homedir.Expand(sdir)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(sbroot, 0755); err != nil {
			return xerrors.Errorf("creating sbroot: %w", err)
		}

		m, err := ioutil.ReadFile(c.Args().First())
		if err != nil {
			return xerrors.Errorf("reading task file: %w", err)
		}

		var task seed.SealBatch
		if err := json.Unmarshal(m, &task); err != nil {
			return xerrors.Errorf("unmarshaling task: %w", err)
		}

		out, err := seed.RunTask(sbroot, task)
		if err != nil {
			return err
		}

		return seed.WriteJson(filepath.Join(sbroot, "seal-info.json"), out)
	},
}

var prepCmd = &cli.Command{
	Name: "prep",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "specify the future address of your miner",
		},
		&cli.Uint64Flag{
			Name:  "sector-size",
			Value: build.SectorSizes[0],
			Usage: "specify size of sectors to pre-seal",
		},
		&cli.StringFlag{
			Name:  "ticket-preimage",
			Value: "lotus is fire",
			Usage: "set the ticket preimage for sealing randomness",
		},
		&cli.IntFlag{
			Name:  "sectors",
			Value: 1,
			Usage: "select number of sectors to pre-seal",
		},
		&cli.IntFlag{
			Name:  "partitions",
			Value: 1,
			Usage: "number of tasks",
		},
	},
	Action: func(c *cli.Context) error {
		sdir := c.String("sectorbuilder-dir")
		sbroot, err := homedir.Expand(sdir)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(sbroot, 0755); err != nil {
			return xerrors.Errorf("creating sbroot: %w", err)
		}

		if c.String("miner-addr") == "" {
			fmt.Println("--miner-addr not specified (Check usage!) (t0101 for localnets)")
		}

		maddr, err := address.NewFromString(c.String("miner-addr"))
		if err != nil {
			return err
		}

		meta, tasks, err := seed.Prepare(sbroot,
			maddr,
			c.Uint64("sector-size"),
			c.Int("sectors"),
			c.Int("partitions"),
			[]byte(c.String("ticket-preimage")))
		if err != nil {
			return err
		}

		if err := seed.WritePrepInfo(sbroot, meta, tasks); err != nil {
			return xerrors.Errorf("writing tasks: %w", err)
		}

		fmt.Println("")
		fmt.Printf("Now ditribute %s/batch-[...]-%s.json files to workers\n", sbroot, meta.MinerAddr)
		fmt.Printf("and run `lotus-seed run batch-[...]-%s.json`\n", meta.MinerAddr)

		return nil
	},
}
