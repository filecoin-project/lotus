package guidedsetup

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ipfs/go-datastore"
	"github.com/samber/lo"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
)

const (
	FlagMinerRepo = "miner-repo"
)

const FlagMinerRepoDeprecation = "storagerepo"

func SaveConfigToLayer(minerRepoPath, chainApiInfo string) (minerAddress address.Address, err error) {
	_, say := SetupLanguage()
	ctx := context.Background()

	r, err := repo.NewFS(minerRepoPath)
	if err != nil {
		return minerAddress, err
	}

	ok, err := r.Exists()
	if err != nil {
		return minerAddress, err
	}

	if !ok {
		return minerAddress, fmt.Errorf("repo not initialized at: %s", minerRepoPath)
	}

	lr, err := r.LockRO(repo.StorageMiner)
	if err != nil {
		return minerAddress, fmt.Errorf("locking repo: %w", err)
	}
	defer func() {
		err = lr.Close()
		if err != nil {
			fmt.Println("error closing repo: ", err)
		}
	}()

	cfgNode, err := lr.Config()
	if err != nil {
		return minerAddress, fmt.Errorf("getting node config: %w", err)
	}
	smCfg := cfgNode.(*config.StorageMiner)

	db, err := harmonydb.NewFromConfig(smCfg.HarmonyDB)
	if err != nil {
		return minerAddress, fmt.Errorf("could not reach the database. Ensure the Miner config toml's HarmonyDB entry"+
			" is setup to reach Yugabyte correctly: %w", err)
	}

	var titles []string
	err = db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
	if err != nil {
		return minerAddress, fmt.Errorf("miner cannot reach the db. Ensure the config toml's HarmonyDB entry"+
			" is setup to reach Yugabyte correctly: %s", err.Error())
	}

	// Copy over identical settings:

	buf, err := os.ReadFile(path.Join(lr.Path(), "config.toml"))
	if err != nil {
		return minerAddress, fmt.Errorf("could not read config.toml: %w", err)
	}
	curioCfg := config.DefaultCurioConfig()

	ensureEmptyArrays(curioCfg)
	_, err = deps.LoadConfigWithUpgrades(string(buf), curioCfg)

	if err != nil {
		return minerAddress, fmt.Errorf("could not decode toml: %w", err)
	}

	// Populate Miner Address
	mmeta, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		return minerAddress, xerrors.Errorf("opening miner metadata datastore: %w", err)
	}
	defer func() {
		// _ = mmeta.Close()
	}()

	maddrBytes, err := mmeta.Get(ctx, datastore.NewKey("miner-address"))
	if err != nil {
		return minerAddress, xerrors.Errorf("getting miner address datastore entry: %w", err)
	}

	addr, err := address.NewFromBytes(maddrBytes)
	if err != nil {
		return minerAddress, xerrors.Errorf("parsing miner actor address: %w", err)
	}

	minerAddress = addr

	curioCfg.Addresses = []config.CurioAddresses{{
		MinerAddresses:        []string{addr.String()},
		PreCommitControl:      smCfg.Addresses.PreCommitControl,
		CommitControl:         smCfg.Addresses.CommitControl,
		TerminateControl:      smCfg.Addresses.TerminateControl,
		DisableOwnerFallback:  smCfg.Addresses.DisableOwnerFallback,
		DisableWorkerFallback: smCfg.Addresses.DisableWorkerFallback,
	}}

	ks, err := lr.KeyStore()
	if err != nil {
		return minerAddress, xerrors.Errorf("keystore err: %w", err)
	}
	js, err := ks.Get(modules.JWTSecretName)
	if err != nil {
		return minerAddress, xerrors.Errorf("error getting JWTSecretName: %w", err)
	}

	curioCfg.Apis.StorageRPCSecret = base64.StdEncoding.EncodeToString(js.PrivateKey)

	curioCfg.Apis.ChainApiInfo = append(curioCfg.Apis.ChainApiInfo, chainApiInfo)
	// Express as configTOML
	configTOML := &bytes.Buffer{}
	if err = toml.NewEncoder(configTOML).Encode(curioCfg); err != nil {
		return minerAddress, err
	}

	if lo.Contains(titles, "base") {
		// append addresses
		var baseCfg = config.DefaultCurioConfig()
		var baseText string
		err = db.QueryRow(ctx, "SELECT config FROM harmony_config WHERE title='base'").Scan(&baseText)
		if err != nil {
			return minerAddress, xerrors.Errorf("Cannot load base config: %w", err)
		}
		ensureEmptyArrays(baseCfg)
		_, err := deps.LoadConfigWithUpgrades(baseText, baseCfg)
		if err != nil {
			return minerAddress, xerrors.Errorf("Cannot load base config: %w", err)
		}
		for _, addr := range baseCfg.Addresses {
			if lo.Contains(addr.MinerAddresses, curioCfg.Addresses[0].MinerAddresses[0]) {
				goto skipWritingToBase
			}
		}
		// write to base
		{
			baseCfg.Addresses = append(baseCfg.Addresses, curioCfg.Addresses[0])
			baseCfg.Addresses = lo.Filter(baseCfg.Addresses, func(a config.CurioAddresses, _ int) bool {
				return len(a.MinerAddresses) > 0
			})
			if baseCfg.Apis.ChainApiInfo == nil {
				baseCfg.Apis.ChainApiInfo = append(baseCfg.Apis.ChainApiInfo, chainApiInfo)
			}
			if baseCfg.Apis.StorageRPCSecret == "" {
				baseCfg.Apis.StorageRPCSecret = curioCfg.Apis.StorageRPCSecret
			}

			cb, err := config.ConfigUpdate(baseCfg, config.DefaultCurioConfig(), config.Commented(true), config.DefaultKeepUncommented(), config.NoEnv())
			if err != nil {
				return minerAddress, xerrors.Errorf("cannot interpret config: %w", err)
			}
			_, err = db.Exec(ctx, "UPDATE harmony_config SET config=$1 WHERE title='base'", string(cb))
			if err != nil {
				return minerAddress, xerrors.Errorf("cannot update base config: %w", err)
			}
			say(plain, "Configuration 'base' was updated to include this miner's address (%s) and its wallet setup.", minerAddress)
		}
		say(plain, "Compare the configurations %s to %s. Changes between the miner IDs other than wallet addresses should be a new, minimal layer for runners that need it.", "base", "mig-"+curioCfg.Addresses[0].MinerAddresses[0])
	skipWritingToBase:
	} else {
		_, err = db.Exec(ctx, `INSERT INTO harmony_config (title, config) VALUES ('base', $1)
		 ON CONFLICT(title) DO UPDATE SET config=EXCLUDED.config`, configTOML)

		if err != nil {
			return minerAddress, xerrors.Errorf("Cannot insert base config: %w", err)
		}
		say(notice, "Configuration 'base' was created to resemble this lotus-miner's config.toml .")
	}

	{ // make a layer representing the migration
		layerName := fmt.Sprintf("mig-%s", curioCfg.Addresses[0].MinerAddresses[0])
		_, err = db.Exec(ctx, "DELETE FROM harmony_config WHERE title=$1", layerName)
		if err != nil {
			return minerAddress, xerrors.Errorf("Cannot delete existing layer: %w", err)
		}

		_, err = db.Exec(ctx, "INSERT INTO harmony_config (title, config) VALUES ($1, $2)", layerName, configTOML.String())
		if err != nil {
			return minerAddress, xerrors.Errorf("Cannot insert layer after layer created message: %w", err)
		}
		say(plain, "Layer %s created. ", layerName)
	}

	dbSettings := getDBSettings(*smCfg)
	say(plain, "To work with the config: ")
	fmt.Println(code.Render(`curio ` + dbSettings + ` config edit base`))
	say(plain, `To run Curio: With machine or cgroup isolation, use the command (with example layer selection):`)
	fmt.Println(code.Render(`curio ` + dbSettings + ` run --layer=post`))
	return minerAddress, nil
}

func getDBSettings(smCfg config.StorageMiner) string {
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
	return dbSettings
}

func ensureEmptyArrays(cfg *config.CurioConfig) {
	if cfg.Addresses == nil {
		cfg.Addresses = []config.CurioAddresses{}
	} else {
		for i := range cfg.Addresses {
			if cfg.Addresses[i].PreCommitControl == nil {
				cfg.Addresses[i].PreCommitControl = []string{}
			}
			if cfg.Addresses[i].CommitControl == nil {
				cfg.Addresses[i].CommitControl = []string{}
			}
			if cfg.Addresses[i].TerminateControl == nil {
				cfg.Addresses[i].TerminateControl = []string{}
			}
		}
	}
	if cfg.Apis.ChainApiInfo == nil {
		cfg.Apis.ChainApiInfo = []string{}
	}
}
