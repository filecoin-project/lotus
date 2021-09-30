package main

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	cliutil "github.com/filecoin-project/lotus/cli/util"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/lib/tracing"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("main")

const (
	FlagMinerRepo   = "miner-repo"
	FlagMarketsRepo = "markets-repo"
)

// TODO remove after deprecation period
const FlagMinerRepoDeprecation = "storagerepo"

func main() {
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
		lcli.WithCategory("market", storageDealsCmd),
		lcli.WithCategory("market", retrievalDealsCmd),
		lcli.WithCategory("market", dataTransfersCmd),
		lcli.WithCategory("market", dagstoreCmd),
		lcli.WithCategory("storage", sectorsCmd),
		lcli.WithCategory("storage", provingCmd),
		lcli.WithCategory("storage", storageCmd),
		lcli.WithCategory("storage", sealingCmd),
		lcli.WithCategory("retrieval", piecesCmd),
	}
	jaeger := tracing.SetupJaegerTracing("lotus")
	defer func() {
		if jaeger != nil {
			jaeger.Flush()
		}
	}()

	for _, cmd := range local {
		cmd := cmd
		originBefore := cmd.Before
		cmd.Before = func(cctx *cli.Context) error {
			trace.UnregisterExporter(jaeger)
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

	// adapt the Net* commands to always hit the node running the markets
	// subsystem, as that is the only one that runs a libp2p node.
	netCmd := *lcli.NetCmd // make a copy.
	prev := netCmd.Before
	netCmd.Before = func(c *cli.Context) error {
		if prev != nil {
			if err := prev(c); err != nil {
				return err
			}
		}
		c.App.Metadata["repoType"] = repo.Markets
		return nil
	}

	app := &cli.App{
		Name:                 "lotus-miner",
		Usage:                "Filecoin decentralized storage network miner",
		Version:              build.UserVersion(),
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
			&cli.StringFlag{
				Name:    FlagMarketsRepo,
				EnvVars: []string{"LOTUS_MARKETS_PATH"},
				Usage:   fmt.Sprintf("Markets repo path"),
			},
			&cli.BoolFlag{
				Name:  "call-on-markets",
				Usage: "(experimental; may be removed) call this command against a markets node; use only with common commands like net, auth, pprof, etc. whose target may be ambiguous",
			},
			cliutil.FlagVeryVerbose,
		},
		Commands: append(local, append(lcli.CommonCommands, &netCmd)...),
		Before: func(c *cli.Context) error {
			// this command is explicitly called on markets, inform
			// common commands by overriding the repoType.
			if c.Bool("call-on-markets") {
				c.App.Metadata["repoType"] = repo.Markets
			}
			return nil
		},
		After: func(c *cli.Context) error {
			if r := recover(); r != nil {
				// Generate report in LOTUS_PATH and re-raise panic
				build.GeneratePanicReport(c.String("panic-reports"), c.String(FlagMinerRepo), c.App.Name)
				panic(r)
			}
			return nil
		},
	}
	app.Setup()
	app.Metadata["repoType"] = repo.StorageMiner
	lcli.RunApp(app)
}

func getActorAddress(ctx context.Context, cctx *cli.Context) (maddr address.Address, err error) {
	if cctx.IsSet("actor") {
		maddr, err = address.NewFromString(cctx.String("actor"))
		if err != nil {
			return maddr, err
		}
		return
	}

	nodeAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
	if err != nil {
		return address.Undef, err
	}
	defer closer()

	maddr, err = nodeAPI.ActorAddress(ctx)
	if err != nil {
		return maddr, xerrors.Errorf("getting actor address: %w", err)
	}

	return maddr, nil
}
