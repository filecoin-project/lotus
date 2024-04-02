package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
)

var configNewCmd = &cli.Command{
	Name:      "new-cluster",
	Usage:     "Create new configuration for a new cluster",
	ArgsUsage: "[SP actor address...]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "repo",
			EnvVars: []string{"LOTUS_PATH"},
			Hidden:  true,
			Value:   "~/.lotus",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 1 {
			return xerrors.New("must specify at least one SP actor address. Use 'lotus-shed miner create' or use 'curio guided-setup'")
		}

		ctx := cctx.Context

		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		full, closer, err := cliutil.GetFullNodeAPIV1(cctx)
		if err != nil {
			return xerrors.Errorf("connecting to full node: %w", err)
		}
		defer closer()

		var titles []string
		err = db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
		if err != nil {
			return fmt.Errorf("cannot reach the db. Ensure that Yugabyte flags are set correctly to"+
				" reach Yugabyte: %s", err.Error())
		}

		// setup config
		curioConfig := config.DefaultCurioConfig()

		for _, addr := range cctx.Args().Slice() {
			maddr, err := address.NewFromString(addr)
			if err != nil {
				return xerrors.Errorf("Invalid address: %s", addr)
			}

			_, err = full.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("Failed to get miner info: %w", err)
			}

			curioConfig.Addresses = append(curioConfig.Addresses, config.CurioAddresses{
				PreCommitControl:      []string{},
				CommitControl:         []string{},
				TerminateControl:      []string{},
				DisableOwnerFallback:  false,
				DisableWorkerFallback: false,
				MinerAddresses:        []string{addr},
			})
		}

		{
			sk, err := io.ReadAll(io.LimitReader(rand.Reader, 32))
			if err != nil {
				return err
			}

			curioConfig.Apis.StorageRPCSecret = base64.StdEncoding.EncodeToString(sk)
		}

		{
			ainfo, err := cliutil.GetAPIInfo(cctx, repo.FullNode)
			if err != nil {
				return xerrors.Errorf("could not get API info for FullNode: %w", err)
			}

			token, err := full.AuthNew(ctx, api.AllPermissions)
			if err != nil {
				return err
			}

			curioConfig.Apis.ChainApiInfo = append(curioConfig.Apis.ChainApiInfo, fmt.Sprintf("%s:%s", string(token), ainfo.Addr))
		}

		curioConfig.Addresses = lo.Filter(curioConfig.Addresses, func(a config.CurioAddresses, _ int) bool {
			return len(a.MinerAddresses) > 0
		})

		// If no base layer is present
		if !lo.Contains(titles, "base") {
			cb, err := config.ConfigUpdate(curioConfig, config.DefaultCurioConfig(), config.Commented(true), config.DefaultKeepUncommented(), config.NoEnv())
			if err != nil {
				return xerrors.Errorf("Failed to generate default config: %w", err)
			}
			cfg := string(cb)
			_, err = db.Exec(ctx, "INSERT INTO harmony_config (title, config) VALUES ('base', $1)", cfg)
			if err != nil {
				return xerrors.Errorf("failed to insert the 'base' into the database: %w", err)
			}
			fmt.Printf("The base layer has been updated with miner[s] %s\n", cctx.Args().Slice())
			return nil
		}

		// if base layer is present
		baseCfg := config.DefaultCurioConfig()
		var baseText string
		err = db.QueryRow(ctx, "SELECT config FROM harmony_config WHERE title='base'").Scan(&baseText)
		if err != nil {
			return xerrors.Errorf("Cannot load base config from database: %w", err)
		}
		_, err = deps.LoadConfigWithUpgrades(baseText, baseCfg)
		if err != nil {
			return xerrors.Errorf("Cannot parse base config: %w", err)
		}

		baseCfg.Addresses = append(baseCfg.Addresses, curioConfig.Addresses...)
		baseCfg.Addresses = lo.Filter(baseCfg.Addresses, func(a config.CurioAddresses, _ int) bool {
			return len(a.MinerAddresses) > 0
		})

		cb, err := config.ConfigUpdate(baseCfg, config.DefaultCurioConfig(), config.Commented(true), config.DefaultKeepUncommented(), config.NoEnv())
		if err != nil {
			return xerrors.Errorf("cannot interpret config: %w", err)
		}
		_, err = db.Exec(ctx, "UPDATE harmony_config SET config=$1 WHERE title='base'", string(cb))
		if err != nil {
			return xerrors.Errorf("cannot update base config: %w", err)
		}

		fmt.Printf("The base layer has been updated with miner[s] %s\n", cctx.Args().Slice())
		return nil
	},
}
