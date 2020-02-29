package main

import (
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
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-sectorbuilder/fs"
	"github.com/filecoin-project/specs-actors/actors/abi"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
)

var log = logging.Logger("lotus-bench")

type BenchResults struct {
	SectorSize abi.SectorSize

	SealingResults []SealingResult

	PostGenerateCandidates time.Duration
	PostEProofCold         time.Duration
	PostEProofHot          time.Duration
	VerifyEPostCold        time.Duration
	VerifyEPostHot         time.Duration
}

type SealingResult struct {
	AddPiece   time.Duration
	PreCommit1 time.Duration
	PreCommit2 time.Duration
	Commit1    time.Duration
	Commit2    time.Duration
	Verify     time.Duration
	Unseal     time.Duration
}

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting lotus-bench")

	build.SectorSizes = append(build.SectorSizes, 2048)

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
				Value: "512MiB",
				Usage: "size of the sectors in bytes, i.e. 32GiB",
			},
			&cli.BoolFlag{
				Name:  "no-gpu",
				Usage: "disable gpu usage for the benchmark run",
			},
			&cli.StringFlag{
				Name:  "miner-addr",
				Usage: "pass miner address (only necessary if using existing sectorbuilder)",
				Value: "t01000",
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
			sectorSize := abi.SectorSize(sectorSizeInt)

			ppt, spt, err := lapi.ProofTypeFromSectorSize(sectorSize)
			if err != nil {
				return err
			}

			cfg := &sectorbuilder.Config{
				Miner:         maddr,
				SealProofType: spt,
				PoStProofType: ppt,
			}

			if robench == "" {
				if err := os.MkdirAll(sbdir, 0775); err != nil {
					return err
				}
			}

			if err := paramfetch.GetParams(build.ParametersJson(), uint64(sectorSize)); err != nil {
				return xerrors.Errorf("getting params: %w", err)
			}

			sbfs := &fs.Basic{
				Miner:  maddr,
				NextID: 1,
				Root:   sbdir,
			}

			sb, err := sectorbuilder.New(sbfs, cfg)
			if err != nil {
				return err
			}

			amid, err := address.IDFromAddress(maddr)
			if err != nil {
				return err
			}

			mid := abi.ActorID(amid)

			var sealTimings []SealingResult
			var sealedSectors []abi.SectorInfo
			numSectors := abi.SectorNumber(1)
			for i := abi.SectorNumber(1); i <= numSectors && robench == ""; i++ {
				start := time.Now()
				log.Info("Writing piece into sector...")

				r := rand.New(rand.NewSource(100 + int64(i)))

				pi, err := sb.AddPiece(context.TODO(), abi.PaddedPieceSize(sectorSize).Unpadded(), i, r, nil)
				if err != nil {
					return err
				}

				addpiece := time.Now()

				trand := sha256.Sum256([]byte(c.String("ticket-preimage")))
				ticket := abi.SealRandomness(trand[:])

				log.Info("Running replication(1)...")
				pieces := []abi.PieceInfo{pi}
				pc1o, err := sb.SealPreCommit1(context.TODO(), i, ticket, pieces)
				if err != nil {
					return xerrors.Errorf("commit: %w", err)
				}

				precommit1 := time.Now()

				log.Info("Running replication(2)...")
				commR, commD, err := sb.SealPreCommit2(context.TODO(), i, pc1o)
				if err != nil {
					return xerrors.Errorf("commit: %w", err)
				}

				precommit2 := time.Now()

				sealedSectors = append(sealedSectors, abi.SectorInfo{
					RegisteredProof: ppt,
					SectorNumber:    i,
					SealedCID:       commR,
				})

				seed := lapi.SealSeed{
					Epoch: 101,
					Value: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 255},
				}

				log.Info("Generating PoRep for sector (1)")
				c1o, err := sb.SealCommit1(context.TODO(), i, ticket, seed.Value, pieces, commR, commD)
				if err != nil {
					return err
				}

				sealcommit1 := time.Now()

				log.Info("Generating PoRep for sector (2)")

				proof, err := sb.SealCommit2(context.TODO(), i, c1o)
				if err != nil {
					return err
				}

				sealcommit2 := time.Now()

				svi := abi.SealVerifyInfo{
					SectorID: abi.SectorID{Miner: abi.ActorID(mid), Number: i},
					OnChain: abi.OnChainSealVerifyInfo{
						SealedCID:        commR,
						InteractiveEpoch: seed.Epoch,
						RegisteredProof:  ppt,
						Proof:            proof,
						DealIDs:          nil,
						SectorNumber:     i,
						SealRandEpoch:    0,
					},
					Randomness:            ticket,
					InteractiveRandomness: seed.Value,
					UnsealedCID:           commD,
				}

				ok, err := sectorbuilder.ProofVerifier.VerifySeal(svi)
				if err != nil {
					return err
				}
				if !ok {
					return xerrors.Errorf("porep proof for sector %d was invalid", i)
				}

				verifySeal := time.Now()

				if !c.Bool("skip-unseal") {
					log.Info("Unsealing sector")
					rc, err := sb.ReadPieceFromSealedSector(context.TODO(), 1, 0, abi.UnpaddedPieceSize(sectorSize), ticket, commD)
					if err != nil {
						return err
					}

					if err := rc.Close(); err != nil {
						return err
					}
				}
				unseal := time.Now()

				sealTimings = append(sealTimings, SealingResult{
					AddPiece:   addpiece.Sub(start),
					PreCommit1: precommit1.Sub(addpiece),
					PreCommit2: precommit2.Sub(precommit1),
					Commit1:    sealcommit1.Sub(precommit2),
					Commit2:    sealcommit2.Sub(sealcommit1),
					Verify:     verifySeal.Sub(sealcommit2),
					Unseal:     unseal.Sub(verifySeal),
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

				var genmm map[string]genesis.Miner
				if err := json.Unmarshal(fdata, &genmm); err != nil {
					return err
				}

				genm, ok := genmm[maddr.String()]
				if !ok {
					return xerrors.Errorf("preseal file didnt have expected miner in it")
				}

				for _, s := range genm.Sectors {
					sealedSectors = append(sealedSectors, abi.SectorInfo{
						SealedCID:    s.CommR,
						SectorNumber: s.SectorID,
					})
				}
			}

			log.Info("generating election post candidates")
			fcandidates, err := sb.GenerateEPostCandidates(sealedSectors, abi.PoStRandomness(challenge[:]), []abi.SectorNumber{})
			if err != nil {
				return err
			}

			var candidates []abi.PoStCandidate
			for _, c := range fcandidates {
				candidates = append(candidates, c.Candidate)
			}

			gencandidates := time.Now()

			log.Info("computing election post snark (cold)")
			proof1, err := sb.ComputeElectionPoSt(sealedSectors, challenge[:], candidates[:1])
			if err != nil {
				return err
			}

			epost1 := time.Now()

			log.Info("computing election post snark (hot)")
			proof2, err := sb.ComputeElectionPoSt(sealedSectors, challenge[:], candidates[:1])
			if err != nil {
				return err
			}

			epost2 := time.Now()

			ccount := sectorbuilder.ElectionPostChallengeCount(uint64(len(sealedSectors)), 0)

			pvi1 := abi.PoStVerifyInfo{
				Randomness:      abi.PoStRandomness(challenge[:]),
				Candidates:      candidates[:1],
				Proofs:          proof1,
				EligibleSectors: sealedSectors,
				Prover:          abi.ActorID(mid),
				ChallengeCount:  ccount,
			}
			ok, err := sectorbuilder.ProofVerifier.VerifyElectionPost(context.TODO(), pvi1)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}

			verifypost1 := time.Now()

			pvi2 := abi.PoStVerifyInfo{
				Randomness:      abi.PoStRandomness(challenge[:]),
				Candidates:      candidates[:1],
				Proofs:          proof2,
				EligibleSectors: sealedSectors,
				Prover:          abi.ActorID(mid),
				ChallengeCount:  ccount,
			}

			ok, err = sectorbuilder.ProofVerifier.VerifyElectionPost(context.TODO(), pvi2)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}
			verifypost2 := time.Now()

			bo := BenchResults{
				SectorSize:     sectorSize,
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
				fmt.Printf("----\nresults (v23) (%d)\n", sectorSize)
				if robench == "" {
					fmt.Printf("seal: addPiece: %s (%s)\n", bo.SealingResults[0].AddPiece, bps(bo.SectorSize, bo.SealingResults[0].AddPiece)) // TODO: average across multiple sealings
					fmt.Printf("seal: preCommit phase 1: %s (%s)\n", bo.SealingResults[0].PreCommit1, bps(bo.SectorSize, bo.SealingResults[0].PreCommit1))
					fmt.Printf("seal: preCommit phase 2: %s (%s)\n", bo.SealingResults[0].PreCommit2, bps(bo.SectorSize, bo.SealingResults[0].PreCommit2))
					fmt.Printf("seal: commit phase 1: %s (%s)\n", bo.SealingResults[0].Commit1, bps(bo.SectorSize, bo.SealingResults[0].Commit1))
					fmt.Printf("seal: commit phase 2: %s (%s)\n", bo.SealingResults[0].Commit2, bps(bo.SectorSize, bo.SealingResults[0].Commit2))
					fmt.Printf("seal: verify: %s\n", bo.SealingResults[0].Verify)
					if !c.Bool("skip-unseal") {
						fmt.Printf("unseal: %s  (%s)\n", bo.SealingResults[0].Unseal, bps(bo.SectorSize, bo.SealingResults[0].Unseal))
					}
				}
				fmt.Printf("generate candidates: %s (%s)\n", bo.PostGenerateCandidates, bps(bo.SectorSize*abi.SectorSize(len(bo.SealingResults)), bo.PostGenerateCandidates))
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

func bps(data abi.SectorSize, d time.Duration) string {
	bdata := new(big.Int).SetUint64(uint64(data))
	bdata = bdata.Mul(bdata, big.NewInt(time.Second.Nanoseconds()))
	bps := bdata.Div(bdata, big.NewInt(d.Nanoseconds()))
	return types.SizeStr(types.BigInt{bps}) + "/s"
}
