package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/pprof"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
)

var log = logging.Logger("lotus-shed")

func main() {
	_ = logging.SetLogLevel("*", "INFO")
	_ = logging.SetLogLevelRegex("badger*", "ERROR")
	_ = logging.SetLogLevel("drand", "ERROR")
	_ = logging.SetLogLevel("chainstore", "ERROR")

	local := []*cli.Command{
		addressCmd,
		statActorCmd,
		statSnapshotCmd,
		statObjCmd,
		base64Cmd,
		base32Cmd,
		base16Cmd,
		bitFieldCmd,
		chainwatchCmd,
		cronWcCmd,
		frozenMinersCmd,
		dealLabelCmd,
		keyinfoCmd,
		jwtCmd,
		noncefix,
		bigIntParseCmd,
		staterootCmd,
		auditsCmd,
		importCarCmd,
		importObjectCmd,
		commpToCidCmd,
		fetchParamCmd,
		postFindCmd,
		proofsCmd,
		verifRegCmd,
		marketCmd,
		mpoolCmd,
		helloCmd,
		genesisVerifyCmd,
		mathCmd,
		minerCmd,
		mpoolStatsCmd,
		exportChainCmd,
		ethCmd,
		exportCarCmd,
		consensusCmd,
		syncCmd,
		stateTreePruneCmd,
		datastoreCmd,
		ledgerCmd,
		sectorsCmd,
		msgCmd,
		electionCmd,
		rpcCmd,
		cidCmd,
		blockmsgidCmd,
		signaturesCmd,
		actorCmd,
		minerTypesCmd,
		minerPeeridCmd,
		minerMultisigsCmd,
		splitstoreCmd,
		fr32Cmd,
		chainCmd,
		balancerCmd,
		migrationsCmd,
		diffCmd,
		msigCmd,
		invariantsCmd,
		gasTraceCmd,
		replayOfflineCmd,
		FevmAnalyticsCmd,
		mismatchesCmd,
		blockCmd,
		adlCmd,
		f3Cmd,
		findMsgCmd,
	}

	app := &cli.App{
		Name:     "lotus-shed",
		Usage:    "A place for all the lotus tools",
		Version:  string(build.NodeUserVersion()),
		Commands: local,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "miner-repo",
				Aliases: []string{"storagerepo"},
				EnvVars: []string{"LOTUS_MINER_PATH", "LOTUS_STORAGE_PATH"},
				Value:   "~/.lotusminer", // TODO: Consider XDG_DATA_HOME
				Usage:   fmt.Sprintf("Specify miner repo path. flag storagerepo and env LOTUS_STORAGE_PATH are DEPRECATION, will REMOVE SOON"),
			},
			&cli.StringFlag{
				Name:  "log-level",
				Value: "info",
			},
			&cli.StringFlag{
				Name:  "pprof",
				Usage: "specify name of file for writing cpu profile to",
			},
			&cli.UintFlag{
				Name:  "pprof-port",
				Usage: "specify port to run pprof server on",
				Action: func(_ *cli.Context, port uint) error {
					if port > 65535 {
						return xerrors.New("invalid port number")
					}
					return nil
				},
			},
		},
		Before: func(cctx *cli.Context) error {
			if prof := cctx.String("pprof"); prof != "" {
				profile, err := os.Create(prof)
				if err != nil {
					return err
				}

				if err := pprof.StartCPUProfile(profile); err != nil {
					return err
				}
			}

			if port := cctx.Int("pprof-port"); port != 0 {
				go func() {
					log.Infow("Starting pprof server", "port", port)
					server := &http.Server{
						Addr:              fmt.Sprintf("localhost:%d", port),
						ReadHeaderTimeout: 5 * time.Second,
					}
					if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
						log.Errorw("pprof server stopped unexpectedly", "err", err)
					}
				}()
			}

			return logging.SetLogLevel("lotus-shed", cctx.String("log-level"))
		},
		After: func(cctx *cli.Context) error {
			if prof := cctx.String("pprof"); prof != "" {
				pprof.StopCPUProfile()
			}
			return nil
		},
	}

	// terminate early on ctrl+c
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		cancel()
		fmt.Println("Received interrupt, shutting down... Press CTRL+C again to force shutdown")
		<-c
		fmt.Println("Forcing stop")
		os.Exit(1)
	}()

	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Errorf("%+v", err)
		os.Exit(1)
		return
	}
}
