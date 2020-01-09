package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/go-units"
	ffi "github.com/filecoin-project/filecoin-ffi"
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
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

	build.SectorSizes = append(build.SectorSizes, 1024)

	app := &cli.App{
		Name:    "lotus-bench",
		Usage:   "Benchmark performance of lotus on your hardware",
		Version: build.UserVersion,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "storage-dir",
				Value: "~/.lotus-bench",
				Usage: "Path to the storage directory that will store sectors long term",
			},
			&cli.StringFlag{
				Name:  "sector-size",
				Value: "1GiB",
				Usage: "size of the sectors in bytes, i.e. 32GiB",
			},
			&cli.BoolFlag{
				Name:  "no-gpu",
				Usage: "disable gpu usage for the benchmark run",
			},
			&cli.StringFlag{
				Name:  "miner-addr",
				Usage: "pass miner address (only necessary if using existing sectorbuilder)",
				Value: "t0101",
			},
			&cli.StringFlag{
				Name:  "benchmark-existing-sectorbuilder",
				Usage: "pass a directory to run election-post timings on an existing sectorbuilder",
			},
			&cli.BoolFlag{
				Name:  "json-out",
				Usage: "output results in json format",
			},
			&cli.BoolFlag{
				Name:  "skip-unseal",
				Usage: "skip the unseal portion of the benchmark",
			},
		},
		Action: func(c *cli.Context) error {
			if c.Bool("no-gpu") {
				os.Setenv("BELLMAN_NO_GPU", "1")
			}

			robench := c.String("benchmark-existing-sectorbuilder")

			var sbdir string

			if robench == "" {
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
				sbdir = tsdir
			} else {
				exp, err := homedir.Expand(robench)
				if err != nil {
					return err
				}
				sbdir = exp
			}

			maddr, err := address.NewFromString(c.String("miner-addr"))
			if err != nil {
				return err
			}

			sectorSizeInt, err := units.RAMInBytes(c.String("sector-size"))
			if err != nil {
				return err
			}
			sectorSize := uint64(sectorSizeInt)

			mds := datastore.NewMapDatastore()
			cfg := &sectorbuilder.Config{
				Miner:         maddr,
				SectorSize:    sectorSize,
				WorkerThreads: 2,
				Dir:           sbdir,
			}

			if robench == "" {
				if err := os.MkdirAll(sbdir, 0775); err != nil {
					return err
				}
			}

			if err := paramfetch.GetParams(build.ParametersJson, sectorSize); err != nil {
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
			for i := uint64(1); i <= numSectors && robench == ""; i++ {
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

				if !c.Bool("skip-unseal") {
					log.Info("Unsealing sector")
					rc, err := sb.ReadPieceFromSealedSector(1, 0, dataSize, ticket.TicketBytes[:], commD[:])
					if err != nil {
						return err
					}

					if err := rc.Close(); err != nil {
						return err
					}
				}
				unseal := time.Now()

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

			if robench != "" {
				// TODO: this assumes we only ever benchmark a preseal
				// sectorbuilder directory... we need a better way to handle
				// this in other cases

				fdata, err := ioutil.ReadFile(filepath.Join(sbdir, "pre-seal-"+maddr.String()+".json"))
				if err != nil {
					return err
				}

				var genmm map[string]genesis.GenesisMiner
				if err := json.Unmarshal(fdata, &genmm); err != nil {
					return err
				}

				genm, ok := genmm[maddr.String()]
				if !ok {
					return xerrors.Errorf("preseal file didnt have expected miner in it")
				}

				for _, s := range genm.Sectors {
					sealedSectors = append(sealedSectors, ffi.PublicSectorInfo{
						CommR:    s.CommR,
						SectorID: s.SectorID,
					})
				}
			}

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

			bo := BenchResults{
				SectorSize:     cfg.SectorSize,
				SealingResults: sealTimings,

				PostGenerateCandidates: gencandidates.Sub(beforePost),
				PostEProofCold:         epost1.Sub(gencandidates),
				PostEProofHot:          epost2.Sub(epost1),
				VerifyEPostCold:        verifypost1.Sub(epost2),
				VerifyEPostHot:         verifypost2.Sub(verifypost1),
			} // TODO: optionally write this as json to a file

			if c.Bool("json-out") {
				data, err := json.MarshalIndent(bo, "", "  ")
				if err != nil {
					return err
				}

				fmt.Println(string(data))
			} else {
				fmt.Printf("results (%d)\n", sectorSize)
				if robench == "" {
					fmt.Printf("seal: addPiece: %s (%s)\n", bo.SealingResults[0].AddPiece, bps(bo.SectorSize, bo.SealingResults[0].AddPiece)) // TODO: average across multiple sealings
					fmt.Printf("seal: preCommit: %s (%s)\n", bo.SealingResults[0].PreCommit, bps(bo.SectorSize, bo.SealingResults[0].PreCommit))
					fmt.Printf("seal: commit: %s (%s)\n", bo.SealingResults[0].Commit, bps(bo.SectorSize, bo.SealingResults[0].Commit))
					fmt.Printf("seal: verify: %s\n", bo.SealingResults[0].Verify)
					fmt.Printf("unseal: %s  (%s)\n", bo.SealingResults[0].Unseal, bps(bo.SectorSize, bo.SealingResults[0].Unseal))
				}
				fmt.Printf("generate candidates: %s (%s)\n", bo.PostGenerateCandidates, bps(bo.SectorSize*uint64(len(bo.SealingResults)), bo.PostGenerateCandidates))
				fmt.Printf("compute epost proof (cold): %s\n", bo.PostEProofCold)
				fmt.Printf("compute epost proof (hot): %s\n", bo.PostEProofHot)
				fmt.Printf("verify epost proof (cold): %s\n", bo.VerifyEPostCold)
				fmt.Printf("verify epost proof (hot): %s\n", bo.VerifyEPostHot)
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Warn(err)
		return
	}
}

func bps(data uint64, d time.Duration) string {
	bdata := new(big.Int).SetUint64(data)
	bdata = bdata.Mul(bdata, big.NewInt(time.Second.Nanoseconds()))
	bps := bdata.Div(bdata, big.NewInt(d.Nanoseconds()))
	return (types.BigInt{bps}).SizeStr() + "/s"
}
