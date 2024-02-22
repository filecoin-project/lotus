package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/curio/guidedSetup"
	"github.com/filecoin-project/lotus/node/repo"
)

var configMigrateCmd = &cli.Command{
	Name:        "from-miner",
	Usage:       "Express a database config (for curio) from an existing miner.",
	Description: "Express a database config (for curio) from an existing miner.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    FlagMinerRepo,
			Aliases: []string{FlagMinerRepoDeprecation},
			EnvVars: []string{"LOTUS_MINER_PATH", "LOTUS_STORAGE_PATH"},
			Value:   "~/.lotusminer",
			Usage:   fmt.Sprintf("Specify miner repo path. flag(%s) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON", FlagMinerRepoDeprecation),
		},
		&cli.StringFlag{
			Name:    "repo",
			EnvVars: []string{"LOTUS_PATH"},
			Hidden:  true,
			Value:   "~/.lotus",
		},
		&cli.StringFlag{
			Name:    "to-layer",
			Aliases: []string{"t"},
			Usage:   "The layer name for this data push. 'base' is recommended for single-miner setup.",
		},
		&cli.BoolFlag{
			Name:    "overwrite",
			Aliases: []string{"o"},
			Usage:   "Use this with --to-layer to replace an existing layer",
		},
	},
	Action: fromMiner,
}

const (
	FlagMinerRepo = "miner-repo"
)

const FlagMinerRepoDeprecation = "storagerepo"

func fromMiner(cctx *cli.Context) (err error) {
	minerRepoPath := cctx.String(FlagMinerRepo)
	layerName := cctx.String("to-layer")
	overwrite := cctx.Bool("overwrite")

	// Populate API Key
	_, header, err := cliutil.GetRawAPI(cctx, repo.FullNode, "v0")
	if err != nil {
		return fmt.Errorf("cannot read API: %w", err)
	}
	err = guidedSetup.SaveConfigToLayer(minerRepoPath, layerName, overwrite, header)
	return err
}
