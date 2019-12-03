package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	ffi "github.com/filecoin-project/filecoin-ffi"
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
	PostEProofCold         time.Duration
	PostEProofHot          time.Duration
	VerifyEPostCold        time.Duration
	VerifyEPostHot         time.Duration
}

type SealingResult struct {
	AddPiece  time.Duration
	PreCommit time.Duration
	Commit    time.Duration
	Verify    time.Duration
	Unseal    time.Duration
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
			&cli.BoolFlag{
				Name:  "no-gpu",
				Usage: "disable gpu usage for the benchmark run",
			},
		},
		Action: func(c *cli.Context) error {
			if c.Bool("no-gpu") {
				os.Setenv("BELLMAN_NO_GPU", "1")
			}
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

			sectorSize := c.Uint64("sector-size")

			mds := datastore.NewMapDatastore()
			cfg := &sectorbuilder.Config{
				Miner:         maddr,
				SectorSize:    sectorSize,
				WorkerThreads: 2,
				CacheDir:      filepath.Join(tsdir, "cache"),
				SealedDir:     filepath.Join(tsdir, "sealed"),
				StagedDir:     filepath.Join(tsdir, "staged"),
				UnsealedDir:   filepath.Join(tsdir, "unsealed"),
			}
			for _, d := range []string{cfg.CacheDir, cfg.SealedDir, cfg.StagedDir, cfg.UnsealedDir} {
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

			dataSize := sectorbuilder.UserBytesForSectorSize(sectorSize)

			var sealTimings []SealingResult
			var sealedSectors []ffi.PublicSectorInfo
			numSectors := uint64(1)
			for i := uint64(1); i <= numSectors; i++ {
				start := time.Now()
				log.Info("Writing piece into sector...")

				r := rand.New(rand.NewSource(100 + int64(i)))

				pi, err := sb.AddPiece(dataSize, i, r, nil)
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

				sealedSectors = append(sealedSectors, ffi.PublicSectorInfo{
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

				sealcommit := time.Now()
				commD := pi.CommP
				ok, err := sectorbuilder.VerifySeal(sectorSize, pco.CommR[:], commD[:], maddr, ticket.TicketBytes[:], seed.TicketBytes[:], i, proof)
				if err != nil {
					return err
				}
				if !ok {
					return xerrors.Errorf("porep proof for sector %d was invalid", i)
				}

				verifySeal := time.Now()

				log.Info("Unsealing sector")
				rc, err := sb.ReadPieceFromSealedSector(1, 0, dataSize, ticket.TicketBytes[:], commD[:])
				if err != nil {
					return err
				}

				unseal := time.Now()

				if err := rc.Close(); err != nil {
					return err
				}

				sealTimings = append(sealTimings, SealingResult{
					AddPiece:  addpiece.Sub(start),
					PreCommit: precommit.Sub(addpiece),
					Commit:    sealcommit.Sub(precommit),
					Verify:    verifySeal.Sub(sealcommit),
					Unseal:    unseal.Sub(verifySeal),
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

			log.Info("computing election post snark (cold)")
			proof1, err := sb.ComputeElectionPoSt(sinfos, challenge[:], candidates[:1])
			if err != nil {
				return err
			}

			epost1 := time.Now()

			log.Info("computing election post snark (hot)")
			proof2, err := sb.ComputeElectionPoSt(sinfos, challenge[:], candidates[:1])
			if err != nil {
				return err
			}

			epost2 := time.Now()

			if !bytes.Equal(proof1, proof2) {
				log.Warn("separate epost calls returned different proof values (this might be bad)")
			}

			ok, err := sectorbuilder.VerifyElectionPost(context.TODO(), sectorSize, sinfos, challenge[:], proof1, candidates[:1], maddr)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}

			verifypost1 := time.Now()

			ok, err = sectorbuilder.VerifyElectionPost(context.TODO(), sectorSize, sinfos, challenge[:], proof2, candidates[:1], maddr)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}
			verifypost2 := time.Now()

			benchout := BenchResults{
				SectorSize:     cfg.SectorSize,
				SealingResults: sealTimings,

				PostGenerateCandidates: gencandidates.Sub(beforePost),
				PostEProofCold:         epost1.Sub(gencandidates),
				PostEProofHot:          epost2.Sub(epost1),
				VerifyEPostCold:        verifypost1.Sub(epost2),
				VerifyEPostHot:         verifypost2.Sub(verifypost1),
			} // TODO: optionally write this as json to a file

			fmt.Println("results")
			fmt.Printf("seal: addPiece: %s\n", benchout.SealingResults[0].AddPiece) // TODO: average across multiple sealings
			fmt.Printf("seal: preCommit: %s\n", benchout.SealingResults[0].PreCommit)
			fmt.Printf("seal: Commit: %s\n", benchout.SealingResults[0].Commit)
			fmt.Printf("seal: Verify: %s\n", benchout.SealingResults[0].Verify)
			fmt.Printf("unseal: %s\n", benchout.SealingResults[0].Unseal)
			fmt.Printf("generate candidates: %s\n", benchout.PostGenerateCandidates)
			fmt.Printf("compute epost proof (cold): %s\n", benchout.PostEProofCold)
			fmt.Printf("compute epost proof (hot): %s\n", benchout.PostEProofHot)
			fmt.Printf("verify epost proof (cold): %s\n", benchout.VerifyEPostCold)
			fmt.Printf("verify epost proof (hot): %s\n", benchout.VerifyEPostHot)
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Warn(err)
		return
	}
}
