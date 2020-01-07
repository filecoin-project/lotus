package main

import (
	"fmt"
	"os"
	"path/filepath"

	"encoding/json"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	badger "github.com/ipfs/go-ds-badger"
	logging "github.com/ipfs/go-log"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
)

var log = logging.Logger("lotus-seed")

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting seed")

	local := []*cli.Command{
		preSealCmd,
		aggregateManifestsCmd,
		aggregateSectorDirsCmd,
	}

	app := &cli.App{
		Name:    "lotus-seed",
		Usage:   "Seal sectors for genesis miner",
		Version: build.UserVersion,
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

var aggregateManifestsCmd = &cli.Command{
	Name:  "aggregate-manifests",
	Usage: "aggregate a set of preseal manifests into a single file",
	Action: func(cctx *cli.Context) error {
		var inputs []map[string]genesis.GenesisMiner
		for _, infi := range cctx.Args().Slice() {
			fi, err := os.Open(infi)
			if err != nil {
				return err
			}
			defer fi.Close()
			var val map[string]genesis.GenesisMiner
			if err := json.NewDecoder(fi).Decode(&val); err != nil {
				return err
			}

			inputs = append(inputs, val)
		}

		output := make(map[string]genesis.GenesisMiner)
		for _, in := range inputs {
			for maddr, val := range in {
				if gm, ok := output[maddr]; ok {
					output[maddr] = mergeGenMiners(gm, val)
				} else {
					output[maddr] = val
				}
			}
		}

		blob, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(blob))
		return nil
	},
}

var aggregateSectorDirsCmd = &cli.Command{
	Name:  "aggregate-sector-dirs",
	Usage: "aggregate a set of preseal manifests into a single file",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "miner",
			Usage: "Specify address of miner to aggregate sectorbuilders for",
		},
		&cli.StringFlag{
			Name:  "dest",
			Usage: "specify directory to create aggregate sector store in",
		},
		&cli.Uint64Flag{
			Name:  "sector-size",
			Usage: "specify size of sectors to aggregate",
			Value: 32 * 1024 * 1024 * 1024,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.String("miner") == "" {
			return fmt.Errorf("must specify miner address with --miner")
		}
		if cctx.String("dest") == "" {
			return fmt.Errorf("must specify dest directory with --dest")
		}

		maddr, err := address.NewFromString(cctx.String("miner"))
		if err != nil {
			return err
		}

		destdir, err := homedir.Expand(cctx.String("dest"))
		if err != nil {
			return err
		}

		if err := os.MkdirAll(destdir, 0755); err != nil {
			return err
		}

		agmds, err := badger.NewDatastore(filepath.Join(destdir, "badger"), nil)
		if err != nil {
			return err
		}
		defer agmds.Close()

		ssize := cctx.Uint64("sector-size")

		agsb, err := sectorbuilder.New(&sectorbuilder.Config{
			Miner:         maddr,
			SectorSize:    ssize,
			Dir:           destdir,
			WorkerThreads: 2,
		}, agmds)
		if err != nil {
			return err
		}

		var aggrGenMiner genesis.GenesisMiner
		var highestSectorID uint64
		for _, dir := range cctx.Args().Slice() {
			dir, err := homedir.Expand(dir)
			if err != nil {
				return xerrors.Errorf("failed to expand %q: %w", dir, err)
			}

			st, err := os.Stat(dir)
			if err != nil {
				return err
			}
			if !st.IsDir() {
				return fmt.Errorf("%q was not a directory", dir)
			}

			fi, err := os.Open(filepath.Join(dir, "pre-seal-"+maddr.String()+".json"))
			if err != nil {
				return err
			}

			var genmm map[string]genesis.GenesisMiner
			if err := json.NewDecoder(fi).Decode(&genmm); err != nil {
				return err
			}

			genm, ok := genmm[maddr.String()]
			if !ok {
				return xerrors.Errorf("input data did not have our miner in it (%s)", maddr)
			}

			if genm.SectorSize != ssize {
				return xerrors.Errorf("sector size mismatch in %q (%d != %d)", dir)
			}

			for _, s := range genm.Sectors {
				if s.SectorID > highestSectorID {
					highestSectorID = s.SectorID
				}
			}

			aggrGenMiner = mergeGenMiners(aggrGenMiner, genm)

			opts := badger.DefaultOptions
			opts.ReadOnly = true
			mds, err := badger.NewDatastore(filepath.Join(dir, "badger"), &opts)
			if err != nil {
				return err
			}
			defer mds.Close()

			sb, err := sectorbuilder.New(&sectorbuilder.Config{
				Miner:         maddr,
				SectorSize:    genm.SectorSize,
				Dir:           dir,
				WorkerThreads: 2,
			}, mds)
			if err != nil {
				return err
			}

			if err := agsb.ImportFrom(sb, false); err != nil {
				return xerrors.Errorf("importing sectors from %q failed: %w", dir, err)
			}
		}

		if err := agsb.SetLastSectorID(highestSectorID); err != nil {
			return err
		}

		if err := seed.WriteGenesisMiner(maddr, destdir, &aggrGenMiner); err != nil {
			return err
		}

		return nil
	},
}

func mergeGenMiners(a, b genesis.GenesisMiner) genesis.GenesisMiner {
	if a.SectorSize != b.SectorSize {
		panic("sector sizes mismatch")
	}

	return genesis.GenesisMiner{
		Owner:      a.Owner,
		Worker:     a.Worker,
		SectorSize: a.SectorSize,
		Key:        a.Key,
		Sectors:    append(a.Sectors, b.Sectors...),
	}
}
