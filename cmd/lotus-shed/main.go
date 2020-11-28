package main

import (
	"fmt"
	"os"

	"github.com/filecoin-project/lotus/build"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
)

var log = logging.Logger("lotus-shed")

func main() {
	logging.SetLogLevel("*", "INFO")

	local := []*cli.Command{
		base64Cmd,
		base32Cmd,
		base16Cmd,
		bitFieldCmd,
		frozenMinersCmd,
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
		miscCmd,
		mpoolCmd,
		genesisVerifyCmd,
		mathCmd,
		mpoolStatsCmd,
		exportChainCmd,
		consensusCmd,
		rollupDealStatsCmd,
		syncCmd,
		stateTreePruneCmd,
		datastoreCmd,
		ledgerCmd,
		sectorsCmd,
		msgCmd,
		electionCmd,
		rpcCmd,
		cidCmd,
	}

	app := &cli.App{
		Name:     "lotus-shed",
		Usage:    "A place for all the lotus tools",
		Version:  build.BuildVersion,
		Commands: local,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus",
			},
			&cli.StringFlag{
				Name:    "miner-repo",
				EnvVars: []string{"LOTUS_MINER_PATH"},
				Value:   "~/.lotusminer",
				Usage:   fmt.Sprintf("Specify miner repo path"),
			},
			&cli.StringFlag{
				Name:  "log-level",
				Value: "info",
			},
		},
		Before: func(cctx *cli.Context) error {
			return logging.SetLogLevel("lotus-shed", cctx.String("log-level"))
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		os.Exit(1)
		return
	}
}
