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
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-address"
	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/sector-storage/ffiwrapper/basicfs"
)

var log = logging.Logger("lotus-bench")

type BenchResults struct {
	SectorSize abi.SectorSize

	SealingResults []SealingResult

	PostGenerateCandidates time.Duration
	PostWinningProofCold   time.Duration
	PostWinningProofHot    time.Duration
	VerifyWinningPostCold  time.Duration
	VerifyWinningPostHot   time.Duration

	PostWindowProofCold  time.Duration
	PostWindowProofHot   time.Duration
	VerifyWindowPostCold time.Duration
	VerifyWindowPostHot  time.Duration
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

type Commit2In struct {
	SectorNum  int64
	Phase1Out  []byte
	SectorSize uint64
}

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting lotus-bench")

	build.SectorSizes = append(build.SectorSizes, 2048)

	app := &cli.App{
		Name:    "lotus-bench",
		Usage:   "Benchmark performance of lotus on your hardware",
		Version: build.UserVersion,
		Commands: []*cli.Command{
			proveCmd,
			sealBenchCmd,
			importBenchCmd,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		return
	}
}

var sealBenchCmd = &cli.Command{
	Name: "sealing",
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
			Usage: "pass a directory to run post timings on an existing sectorbuilder",
		},
		&cli.BoolFlag{
			Name:  "json-out",
			Usage: "output results in json format",
		},
		&cli.BoolFlag{
			Name:  "skip-commit2",
			Usage: "skip the commit2 (snark) portion of the benchmark",
		},
		&cli.BoolFlag{
			Name:  "skip-unseal",
			Usage: "skip the unseal portion of the benchmark",
		},
		&cli.StringFlag{
			Name:  "save-commit2-input",
			Usage: "Save commit2 input to a file",
		},
		&cli.IntFlag{
			Name:  "num-sectors",
			Value: 1,
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

		spt, err := ffiwrapper.SealProofTypeFromSectorSize(sectorSize)
		if err != nil {
			return err
		}

		cfg := &ffiwrapper.Config{
			SealProofType: spt,
		}

		if robench == "" {
			if err := os.MkdirAll(sbdir, 0775); err != nil {
				return err
			}
		}

		if !c.Bool("skip-commit2") {
			if err := paramfetch.GetParams(build.ParametersJson(), uint64(sectorSize)); err != nil {
				return xerrors.Errorf("getting params: %w", err)
			}
		}

		sbfs := &basicfs.Provider{
			Root: sbdir,
		}

		sb, err := ffiwrapper.New(sbfs, cfg)
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
		numSectors := abi.SectorNumber(c.Int("num-sectors"))
		for i := abi.SectorNumber(1); i <= numSectors && robench == ""; i++ {
			sid := abi.SectorID{
				Miner:  mid,
				Number: i,
			}

			start := time.Now()
			log.Info("Writing piece into sector...")

			r := rand.New(rand.NewSource(100 + int64(i)))

			pi, err := sb.AddPiece(context.TODO(), sid, nil, abi.PaddedPieceSize(sectorSize).Unpadded(), r)
			if err != nil {
				return err
			}

			addpiece := time.Now()

			trand := sha256.Sum256([]byte(c.String("ticket-preimage")))
			ticket := abi.SealRandomness(trand[:])

			log.Info("Running replication(1)...")
			pieces := []abi.PieceInfo{pi}
			pc1o, err := sb.SealPreCommit1(context.TODO(), sid, ticket, pieces)
			if err != nil {
				return xerrors.Errorf("commit: %w", err)
			}

			precommit1 := time.Now()

			log.Info("Running replication(2)...")
			cids, err := sb.SealPreCommit2(context.TODO(), sid, pc1o)
			if err != nil {
				return xerrors.Errorf("commit: %w", err)
			}

			precommit2 := time.Now()

			sealedSectors = append(sealedSectors, abi.SectorInfo{
				RegisteredProof: spt,
				SectorNumber:    i,
				SealedCID:       cids.Sealed,
			})

			seed := lapi.SealSeed{
				Epoch: 101,
				Value: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 255},
			}

			log.Info("Generating PoRep for sector (1)")
			c1o, err := sb.SealCommit1(context.TODO(), sid, ticket, seed.Value, pieces, cids)
			if err != nil {
				return err
			}

			sealcommit1 := time.Now()

			log.Info("Generating PoRep for sector (2)")

			if c.String("save-commit2-input") != "" {
				c2in := Commit2In{
					SectorNum:  int64(i),
					Phase1Out:  c1o,
					SectorSize: uint64(sectorSize),
				}

				b, err := json.Marshal(&c2in)
				if err != nil {
					return err
				}

				if err := ioutil.WriteFile(c.String("save-commit2-input"), b, 0664); err != nil {
					log.Warnf("%+v", err)
				}
			}

			var proof storage.Proof
			if !c.Bool("skip-commit2") {
				proof, err = sb.SealCommit2(context.TODO(), sid, c1o)
				if err != nil {
					return err
				}
			}

			sealcommit2 := time.Now()

			if !c.Bool("skip-commit2") {

				svi := abi.SealVerifyInfo{
					SectorID: abi.SectorID{Miner: mid, Number: i},
					OnChain: abi.OnChainSealVerifyInfo{
						SealedCID:        cids.Sealed,
						InteractiveEpoch: seed.Epoch,
						RegisteredProof:  spt,
						Proof:            proof,
						DealIDs:          nil,
						SectorNumber:     i,
						SealRandEpoch:    0,
					},
					Randomness:            ticket,
					InteractiveRandomness: seed.Value,
					UnsealedCID:           cids.Unsealed,
				}

				ok, err := ffiwrapper.ProofVerifier.VerifySeal(svi)
				if err != nil {
					return err
				}
				if !ok {
					return xerrors.Errorf("porep proof for sector %d was invalid", i)
				}
			}

			verifySeal := time.Now()

			if !c.Bool("skip-unseal") {
				log.Info("Unsealing sector")
				// TODO: RM unsealed sector first
				rc, err := sb.ReadPieceFromSealedSector(context.TODO(), abi.SectorID{Miner: mid, Number: 1}, 0, abi.UnpaddedPieceSize(sectorSize), ticket, cids.Unsealed)
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

		bo := BenchResults{
			SectorSize:     sectorSize,
			SealingResults: sealTimings,
		}

		if !c.Bool("skip-commit2") {
			log.Info("generating winning post candidates")
			fcandidates, err := ffiwrapper.ProofVerifier.GenerateWinningPoStSectorChallenge(context.TODO(), spt, mid, challenge[:], uint64(len(sealedSectors)))
			if err != nil {
				return err
			}

			candidates := make([]abi.SectorInfo, len(fcandidates))
			for i, fcandidate := range fcandidates {
				candidates[i] = sealedSectors[fcandidate]
			}

			gencandidates := time.Now()

			log.Info("computing winning post snark (cold)")
			proof1, err := sb.GenerateWinningPoSt(context.TODO(), mid, candidates, challenge[:])
			if err != nil {
				return err
			}

			winnnigpost1 := time.Now()

			log.Info("computing winning post snark (hot)")
			proof2, err := sb.GenerateWinningPoSt(context.TODO(), mid, candidates, challenge[:])
			if err != nil {
				return err
			}

			winnningpost2 := time.Now()

			pvi1 := abi.WinningPoStVerifyInfo{
				Randomness:        abi.PoStRandomness(challenge[:]),
				Proofs:            proof1,
				ChallengedSectors: candidates,
				Prover:            mid,
			}
			ok, err := ffiwrapper.ProofVerifier.VerifyWinningPoSt(context.TODO(), pvi1)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}

			verifyWinnnigPost1 := time.Now()

			pvi2 := abi.WinningPoStVerifyInfo{
				Randomness:        abi.PoStRandomness(challenge[:]),
				Proofs:            proof2,
				ChallengedSectors: candidates,
				Prover:            mid,
			}

			ok, err = ffiwrapper.ProofVerifier.VerifyWinningPoSt(context.TODO(), pvi2)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}
			verifyWinningPost2 := time.Now()

			log.Info("computing window post snark (cold)")
			wproof1, err := sb.GenerateWindowPoSt(context.TODO(), mid, sealedSectors, challenge[:])
			if err != nil {
				return err
			}

			windowpost1 := time.Now()

			log.Info("computing window post snark (hot)")
			wproof2, err := sb.GenerateWindowPoSt(context.TODO(), mid, sealedSectors, challenge[:])
			if err != nil {
				return err
			}

			windowpost2 := time.Now()

			wpvi1 := abi.WindowPoStVerifyInfo{
				Randomness:        challenge[:],
				Proofs:            wproof1,
				ChallengedSectors: sealedSectors,
				Prover:            mid,
			}
			ok, err = ffiwrapper.ProofVerifier.VerifyWindowPoSt(context.TODO(), wpvi1)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}

			verifyWindowpost1 := time.Now()

			wpvi2 := abi.WindowPoStVerifyInfo{
				Randomness:        challenge[:],
				Proofs:            wproof2,
				ChallengedSectors: sealedSectors,
				Prover:            mid,
			}
			ok, err = ffiwrapper.ProofVerifier.VerifyWindowPoSt(context.TODO(), wpvi2)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}

			verifyWindowpost2 := time.Now()

			bo.PostGenerateCandidates = gencandidates.Sub(beforePost)
			bo.PostWinningProofCold = winnnigpost1.Sub(gencandidates)
			bo.PostWinningProofHot = winnningpost2.Sub(winnnigpost1)
			bo.VerifyWinningPostCold = verifyWinnnigPost1.Sub(winnningpost2)
			bo.VerifyWinningPostHot = verifyWinningPost2.Sub(verifyWinnnigPost1)

			bo.PostWindowProofCold = windowpost1.Sub(verifyWinningPost2)
			bo.PostWindowProofHot = windowpost2.Sub(windowpost1)
			bo.VerifyWindowPostCold = verifyWindowpost1.Sub(windowpost2)
			bo.VerifyWindowPostHot = verifyWindowpost2.Sub(verifyWindowpost1)
		}

		if c.Bool("json-out") {
			data, err := json.MarshalIndent(bo, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(data))
		} else {
			fmt.Printf("----\nresults (v25) (%d)\n", sectorSize)
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
				fmt.Println("")
			}
			if !c.Bool("skip-commit2") {
				fmt.Printf("generate candidates: %s (%s)\n", bo.PostGenerateCandidates, bps(bo.SectorSize*abi.SectorSize(len(bo.SealingResults)), bo.PostGenerateCandidates))
				fmt.Printf("compute winnnig post proof (cold): %s\n", bo.PostWinningProofCold)
				fmt.Printf("compute winnnig post proof (hot): %s\n", bo.PostWinningProofHot)
				fmt.Printf("verify winnnig post proof (cold): %s\n", bo.VerifyWinningPostCold)
				fmt.Printf("verify winnnig post proof (hot): %s\n\n", bo.VerifyWinningPostHot)

				fmt.Printf("compute window post proof (cold): %s\n", bo.PostWindowProofCold)
				fmt.Printf("compute window post proof (hot): %s\n", bo.PostWindowProofHot)
				fmt.Printf("verify window post proof (cold): %s\n", bo.VerifyWindowPostCold)
				fmt.Printf("verify window post proof (hot): %s\n", bo.VerifyWindowPostHot)
			}
		}
		return nil
	},
}

var proveCmd = &cli.Command{
	Name:  "prove",
	Usage: "Benchmark a proof computation",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-gpu",
			Usage: "disable gpu usage for the benchmark run",
		},
	},
	Action: func(c *cli.Context) error {
		if c.Bool("no-gpu") {
			os.Setenv("BELLMAN_NO_GPU", "1")
		}

		if !c.Args().Present() {
			return xerrors.Errorf("Usage: lotus-bench prove [input.json]")
		}

		inb, err := ioutil.ReadFile(c.Args().First())
		if err != nil {
			return xerrors.Errorf("reading input file: %w", err)
		}

		var c2in Commit2In
		if err := json.Unmarshal(inb, &c2in); err != nil {
			return xerrors.Errorf("unmarshalling input file: %w", err)
		}

		if err := paramfetch.GetParams(build.ParametersJson(), c2in.SectorSize); err != nil {
			return xerrors.Errorf("getting params: %w", err)
		}

		maddr, err := address.NewFromString(c.String("miner-addr"))
		if err != nil {
			return err
		}
		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}

		spt, err := ffiwrapper.SealProofTypeFromSectorSize(abi.SectorSize(c2in.SectorSize))
		if err != nil {
			return err
		}

		cfg := &ffiwrapper.Config{
			SealProofType: spt,
		}

		sb, err := ffiwrapper.New(nil, cfg)
		if err != nil {
			return err
		}

		start := time.Now()

		proof, err := sb.SealCommit2(context.TODO(), abi.SectorID{Miner: abi.ActorID(mid), Number: abi.SectorNumber(c2in.SectorNum)}, c2in.Phase1Out)
		if err != nil {
			return err
		}

		sealCommit2 := time.Now()

		fmt.Printf("proof: %x\n", proof)

		fmt.Printf("----\nresults (v23) (%d)\n", c2in.SectorSize)
		dur := sealCommit2.Sub(start)

		fmt.Printf("seal: commit phase 2: %s (%s)\n", dur, bps(abi.SectorSize(c2in.SectorSize), dur))
		return nil
	},
}

func bps(data abi.SectorSize, d time.Duration) string {
	bdata := new(big.Int).SetUint64(uint64(data))
	bdata = bdata.Mul(bdata, big.NewInt(time.Second.Nanoseconds()))
	bps := bdata.Div(bdata, big.NewInt(d.Nanoseconds()))
	return types.SizeStr(types.BigInt{bps}) + "/s"
}
