package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"

	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"

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
		miscCmd,
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
		sendCsvCmd,
		terminationsCmd,
		migrationsCmd,
		diffCmd,
		itestdCmd,
		msigCmd,
		fip36PollCmd,
		invariantsCmd,
		gasTraceCmd,
		replayOfflineCmd,
		indexesCmd,
		FevmAnalyticsCmd,
		mismatchesCmd,
		blockCmd,
	}

	app := &cli.App{
		Name:     "lotus-shed",
		Usage:    "A place for all the lotus tools",
		Version:  build.UserVersion(),
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
