package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/go-units"
	logging "github.com/ipfs/go-log/v2"
	"github.com/minio/blake2b-simd"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	verifierffi "github.com/filecoin-project/lotus/chain/verifier/ffi"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("lotus-bench")

type BenchResults struct {
	EnvVar map[string]string

	SectorSize   abi.SectorSize
	SectorNumber int

	SealingSum     SealingResult
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

func (bo *BenchResults) SumSealingTime() error {
	if len(bo.SealingResults) <= 0 {
		return xerrors.Errorf("BenchResults SealingResults len <= 0")
	}
	if len(bo.SealingResults) != bo.SectorNumber {
		return xerrors.Errorf("BenchResults SealingResults len(%d) != bo.SectorNumber(%d)", len(bo.SealingResults), bo.SectorNumber)
	}

	for _, sealing := range bo.SealingResults {
		bo.SealingSum.AddPiece += sealing.AddPiece
		bo.SealingSum.PreCommit1 += sealing.PreCommit1
		bo.SealingSum.PreCommit2 += sealing.PreCommit2
		bo.SealingSum.Commit1 += sealing.Commit1
		bo.SealingSum.Commit2 += sealing.Commit2
		bo.SealingSum.Verify += sealing.Verify
		bo.SealingSum.Unseal += sealing.Unseal
	}
	return nil
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
	_ = logging.SetLogLevel("*", "INFO")

	log.Info("Starting lotus-bench")

	app := &cli.App{
		Name:                      "lotus-bench",
		Usage:                     "Benchmark performance of lotus on your hardware",
		Version:                   string(build.NodeUserVersion()),
		DisableSliceFlagSeparator: true,
		Commands: []*cli.Command{
			proveCmd,
			sealBenchCmd,
			simpleCmd,
			importBenchCmd,
			cliCmd,
			rpcCmd,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		return
	}
}

var sealBenchCmd = &cli.Command{
	Name:  "sealing",
	Usage: "Benchmark seal and winning post and window post",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "storage-dir",
			Value: "~/.lotus-bench",
			Usage: "path to the storage directory that will store sectors long term",
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
			Name:  "ticket-preimage",
			Usage: "ticket random",
		},
		&cli.StringFlag{
			Name:  "save-commit2-input",
			Usage: "save commit2 input to a file",
		},
		&cli.IntFlag{
			Name:  "num-sectors",
			Usage: "select number of sectors to seal",
			Value: 1,
		},
		&cli.IntFlag{
			Name:  "parallel",
			Usage: "num run in parallel",
			Value: 1,
		},
	},
	Action: func(c *cli.Context) error {
		if c.Bool("no-gpu") {
			err := os.Setenv("BELLMAN_NO_GPU", "1")
			if err != nil {
				return xerrors.Errorf("setting no-gpu flag: %w", err)
			}
		}

		robench := c.String("benchmark-existing-sectorbuilder")

		var sbdir string

		if robench == "" {
			sdir, err := homedir.Expand(c.String("storage-dir"))
			if err != nil {
				return err
			}

			err = os.MkdirAll(sdir, 0775) //nolint:gosec
			if err != nil {
				return xerrors.Errorf("creating sectorbuilder dir: %w", err)
			}

			tsdir, err := os.MkdirTemp(sdir, "bench")
			if err != nil {
				return err
			}
			defer func() {
				if err := os.RemoveAll(tsdir); err != nil {
					log.Warn("remove all: ", err)
				}
			}()

			// TODO: pretty sure this isnt even needed?
			if err := os.MkdirAll(tsdir, 0775); err != nil {
				return err
			}

			sbdir = tsdir
		} else {
			exp, err := homedir.Expand(robench)
			if err != nil {
				return err
			}
			sbdir = exp
		}

		// miner address
		maddr, err := address.NewFromString(c.String("miner-addr"))
		if err != nil {
			return err
		}
		amid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}
		mid := abi.ActorID(amid)

		// sector size
		sectorSizeInt, err := units.RAMInBytes(c.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)

		// Only fetch parameters if actually needed
		skipc2 := c.Bool("skip-commit2")
		if !skipc2 {
			if err := paramfetch.GetParams(lcli.ReqContext(c), build.ParametersJSON(), build.SrsJSON(), uint64(sectorSize)); err != nil {
				return xerrors.Errorf("getting params: %w", err)
			}
		}

		sbfs := &basicfs.Provider{
			Root: sbdir,
		}

		sb, err := ffiwrapper.New(sbfs)
		if err != nil {
			return err
		}

		sectorNumber := c.Int("num-sectors")

		var sealTimings []SealingResult
		var extendedSealedSectors []prooftypes.ExtendedSectorInfo
		var sealedSectors []prooftypes.SectorInfo

		if robench == "" {
			var err error
			parCfg := ParCfg{
				PreCommit1: c.Int("parallel"),
				PreCommit2: 1,
				Commit:     1,
			}
			sealTimings, extendedSealedSectors, err = runSeals(sb, sbfs, sectorNumber, parCfg, mid, sectorSize, []byte(c.String("ticket-preimage")), c.String("save-commit2-input"), skipc2, c.Bool("skip-unseal"))
			if err != nil {
				return xerrors.Errorf("failed to run seals: %w", err)
			}
			for _, s := range extendedSealedSectors {
				sealedSectors = append(sealedSectors, prooftypes.SectorInfo{
					SealedCID:    s.SealedCID,
					SectorNumber: s.SectorNumber,
					SealProof:    s.SealProof,
				})
			}
		} else {
			// TODO: implement sbfs.List() and use that for all cases (preexisting sectorbuilder or not)

			// TODO: this assumes we only ever benchmark a preseal
			// sectorbuilder directory... we need a better way to handle
			// this in other cases

			fdata, err := os.ReadFile(filepath.Join(sbdir, "pre-seal-"+maddr.String()+".json"))
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
				extendedSealedSectors = append(extendedSealedSectors, prooftypes.ExtendedSectorInfo{
					SealedCID:    s.CommR,
					SectorNumber: s.SectorID,
					SealProof:    s.ProofType,
					SectorKey:    nil,
				})
				sealedSectors = append(sealedSectors, prooftypes.SectorInfo{
					SealedCID:    s.CommR,
					SectorNumber: s.SectorID,
					SealProof:    s.ProofType,
				})
			}
		}

		bo := BenchResults{
			SectorSize:     sectorSize,
			SectorNumber:   sectorNumber,
			SealingResults: sealTimings,
		}
		if err := bo.SumSealingTime(); err != nil {
			return err
		}

		var challenge [32]byte
		_, err = rand.Read(challenge[:])
		if err != nil {
			return err
		}

		beforePost := time.Now()

		if !skipc2 {
			log.Info("generating winning post candidates")
			wipt, err := spt(sectorSize, miner.SealProofVariant_Standard).RegisteredWinningPoStProof()
			if err != nil {
				return err
			}

			fcandidates, err := verifierffi.ProofVerifier.GenerateWinningPoStSectorChallenge(context.TODO(), wipt, mid, challenge[:], uint64(len(extendedSealedSectors)))
			if err != nil {
				return err
			}

			xcandidates := make([]prooftypes.ExtendedSectorInfo, len(fcandidates))
			for i, fcandidate := range fcandidates {
				xcandidates[i] = extendedSealedSectors[fcandidate]
			}

			gencandidates := time.Now()

			log.Info("computing winning post snark (cold)")
			proof1, err := sb.GenerateWinningPoSt(context.TODO(), mid, xcandidates, challenge[:])
			if err != nil {
				return err
			}

			winningpost1 := time.Now()

			log.Info("computing winning post snark (hot)")
			proof2, err := sb.GenerateWinningPoSt(context.TODO(), mid, xcandidates, challenge[:])
			if err != nil {
				return err
			}

			candidates := make([]prooftypes.SectorInfo, len(xcandidates))
			for i, xsi := range xcandidates {
				candidates[i] = prooftypes.SectorInfo{
					SealedCID:    xsi.SealedCID,
					SectorNumber: xsi.SectorNumber,
					SealProof:    xsi.SealProof,
				}
			}

			winnningpost2 := time.Now()

			pvi1 := prooftypes.WinningPoStVerifyInfo{
				Randomness:        abi.PoStRandomness(challenge[:]),
				Proofs:            proof1,
				ChallengedSectors: candidates,
				Prover:            mid,
			}
			ok, err := verifierffi.ProofVerifier.VerifyWinningPoSt(context.TODO(), pvi1)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}

			verifyWinningPost1 := time.Now()

			pvi2 := prooftypes.WinningPoStVerifyInfo{
				Randomness:        abi.PoStRandomness(challenge[:]),
				Proofs:            proof2,
				ChallengedSectors: candidates,
				Prover:            mid,
			}

			ok, err = verifierffi.ProofVerifier.VerifyWinningPoSt(context.TODO(), pvi2)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("post verification failed")
			}
			verifyWinningPost2 := time.Now()

			ppt, err := sealedSectors[0].SealProof.RegisteredWindowPoStProof()
			if err != nil {
				return err
			}

			ppt, err = ppt.ToV1_1PostProof()
			if err != nil {
				return err
			}

			log.Info("computing window post snark (cold)")
			wproof1, _, err := sb.GenerateWindowPoSt(context.TODO(), mid, ppt, extendedSealedSectors, challenge[:])
			if err != nil {
				return err
			}

			windowpost1 := time.Now()

			log.Info("computing window post snark (hot)")
			wproof2, _, err := sb.GenerateWindowPoSt(context.TODO(), mid, ppt, extendedSealedSectors, challenge[:])
			if err != nil {
				return err
			}

			windowpost2 := time.Now()

			wpvi1 := prooftypes.WindowPoStVerifyInfo{
				Randomness:        challenge[:],
				Proofs:            wproof1,
				ChallengedSectors: sealedSectors,
				Prover:            mid,
			}
			ok, err = verifierffi.ProofVerifier.VerifyWindowPoSt(context.TODO(), wpvi1)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("window post verification failed")
			}

			verifyWindowpost1 := time.Now()

			wpvi2 := prooftypes.WindowPoStVerifyInfo{
				Randomness:        challenge[:],
				Proofs:            wproof2,
				ChallengedSectors: sealedSectors,
				Prover:            mid,
			}
			ok, err = verifierffi.ProofVerifier.VerifyWindowPoSt(context.TODO(), wpvi2)
			if err != nil {
				return err
			}
			if !ok {
				log.Error("window post verification failed")
			}

			verifyWindowpost2 := time.Now()

			bo.PostGenerateCandidates = gencandidates.Sub(beforePost)
			bo.PostWinningProofCold = winningpost1.Sub(gencandidates)
			bo.PostWinningProofHot = winnningpost2.Sub(winningpost1)
			bo.VerifyWinningPostCold = verifyWinningPost1.Sub(winnningpost2)
			bo.VerifyWinningPostHot = verifyWinningPost2.Sub(verifyWinningPost1)

			bo.PostWindowProofCold = windowpost1.Sub(verifyWinningPost2)
			bo.PostWindowProofHot = windowpost2.Sub(windowpost1)
			bo.VerifyWindowPostCold = verifyWindowpost1.Sub(windowpost2)
			bo.VerifyWindowPostHot = verifyWindowpost2.Sub(verifyWindowpost1)
		}

		bo.EnvVar = make(map[string]string)
		for _, envKey := range []string{"BELLMAN_NO_GPU", "FIL_PROOFS_USE_GPU_COLUMN_BUILDER",
			"FIL_PROOFS_USE_GPU_TREE_BUILDER", "FIL_PROOFS_USE_MULTICORE_SDR", "BELLMAN_CUSTOM_GPU"} {
			envValue, found := os.LookupEnv(envKey)
			if found {
				bo.EnvVar[envKey] = envValue
			}
		}

		if c.Bool("json-out") {
			data, err := json.MarshalIndent(bo, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(data))
		} else {
			fmt.Println("environment variable list:")
			for envKey, envValue := range bo.EnvVar {
				fmt.Printf("%s=%s\n", envKey, envValue)
			}
			fmt.Printf("----\nresults (v28) SectorSize:(%d), SectorNumber:(%d)\n", sectorSize, sectorNumber)
			if robench == "" {
				fmt.Printf("seal: addPiece: %s (%s)\n", bo.SealingSum.AddPiece, bps(bo.SectorSize, bo.SectorNumber, bo.SealingSum.AddPiece))
				fmt.Printf("seal: preCommit phase 1: %s (%s)\n", bo.SealingSum.PreCommit1, bps(bo.SectorSize, bo.SectorNumber, bo.SealingSum.PreCommit1))
				fmt.Printf("seal: preCommit phase 2: %s (%s)\n", bo.SealingSum.PreCommit2, bps(bo.SectorSize, bo.SectorNumber, bo.SealingSum.PreCommit2))
				fmt.Printf("seal: commit phase 1: %s (%s)\n", bo.SealingSum.Commit1, bps(bo.SectorSize, bo.SectorNumber, bo.SealingSum.Commit1))
				fmt.Printf("seal: commit phase 2: %s (%s)\n", bo.SealingSum.Commit2, bps(bo.SectorSize, bo.SectorNumber, bo.SealingSum.Commit2))
				fmt.Printf("seal: verify: %s\n", bo.SealingSum.Verify)
				if !c.Bool("skip-unseal") {
					fmt.Printf("unseal: %s  (%s)\n", bo.SealingSum.Unseal, bps(bo.SectorSize, bo.SectorNumber, bo.SealingSum.Unseal))
				}
				fmt.Println("")
			}
			if !skipc2 {
				fmt.Printf("generate candidates: %s (%s)\n", bo.PostGenerateCandidates, bps(bo.SectorSize, len(bo.SealingResults), bo.PostGenerateCandidates))
				fmt.Printf("compute winning post proof (cold): %s\n", bo.PostWinningProofCold)
				fmt.Printf("compute winning post proof (hot): %s\n", bo.PostWinningProofHot)
				fmt.Printf("verify winning post proof (cold): %s\n", bo.VerifyWinningPostCold)
				fmt.Printf("verify winning post proof (hot): %s\n\n", bo.VerifyWinningPostHot)

				fmt.Printf("compute window post proof (cold): %s\n", bo.PostWindowProofCold)
				fmt.Printf("compute window post proof (hot): %s\n", bo.PostWindowProofHot)
				fmt.Printf("verify window post proof (cold): %s\n", bo.VerifyWindowPostCold)
				fmt.Printf("verify window post proof (hot): %s\n", bo.VerifyWindowPostHot)
			}
		}
		return nil
	},
}

type ParCfg struct {
	PreCommit1 int
	PreCommit2 int
	Commit     int
}

func runSeals(sb *ffiwrapper.Sealer, sbfs *basicfs.Provider, numSectors int, par ParCfg, mid abi.ActorID, sectorSize abi.SectorSize, ticketPreimage []byte, saveC2inp string, skipc2, skipunseal bool) ([]SealingResult, []prooftypes.ExtendedSectorInfo, error) {
	var pieces []abi.PieceInfo
	sealTimings := make([]SealingResult, numSectors)
	sealedSectors := make([]prooftypes.ExtendedSectorInfo, numSectors)

	preCommit2Sema := make(chan struct{}, par.PreCommit2)
	commitSema := make(chan struct{}, par.Commit)

	if numSectors%par.PreCommit1 != 0 {
		return nil, nil, fmt.Errorf("parallelism factor must cleanly divide numSectors")
	}
	for i := abi.SectorNumber(0); i < abi.SectorNumber(numSectors); i++ {
		sid := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  mid,
				Number: i,
			},
			ProofType: spt(sectorSize, miner.SealProofVariant_Standard),
		}

		start := time.Now()
		log.Infof("[%d] Writing piece into sector...", i)

		pi, err := sb.AddPiece(context.TODO(), sid, nil, abi.PaddedPieceSize(sectorSize).Unpadded(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}

		pieces = append(pieces, pi)

		sealTimings[i].AddPiece = time.Since(start)
	}

	sectorsPerWorker := numSectors / par.PreCommit1

	errs := make(chan error, par.PreCommit1)
	for wid := 0; wid < par.PreCommit1; wid++ {
		go func(worker int) {
			sealerr := func() error {
				start := worker * sectorsPerWorker
				end := start + sectorsPerWorker
				for i := abi.SectorNumber(start); i < abi.SectorNumber(end); i++ {
					sid := storiface.SectorRef{
						ID: abi.SectorID{
							Miner:  mid,
							Number: i,
						},
						ProofType: spt(sectorSize, miner.SealProofVariant_Standard),
					}

					start := time.Now()

					trand := blake2b.Sum256(ticketPreimage)
					ticket := abi.SealRandomness(trand[:])

					log.Infof("[%d] Running replication(1)...", i)
					piece := []abi.PieceInfo{pieces[i]}
					pc1o, err := sb.SealPreCommit1(context.TODO(), sid, ticket, piece)
					if err != nil {
						return xerrors.Errorf("commit: %w", err)
					}

					precommit1 := time.Now()

					preCommit2Sema <- struct{}{}
					pc2Start := time.Now()
					log.Infof("[%d] Running replication(2)...", i)
					cids, err := sb.SealPreCommit2(context.TODO(), sid, pc1o)
					if err != nil {
						return xerrors.Errorf("commit: %w", err)
					}

					precommit2 := time.Now()
					<-preCommit2Sema

					sealedSectors[i] = prooftypes.ExtendedSectorInfo{
						SealProof:    sid.ProofType,
						SectorNumber: i,
						SealedCID:    cids.Sealed,
						SectorKey:    nil,
					}

					seed := lapi.SealSeed{
						Epoch: 101,
						Value: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 255},
					}

					commitSema <- struct{}{}
					commitStart := time.Now()
					log.Infof("[%d] Generating PoRep for sector (1)", i)
					c1o, err := sb.SealCommit1(context.TODO(), sid, ticket, seed.Value, piece, cids)
					if err != nil {
						return err
					}

					sealcommit1 := time.Now()

					log.Infof("[%d] Generating PoRep for sector (2)", i)

					if saveC2inp != "" {
						c2in := Commit2In{
							SectorNum:  int64(i),
							Phase1Out:  c1o,
							SectorSize: uint64(sectorSize),
						}

						b, err := json.Marshal(&c2in)
						if err != nil {
							return err
						}

						if err := os.WriteFile(saveC2inp, b, 0664); err != nil {
							log.Warnf("%+v", err)
						}
					}

					var proof storiface.Proof
					if !skipc2 {
						proof, err = sb.SealCommit2(context.TODO(), sid, c1o)
						if err != nil {
							return err
						}
					}

					sealcommit2 := time.Now()
					<-commitSema

					if !skipc2 {
						svi := prooftypes.SealVerifyInfo{
							SectorID:              abi.SectorID{Miner: mid, Number: i},
							SealedCID:             cids.Sealed,
							SealProof:             sid.ProofType,
							Proof:                 proof,
							DealIDs:               nil,
							Randomness:            ticket,
							InteractiveRandomness: seed.Value,
							UnsealedCID:           cids.Unsealed,
						}

						ok, err := verifierffi.ProofVerifier.VerifySeal(svi)
						if err != nil {
							return err
						}
						if !ok {
							return xerrors.Errorf("porep proof for sector %d was invalid", i)
						}
					}

					verifySeal := time.Now()

					if !skipunseal {
						log.Infof("[%d] Unsealing sector", i)
						{
							p, done, err := sbfs.AcquireSector(context.TODO(), sid, storiface.FTUnsealed, storiface.FTNone, storiface.PathSealing)
							if err != nil {
								return xerrors.Errorf("acquire unsealed sector for removing: %w", err)
							}
							done()

							if err := os.Remove(p.Unsealed); err != nil {
								return xerrors.Errorf("removing unsealed sector: %w", err)
							}
						}

						err := sb.UnsealPiece(context.TODO(), sid, 0, abi.PaddedPieceSize(sectorSize).Unpadded(), ticket, cids.Unsealed)
						if err != nil {
							return err
						}
					}
					unseal := time.Now()

					sealTimings[i].PreCommit1 = precommit1.Sub(start)
					sealTimings[i].PreCommit2 = precommit2.Sub(pc2Start)
					sealTimings[i].Commit1 = sealcommit1.Sub(commitStart)
					sealTimings[i].Commit2 = sealcommit2.Sub(sealcommit1)
					sealTimings[i].Verify = verifySeal.Sub(sealcommit2)
					sealTimings[i].Unseal = unseal.Sub(verifySeal)
				}
				return nil
			}()
			if sealerr != nil {
				errs <- sealerr
				return
			}
			errs <- nil
		}(wid)
	}

	for i := 0; i < par.PreCommit1; i++ {
		err := <-errs
		if err != nil {
			return nil, nil, err
		}
	}

	return sealTimings, sealedSectors, nil
}

var proveCmd = &cli.Command{
	Name:      "prove",
	Usage:     "Benchmark a proof computation",
	ArgsUsage: "[input.json]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-gpu",
			Usage: "disable gpu usage for the benchmark run",
		},
		&cli.StringFlag{
			Name:  "miner-addr",
			Usage: "pass miner address (only necessary if using existing sectorbuilder)",
			Value: "t01000",
		},
	},
	Action: func(c *cli.Context) error {
		if c.Bool("no-gpu") {
			err := os.Setenv("BELLMAN_NO_GPU", "1")
			if err != nil {
				return xerrors.Errorf("setting no-gpu flag: %w", err)
			}
		}

		if !c.Args().Present() {
			return xerrors.Errorf("Usage: lotus-bench prove [input.json]")
		}

		inb, err := os.ReadFile(c.Args().First())
		if err != nil {
			return xerrors.Errorf("reading input file: %w", err)
		}

		var c2in Commit2In
		if err := json.Unmarshal(inb, &c2in); err != nil {
			return xerrors.Errorf("unmarshalling input file: %w", err)
		}

		if err := paramfetch.GetParams(lcli.ReqContext(c), build.ParametersJSON(), build.SrsJSON(), c2in.SectorSize); err != nil {
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

		sb, err := ffiwrapper.New(nil)
		if err != nil {
			return err
		}

		ref := storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(c2in.SectorNum),
			},
			ProofType: spt(abi.SectorSize(c2in.SectorSize), miner.SealProofVariant_Standard),
		}

		fmt.Printf("----\nstart proof computation\n")
		start := time.Now()

		proof, err := sb.SealCommit2(context.TODO(), ref, c2in.Phase1Out)
		if err != nil {
			return err
		}

		sealCommit2 := time.Now()

		fmt.Printf("proof: %x\n", proof)

		fmt.Printf("----\nresults (v28) (%d)\n", c2in.SectorSize)
		dur := sealCommit2.Sub(start)

		fmt.Printf("seal: commit phase 2: %s (%s)\n", dur, bps(abi.SectorSize(c2in.SectorSize), 1, dur))
		return nil
	},
}

func bps(sectorSize abi.SectorSize, sectorNum int, d time.Duration) string {
	bdata := new(big.Int).SetUint64(uint64(sectorSize))
	bdata = bdata.Mul(bdata, big.NewInt(int64(sectorNum)))
	bdata = bdata.Mul(bdata, big.NewInt(time.Second.Nanoseconds()))
	bps := bdata.Div(bdata, big.NewInt(d.Nanoseconds()))
	return types.SizeStr(types.BigInt{Int: bps}) + "/s"
}

func spt(ssize abi.SectorSize, variant miner.SealProofVariant) abi.RegisteredSealProof {
	spt, err := miner.SealProofTypeFromSectorSize(ssize, buildconstants.TestNetworkVersion, variant)
	if err != nil {
		panic(err)
	}

	return spt
}
