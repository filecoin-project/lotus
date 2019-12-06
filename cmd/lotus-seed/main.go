package main

import (
	"os"

	logging "github.com/ipfs/go-log"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
)

var log = logging.Logger("lotus-seed")

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting seed")

	local := []*cli.Command{
		preSealCmd,
	}

	app := &cli.App{
		Name:    "lotus-seed",
		Usage:   "Seal sectors for genesis miner",
		Version: build.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "sectorbuilder-dir",
				Value: "~/.genesis-sectors",
			},
		},

		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		log.Warn(err)
		return
	}
}

var preSealCmd = &cli.Command{
	Name: "pre-seal",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "miner-addr",
			Value: "t0101",
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
			Name:  "num-sectors",
			Value: 1,
			Usage: "select number of sectors to pre-seal",
		},
		&cli.Uint64Flag{
			Name:  "sector-offset",
			Value: 0,
			Usage: "how many sector ids to skip when starting to seal",
		},
	},
	Action: func(c *cli.Context) error {
		sdir := c.String("sectorbuilder-dir")
		sbroot, err := homedir.Expand(sdir)
		if err != nil {
			return err
		}

		maddr, err := address.NewFromString(c.String("miner-addr"))
		if err != nil {
			return err
		}

		gm, err := seed.PreSeal(maddr, c.Uint64("sector-size"), c.Uint64("sector-offset"), c.Int("num-sectors"), sbroot, []byte(c.String("ticket-preimage")))
		if err != nil {
			return err
		}

		return seed.WriteGenesisMiner(maddr, sbroot, gm)
	},
}
