package main

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

var log = logging.Logger("lotus-bench")

type BenchResults struct {
	SectorSize uint64

	SealingResults []SealingResult

	PostGenerateCandidates time.Duration
	PostEProof             time.Duration
}

type SealingResult struct {
	AddPiece  time.Duration
	PreCommit time.Duration
	Commit    time.Duration
}

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting lotus-bench")

	app := &cli.App{
		Name:    "lotus-bench",
		Usage:   "Benchmark performance of lotus on your hardware",
		Version: build.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "storage-dir",
				Value: "~/.lotus-bench",
				Usage: "Path to the storage directory that will store sectors long term",
			},
			&cli.Uint64Flag{
				Name:  "sector-size",
				Value: 1024,
			},
		},
		Action: func(c *cli.Context) error {
			sdir, err := homedir.Expand(c.String("storage-dir"))
			if err != nil {
				return err
			}

			os.MkdirAll(sdir, 0775)

			tsdir, err := ioutil.TempDir(sdir, "bench")
			if err != nil {
				return err
			}
			defer func() {
				if err := os.RemoveAll(tsdir); err != nil {
					log.Warn("remove all: ", err)
				}
			}()

			maddr, err := address.NewFromString("t0101")
			if err != nil {
				return err
			}

			mds := datastore.NewMapDatastore()
			cfg := &sectorbuilder.Config{
				Miner:         maddr,
				SectorSize:    c.Uint64("sector-size"),
				WorkerThreads: 2,
				CacheDir:      filepath.Join(tsdir, "cache"),
				SealedDir:     filepath.Join(tsdir, "sealed"),
				StagedDir:     filepath.Join(tsdir, "staged"),
				MetadataDir:   filepath.Join(tsdir, "meta"),
			}
			for _, d := range []string{cfg.CacheDir, cfg.SealedDir, cfg.StagedDir, cfg.MetadataDir} {
				if err := os.MkdirAll(d, 0775); err != nil {
					return err
				}
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

			var sealTimings []SealingResult
			var sealedSectors []sectorbuilder.PublicSectorInfo
			numSectors := uint64(1)
			for i := uint64(1); i <= numSectors; i++ {
				start := time.Now()
				log.Info("Writing piece into sector...")
				pi, err := sb.AddPiece(size, i, r, nil)
				if err != nil {
					return err
				}

				addpiece := time.Now()

				trand := sha256.Sum256([]byte(c.String("ticket-preimage")))
				ticket := sectorbuilder.SealTicket{
					TicketBytes: trand,
				}

				log.Info("Running replication...")
				pieces := []sectorbuilder.PublicPieceInfo{pi}
				pco, err := sb.SealPreCommit(i, ticket, pieces)
				if err != nil {
					return xerrors.Errorf("commit: %w", err)
				}

				precommit := time.Now()

				sealedSectors = append(sealedSectors, sectorbuilder.PublicSectorInfo{
					CommR:    pco.CommR,
					SectorID: i,
				})

				seed := sectorbuilder.SealSeed{
					BlockHeight: 101,
					TicketBytes: [32]byte{1, 2, 3, 4, 5},
				}

				log.Info("Generating PoRep for sector")
				proof, err := sb.SealCommit(i, ticket, seed, pieces, pco)
				if err != nil {
					return err
				}
				_ = proof // todo verify

				sealcommit := time.Now()

				sealTimings = append(sealTimings, SealingResult{
					AddPiece:  addpiece.Sub(start),
					PreCommit: precommit.Sub(addpiece),
					Commit:    sealcommit.Sub(precommit),
				})
			}

			beforePost := time.Now()

			var challenge [32]byte
			rand.Read(challenge[:])

			log.Info("generating election post candidates")
			sinfos := sectorbuilder.NewSortedPublicSectorInfo(sealedSectors)
			candidates, err := sb.GenerateEPostCandidates(sinfos, challenge, []uint64{})
			if err != nil {
				return err
			}

			gencandidates := time.Now()

			log.Info("computing election post snark")
			proof, err := sb.ComputeElectionPoSt(sinfos, challenge[:], candidates[:1])
			if err != nil {
				return err
			}
			_ = proof // todo verify

			epost := time.Now()

			benchout := BenchResults{
				SectorSize:     cfg.SectorSize,
				SealingResults: sealTimings,

				PostGenerateCandidates: gencandidates.Sub(beforePost),
				PostEProof:             epost.Sub(gencandidates),
			} // TODO: optionally write this as json to a file

			fmt.Println("results")
			fmt.Printf("seal: addPiece: %s\n", benchout.SealingResults[0].AddPiece) // TODO: average across multiple sealings
			fmt.Printf("seal: preCommit: %s\n", benchout.SealingResults[0].PreCommit)
			fmt.Printf("seal: Commit: %s\n", benchout.SealingResults[0].Commit)
			fmt.Printf("generate candidates: %s\n", benchout.PostGenerateCandidates)
			fmt.Printf("compute epost proof: %s\n", benchout.PostEProof)
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Warn(err)
		return
	}
}
