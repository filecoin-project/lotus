package miner

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/lib/tracing"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("lotus-miner")

const (
	FlagMinerRepo = "miner-repo"
)

// TODO remove after deprecation period
const FlagMinerRepoDeprecation = "storagerepo"

func App() *cli.App {
	api.RunningNodeType = api.NodeMiner

	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		initCmd,
		runCmd,
		stopCmd,
		configCmd,
		backupCmd,
		lcli.WithCategory("chain", actorCmd),
		lcli.WithCategory("chain", infoCmd),
		lcli.WithCategory("storage", sectorsCmd),
		lcli.WithCategory("storage", provingCmd),
		lcli.WithCategory("storage", storageCmd),
		lcli.WithCategory("storage", sealingCmd),
	}

	jaeger := tracing.SetupJaegerTracing("lotus")
	defer func() {
		if jaeger != nil {
			_ = jaeger.ForceFlush(context.Background())
		}
	}()

	for _, cmd := range local {
		cmd := cmd
		originBefore := cmd.Before
		cmd.Before = func(cctx *cli.Context) error {
			if jaeger != nil {
				_ = jaeger.Shutdown(cctx.Context)
			}
			jaeger = tracing.SetupJaegerTracing("lotus/" + cmd.Name)

			if cctx.IsSet("color") {
				color.NoColor = !cctx.Bool("color")
			}

			if originBefore != nil {
				return originBefore(cctx)
			}

			return nil
		}
	}

	app := &cli.App{
		Name:                 "lotus-miner",
		Usage:                "Filecoin decentralized storage network miner",
		Version:              string(build.MinerUserVersion()),
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "actor",
				Value:   "",
				Usage:   "specify other actor to query / manipulate",
				Aliases: []string{"a"},
			},
			&cli.BoolFlag{
				// examined in the Before above
				Name:        "color",
				Usage:       "use color in display output",
				DefaultText: "depends on output being a TTY",
			},
			&cli.StringFlag{
				Name:    "panic-reports",
				EnvVars: []string{"LOTUS_PANIC_REPORT_PATH"},
				Hidden:  true,
				Value:   "~/.lotusminer", // should follow --repo default
			},
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    FlagMinerRepo,
				Aliases: []string{FlagMinerRepoDeprecation},
				EnvVars: []string{"LOTUS_MINER_PATH", "LOTUS_STORAGE_PATH"},
				Value:   "~/.lotusminer", // TODO: Consider XDG_DATA_HOME
				Usage:   fmt.Sprintf("Specify miner repo path. flag(%s) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON", FlagMinerRepoDeprecation),
			},
			cliutil.FlagVeryVerbose,
		},
		Commands: append(local, lcli.CommonCommands...),
		After: func(c *cli.Context) error {
			if r := recover(); r != nil {
				// Generate report in LOTUS_PATH and re-raise panic
				build.GenerateMinerPanicReport(c.String("panic-reports"), c.String(FlagMinerRepo), c.App.Name)
				panic(r)
			}
			return nil
		},
	}
	app.Setup()
	app.Metadata["repoType"] = repo.StorageMiner
	return app
}

func getActorAddress(ctx context.Context, cctx *cli.Context) (maddr address.Address, err error) {
	if cctx.IsSet("actor") {
		maddr, err = address.NewFromString(cctx.String("actor"))
		if err != nil {
			return maddr, err
		}
		return
	}

	minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
	if err != nil {
		return address.Undef, err
	}
	defer closer()

	maddr, err = minerApi.ActorAddress(ctx)
	if err != nil {
		return maddr, xerrors.Errorf("getting actor address: %w", err)
	}

	return maddr, nil
}

func LMActorOrEnvGetter(cctx *cli.Context) (address.Address, error) {
	return getActorAddress(cctx.Context, cctx)
}
