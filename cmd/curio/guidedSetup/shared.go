package guidedSetup

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ipfs/go-datastore"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
)

func SaveConfigToLayer(minerRepoPath, layerName string, overwrite bool, header http.Header) (err error) {
	_, say := SetupLanguage()
	ctx := context.Background()

	r, err := repo.NewFS(minerRepoPath)
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

	// Copy over identical settings:

	buf, err := os.ReadFile(path.Join(lr.Path(), "config.toml"))
	if err != nil {
		return fmt.Errorf("could not read config.toml: %w", err)
	}
	var curioCfg config.CurioConfig
	_, err = toml.Decode(string(buf), &curioCfg)
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
		return xerrors.Errorf("keystore err: %w", err)
	}
	js, err := ks.Get(modules.JWTSecretName)
	if err != nil {
		return xerrors.Errorf("error getting JWTSecretName: %w", err)
	}

	curioCfg.Apis.StorageRPCSecret = base64.StdEncoding.EncodeToString(js.PrivateKey)

	ainfo, err := cliutil.GetAPIInfo(&cli.Context{}, repo.FullNode)
	if err != nil {
		return xerrors.Errorf(`could not get API info for FullNode: %w
		Set the environment variable to the value of "lotus auth api-info --perm=admin"`, err)
	}
	curioCfg.Apis.ChainApiInfo = []string{header.Get("Authorization")[7:] + ":" + ainfo.Addr}

	// Express as configTOML
	configTOML := &bytes.Buffer{}
	if err = toml.NewEncoder(configTOML).Encode(curioCfg); err != nil {
		return err
	}

	if !lo.Contains(titles, "base") { // TODO REMOVE THIS. INSTEAD ADD A BASE or append to addresses
		cfg, err := deps.GetDefaultConfig(true)
		if err != nil {
			return xerrors.Errorf("Cannot get default config: %w", err)
		}
		_, err = db.Exec(ctx, "INSERT INTO harmony_config (title, config) VALUES ('base', $1)", cfg)

		if err != nil {
			return err
		}
	}
	if layerName == "" { // only make mig if base exists and we are different. // compare to base.
		layerName = fmt.Sprintf("mig-%s", curioCfg.Addresses[0].MinerAddresses[0])
	} else {
		if lo.Contains(titles, layerName) && !overwrite {
			return errors.New("the overwrite flag is needed to replace existing layer: " + layerName)
		}
	}
	say(plain, "Layer %s created. ", layerName)
	if overwrite {
		_, err := db.Exec(ctx, "DELETE FROM harmony_config WHERE title=$1", layerName)
		if err != nil {
			return err
		}
	}

	_, err = db.Exec(ctx, "INSERT INTO harmony_config (title, config) VALUES ($1, $2)", layerName, configTOML.String())
	if err != nil {
		return err
	}

	dbSettings := getDBSettings(*smCfg)
	say(plain, `To work with the config:`)
	say(code, `curio `+dbSettings+` config edit `+layerName)
	say(plain, `To run Curio: in its own machine or cgroup without other files, use the command:`)
	say(code, `curio `+dbSettings+` run --layer=`+layerName+`,post`)
	return nil
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
