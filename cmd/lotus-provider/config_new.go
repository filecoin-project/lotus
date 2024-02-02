package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/fatih/color"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"io"
)

var configNewCmd = &cli.Command{
	Name:      "new-cluster",
	Usage:     "Create new coniguration for a new cluster",
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
		configColor := color.New(color.FgHiGreen).SprintFunc()

		if cctx.Args().Len() < 1 {
			return xerrors.New("must specify at least one SP actor address. Use 'lotus-shed miner create'")
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
			return fmt.Errorf("miner cannot reach the db. Ensure the config toml's HarmonyDB entry"+
				" is setup to reach Yugabyte correctly: %s", err.Error())
		}

		name := cctx.String("to-layer")
		if name == "" {
			name = fmt.Sprintf("cluster%d", len(titles))
		} else {
			if lo.Contains(titles, name) && !cctx.Bool("overwrite") {
				return xerrors.New("the overwrite flag is needed to replace existing layer: " + name)
			}
		}
		msg := "Layer " + configColor(name) + ` created. `

		// setup config
		lpCfg := config.DefaultLotusProvider()

		for _, addr := range cctx.Args().Slice() {
			maddr, err := address.NewFromString(addr)
			if err != nil {
				return xerrors.Errorf("Invalid address: %s", addr)
			}

			_, err = full.StateMinerInfo(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("Failed to get miner info: %w", err)
			}

			lpCfg.Addresses.MinerAddresses = append(lpCfg.Addresses.MinerAddresses, addr)
		}

		{
			sk, err := io.ReadAll(io.LimitReader(rand.Reader, 32))
			if err != nil {
				return err
			}

			lpCfg.Apis.StorageRPCSecret = base64.StdEncoding.EncodeToString(sk)
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

			lpCfg.Apis.ChainApiInfo = append(lpCfg.Apis.ChainApiInfo, fmt.Sprintf("%s:%s", string(token), ainfo.Addr))
		}

		// write config

		configTOML := &bytes.Buffer{}
		if err = toml.NewEncoder(configTOML).Encode(lpCfg); err != nil {
			return err
		}

		if !lo.Contains(titles, "base") {
			cfg, err := getDefaultConfig(true)
			if err != nil {
				return xerrors.Errorf("Cannot get default config: %w", err)
			}
			_, err = db.Exec(ctx, "INSERT INTO harmony_config (title, config) VALUES ('base', $1)", cfg)

			if err != nil {
				return err
			}
		}

		if cctx.Bool("overwrite") {
			i, err := db.Exec(ctx, "DELETE FROM harmony_config WHERE title=$1", name)
			if i != 0 {
				fmt.Println("Overwriting existing layer")
			}
			if err != nil {
				fmt.Println("Got error while deleting existing layer: " + err.Error())
			}
		}

		_, err = db.Exec(ctx, "INSERT INTO harmony_config (title, config) VALUES ($1, $2)", name, configTOML.String())
		if err != nil {
			return err
		}

		fmt.Println(msg)
		return nil
	},
}
