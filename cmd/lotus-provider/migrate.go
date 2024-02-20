package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/ipfs/go-datastore"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/guidedSetup"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
)

var configMigrateCmd = &cli.Command{
	Name:        "from-miner",
	Usage:       "Express a database config (for lotus-provider) from an existing miner.",
	Description: "Express a database config (for lotus-provider) from an existing miner.",
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
	ctx := context.Background()

	r, err := repo.NewFS(cctx.String(FlagMinerRepo))
	if err != nil {
		return err
	}

	ok, err := r.Exists()
	if err != nil {
		return err
	}

	if !ok {
		return fmt.Errorf("repo not initialized")
	}

	lr, err := r.LockRO(repo.StorageMiner)
	if err != nil {
		return fmt.Errorf("locking repo: %w", err)
	}
	defer func() { _ = lr.Close() }()

	cfgNode, err := lr.Config()
	if err != nil {
		return fmt.Errorf("getting node config: %w", err)
	}
	smCfg := cfgNode.(*config.StorageMiner)

	db, err := harmonydb.NewFromConfig(smCfg.HarmonyDB)
	if err != nil {
		return fmt.Errorf("could not reach the database. Ensure the Miner config toml's HarmonyDB entry"+
			" is setup to reach Yugabyte correctly: %w", err)
	}

	var titles []string
	err = db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
	if err != nil {
		return fmt.Errorf("miner cannot reach the db. Ensure the config toml's HarmonyDB entry"+
			" is setup to reach Yugabyte correctly: %s", err.Error())
	}
	name := cctx.String("to-layer")
	if name == "" {
		name = fmt.Sprintf("mig%d", len(titles))
	} else {
		if lo.Contains(titles, name) && !cctx.Bool("overwrite") {
			return errors.New("the overwrite flag is needed to replace existing layer: " + name)
		}
	}

	// Copy over identical settings:

	buf, err := os.ReadFile(path.Join(lr.Path(), "config.toml"))
	if err != nil {
		return fmt.Errorf("could not read config.toml: %w", err)
	}
	var lpCfg config.LotusProviderConfig
	_, err = deps.LoadConfigWithUpgrades(string(buf), &lpCfg)
	if err != nil {
		return fmt.Errorf("could not decode toml: %w", err)
	}

	// Populate Miner Address
	mmeta, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		return xerrors.Errorf("opening miner metadata datastore: %w", err)
	}
	defer func() {
		_ = mmeta.Close()
	}()

	maddrBytes, err := mmeta.Get(ctx, datastore.NewKey("miner-address"))
	if err != nil {
		return xerrors.Errorf("getting miner address datastore entry: %w", err)
	}

	addr, err := address.NewFromBytes(maddrBytes)
	if err != nil {
		return xerrors.Errorf("parsing miner actor address: %w", err)
	}

	lpCfg.Addresses = []config.LotusProviderAddresses{{
		MinerAddresses: []string{addr.String()},
	}}

	ks, err := lr.KeyStore()
	if err != nil {
		return xerrors.Errorf("keystore err: %w", err)
	}
	js, err := ks.Get(modules.JWTSecretName)
	if err != nil {
		return xerrors.Errorf("error getting JWTSecretName: %w", err)
	}

	lpCfg.Apis.StorageRPCSecret = base64.StdEncoding.EncodeToString(js.PrivateKey)

	// Populate API Key
	_, header, err := cliutil.GetRawAPI(cctx, repo.FullNode, "v0")
	if err != nil {
		return fmt.Errorf("cannot read API: %w", err)
	}
	err = guidedSetup.SaveConfigToLayer(minerRepoPath, layerName, overwrite, header)
	return err
}
