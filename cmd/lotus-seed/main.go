package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"

	badger "github.com/ipfs/go-ds-badger"
	logging "github.com/ipfs/go-log"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
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
			Value: 1024,
			Usage: "specify size of sectors to pre-seal",
		},
		&cli.StringFlag{
			Name:  "ticket-preimage",
			Value: "lotus is fire",
			Usage: "set the ticket preimage for sealing randomness",
		},
		&cli.Uint64Flag{
			Name:  "num-sectors",
			Value: 1,
			Usage: "select number of sectors to pre-seal",
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

		cfg := &sectorbuilder.Config{
			Miner:         maddr,
			SectorSize:    c.Uint64("sector-size"),
			CacheDir:      filepath.Join(sbroot, "cache"),
			SealedDir:     filepath.Join(sbroot, "sealed"),
			StagedDir:     filepath.Join(sbroot, "staging"),
			MetadataDir:   filepath.Join(sbroot, "meta"),
			WorkerThreads: 2,
		}

		for _, d := range []string{cfg.CacheDir, cfg.SealedDir, cfg.StagedDir, cfg.MetadataDir} {
			if err := os.MkdirAll(d, 0775); err != nil {
				return err
			}
		}

		mds, err := badger.NewDatastore(filepath.Join(sbroot, "badger"), nil)
		if err != nil {
			return err
		}

		if err := build.GetParams(true, false); err != nil {
			return xerrors.Errorf("getting params: %w", err)
		}

		sb, err := sectorbuilder.New(cfg, mds)
		if err != nil {
			return err
		}

		r := rand.New(rand.NewSource(101))
		size := sectorbuilder.UserBytesForSectorSize(c.Uint64("sector-size"))

		var sealedSectors []genesis.PreSeal
		for i := uint64(1); i <= c.Uint64("num-sectors"); i++ {
			sid, err := sb.AcquireSectorId()
			if err != nil {
				return err
			}

			pi, err := sb.AddPiece(size, sid, r, nil)
			if err != nil {
				return err
			}

			trand := sha256.Sum256([]byte(c.String("ticket-preimage")))
			ticket := sectorbuilder.SealTicket{
				TicketBytes: trand,
			}

			fmt.Println("Piece info: ", pi)

			pco, err := sb.SealPreCommit(sid, ticket, []sectorbuilder.PublicPieceInfo{pi})
			if err != nil {
				return xerrors.Errorf("commit: %w", err)
			}

			sealedSectors = append(sealedSectors, genesis.PreSeal{
				CommR:    pco.CommR,
				CommD:    pco.CommD,
				SectorID: sid,
			})
		}

		output := map[string]genesis.GenesisMiner{
			maddr.String(): genesis.GenesisMiner{
				Sectors: sealedSectors,
			},
		}

		out, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return err
		}

		if err := ioutil.WriteFile("pre-seal-"+maddr.String()+".json", out, 0664); err != nil {
			return err
		}

		if err := mds.Close(); err != nil {
			return xerrors.Errorf("closing datastore: %w", err)
		}

		return nil
	},
}
