package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
)

var providerCmd = &cli.Command{
	Name:        "provider",
	Description: "Run a lotus-provider helper",
	Subcommands: []*cli.Command{
		{
			Name:        "from-miner",
			Description: "Express a database config from an existing miner.",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    FlagMinerRepo,
					Aliases: []string{FlagMinerRepoDeprecation},
					EnvVars: []string{"LOTUS_MINER_PATH", "LOTUS_STORAGE_PATH"},
					Value:   "~/.lotusminer",
					Usage:   fmt.Sprintf("Specify miner repo path. flag(%s) and env(LOTUS_STORAGE_PATH) are DEPRECATION, will REMOVE SOON", FlagMinerRepoDeprecation),
				},
				&cli.StringFlag{
					Name:    "layer",
					Aliases: []string{"l"},
					Usage:   "The layer name for this data push. 'base' is recommended for single-miner setup. Overwrites.",
				},
			},
			Action: fromMiner,
		},
	},
}

const (
	FlagMinerRepo = "miner-repo"
)

const FlagMinerRepoDeprecation = "storagerepo"

func fromMiner(cctx *cli.Context) error {
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
	err = db.Select(ctx, &titles, `SELECT title FROM harmony_config`)
	if err != nil {
		return fmt.Errorf("miner cannot reach the db. Ensure the config toml's HarmonyDB entry"+
			" is setup to reach Yugabyte correctly: %s", err.Error())
	}
	name := cctx.String("layer")
	if name == "" {
		name = fmt.Sprintf("mig%d", len(titles))
	}
	msg := "Layer " + name + ` created. `

	// Copy over identical settings:

	buf, err := os.ReadFile(path.Join(lr.Path(), "config.toml"))
	if err != nil {
		return fmt.Errorf("could not read config.toml: %w", err)
	}
	var lpCfg config.LotusProviderConfig
	_, err = toml.Decode(string(buf), &lpCfg)
	if err != nil {
		return fmt.Errorf("could not decode toml: %w", err)
	}

	// Populate Miner Address
	sm, cc, err := cliutil.GetStorageMinerAPI(cctx)
	if err != nil {
		return fmt.Errorf("could not get storageMiner API: %w", err)
	}
	defer cc()
	addr, err := sm.ActorAddress(ctx)
	if err != nil {
		return fmt.Errorf("could not read actor address: %w", err)
	}

	lpCfg.Addresses.MinerAddresses = []string{addr.String()}

	// TODO Apis.StorageSecret  storage repo and reading the rpc secret
	//lr.??

	// Populate API Key
	_, header, err := cliutil.GetRawAPI(cctx, repo.FullNode, "v0")
	if err != nil {
		return fmt.Errorf("cannot read API: %w", err)
	}

	lpCfg.Apis.FULLNODE_API_INFO = []string{header.Get("Authorization")[7:]}

	// Enable WindowPoSt
	lpCfg.Subsystems.EnableWindowPost = true
	msg += `\nBefore running lotus-provider, ensure any miner/worker answering of WindowPost is disabled.\n`

	// Express as configTOML
	configTOML := &bytes.Buffer{}
	if err = toml.NewEncoder(configTOML).Encode(lpCfg); err != nil {
		return err
	}

	if !lo.Contains(titles, "base") {
		_, err = db.Exec(ctx, "INSERT INTO harmony_config (title, config) VALUES ('base', '')", "base")
		if err != nil {
			return err
		}
	}
	_, err = db.Exec(ctx, "INSERT INTO harmony_config (title, config) VALUES ($1, $2)", name, configTOML.String())
	if err != nil {
		return err
	}

	dbSettings := ""
	def := config.DefaultStorageMiner().HarmonyDB
	if def.Hosts[0] != smCfg.HarmonyDB.Hosts[0] {
		dbSettings += ` --db-host="` + strings.Join(smCfg.HarmonyDB.Hosts, ",") + `"`
	}
	if def.Port != smCfg.HarmonyDB.Port {
		dbSettings += " --db-port=" + smCfg.HarmonyDB.Port
	}
	if def.Username != smCfg.HarmonyDB.Username {
		dbSettings += ` --db-user="` + smCfg.HarmonyDB.Username + `"`
	}
	if def.Password != smCfg.HarmonyDB.Password {
		dbSettings += ` --db-password="` + smCfg.HarmonyDB.Password + `"`
	}
	if def.Database != smCfg.HarmonyDB.Database {
		dbSettings += ` --db-name="` + smCfg.HarmonyDB.Database + `"`
	}

	msg += `
To work with the config:
./lotus-provider ` + dbSettings + ` config help `
	msg += `
To run Lotus Provider: in its own machine or cgroup without other files, use the command: 
./lotus-provider ` + dbSettings + ` run --layers="` + name + `"
	`
	fmt.Println(msg)
	return nil
}
