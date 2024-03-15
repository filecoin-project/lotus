// Package deps provides the dependencies for the curio node.
package deps

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gbrlsnchs/jwt/v3"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/api"
	curio "github.com/filecoin-project/lotus/curiosrc"
	"github.com/filecoin-project/lotus/curiosrc/multictladdr"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/alerting"
	"github.com/filecoin-project/lotus/journal/fsjournal"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("curio/deps")

func MakeDB(cctx *cli.Context) (*harmonydb.DB, error) {
	// #1 CLI opts
	fromCLI := func() (*harmonydb.DB, error) {
		dbConfig := config.HarmonyDB{
			Username: cctx.String("db-user"),
			Password: cctx.String("db-password"),
			Hosts:    strings.Split(cctx.String("db-host"), ","),
			Database: cctx.String("db-name"),
			Port:     cctx.String("db-port"),
		}
		return harmonydb.NewFromConfig(dbConfig)
	}

	readToml := func(path string) (*harmonydb.DB, error) {
		cfg, err := config.FromFile(path)
		if err != nil {
			return nil, err
		}
		if c, ok := cfg.(*config.StorageMiner); ok {
			return harmonydb.NewFromConfig(c.HarmonyDB)
		}
		return nil, errors.New("not a miner config")
	}

	// #2 Try local miner config
	fromMinerEnv := func() (*harmonydb.DB, error) {
		v := os.Getenv("LOTUS_MINER_PATH")
		if v == "" {
			return nil, errors.New("no miner env")
		}
		return readToml(filepath.Join(v, "config.toml"))

	}

	fromMiner := func() (*harmonydb.DB, error) {
		u, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		return readToml(filepath.Join(u, ".lotusminer/config.toml"))
	}
	fromEnv := func() (*harmonydb.DB, error) {
		// #3 Try env
		u, err := url.Parse(os.Getenv("CURIO_DB"))
		if err != nil {
			return nil, errors.New("no db connection string found in CURIO_DB env")
		}
		cfg := config.DefaultStorageMiner().HarmonyDB
		if u.User.Username() != "" {
			cfg.Username = u.User.Username()
		}
		if p, ok := u.User.Password(); ok && p != "" {
			cfg.Password = p
		}
		if u.Hostname() != "" {
			cfg.Hosts = []string{u.Hostname()}
		}
		if u.Port() != "" {
			cfg.Port = u.Port()
		}
		if strings.TrimPrefix(u.Path, "/") != "" {
			cfg.Database = strings.TrimPrefix(u.Path, "/")
		}

		return harmonydb.NewFromConfig(cfg)
	}

	for _, f := range []func() (*harmonydb.DB, error){fromCLI, fromMinerEnv, fromMiner, fromEnv} {
		db, err := f()
		if err != nil {
			continue
		}
		return db, nil
	}
	log.Error("No db connection string found. User CLI args or env var: set CURIO_DB=postgres://USER:PASSWORD@HOST:PORT/DATABASE")
	return fromCLI() //in-case it's not about bad config.
}

type JwtPayload struct {
	Allow []auth.Permission
}

func StorageAuth(apiKey string) (sealer.StorageAuth, error) {
	if apiKey == "" {
		return nil, xerrors.Errorf("no api key provided")
	}

	rawKey, err := base64.StdEncoding.DecodeString(apiKey)
	if err != nil {
		return nil, xerrors.Errorf("decoding api key: %w", err)
	}

	key := jwt.NewHS256(rawKey)

	p := JwtPayload{
		Allow: []auth.Permission{"admin"},
	}

	token, err := jwt.Sign(&p, key)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))
	return sealer.StorageAuth(headers), nil
}

func GetDeps(ctx context.Context, cctx *cli.Context) (*Deps, error) {
	var deps Deps
	return &deps, deps.PopulateRemainingDeps(ctx, cctx, true)
}

type Deps struct {
	Cfg        *config.CurioConfig // values
	DB         *harmonydb.DB       // has itest capability
	Full       api.FullNode
	Verif      storiface.Verifier
	LW         *sealer.LocalWorker
	As         *multictladdr.MultiAddressSelector
	Maddrs     map[dtypes.MinerAddress]bool
	ProofTypes map[abi.RegisteredSealProof]bool
	Stor       *paths.Remote
	Si         *paths.DBIndex
	LocalStore *paths.Local
	LocalPaths *paths.BasicLocalStorage
	ListenAddr string
}

const (
	FlagRepoPath = "repo-path"
)

func (deps *Deps) PopulateRemainingDeps(ctx context.Context, cctx *cli.Context, makeRepo bool) error {
	var err error
	if makeRepo {
		// Open repo
		repoPath := cctx.String(FlagRepoPath)
		fmt.Println("repopath", repoPath)
		r, err := repo.NewFS(repoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if !ok {
			if err := r.Init(repo.Curio); err != nil {
				return err
			}
		}
	}

	if deps.Cfg == nil {
		deps.DB, err = MakeDB(cctx)
		if err != nil {
			return err
		}
	}

	if deps.Cfg == nil {
		// The config feeds into task runners & their helpers
		deps.Cfg, err = GetConfig(cctx, deps.DB)
		if err != nil {
			return xerrors.Errorf("populate config: %w", err)
		}
	}

	log.Debugw("config", "config", deps.Cfg)

	if deps.Verif == nil {
		deps.Verif = ffiwrapper.ProofVerifier
	}

	if deps.As == nil {
		deps.As, err = curio.AddressSelector(deps.Cfg.Addresses)()
		if err != nil {
			return err
		}
	}

	if deps.Si == nil {
		de, err := journal.ParseDisabledEvents(deps.Cfg.Journal.DisabledEvents)
		if err != nil {
			return err
		}
		j, err := fsjournal.OpenFSJournalPath(cctx.String("journal"), de)
		if err != nil {
			return err
		}
		go func() {
			<-ctx.Done()
			_ = j.Close()
		}()

		al := alerting.NewAlertingSystem(j)
		deps.Si = paths.NewDBIndex(al, deps.DB)
	}

	if deps.Full == nil {
		var fullCloser func()
		cfgApiInfo := deps.Cfg.Apis.ChainApiInfo
		if v := os.Getenv("FULLNODE_API_INFO"); v != "" {
			cfgApiInfo = []string{v}
		}
		deps.Full, fullCloser, err = getFullNodeAPIV1Curio(cctx, cfgApiInfo)
		if err != nil {
			return err
		}

		go func() {
			<-ctx.Done()
			fullCloser()
		}()
	}

	deps.LocalPaths = &paths.BasicLocalStorage{
		PathToJSON: cctx.String("storage-json"),
	}

	if deps.ListenAddr == "" {
		listenAddr := cctx.String("listen")
		const unspecifiedAddress = "0.0.0.0"
		addressSlice := strings.Split(listenAddr, ":")
		if ip := net.ParseIP(addressSlice[0]); ip != nil {
			if ip.String() == unspecifiedAddress {
				rip, err := deps.DB.GetRoutableIP()
				if err != nil {
					return err
				}
				deps.ListenAddr = rip + ":" + addressSlice[1]
			}
		}
	}
	if deps.LocalStore == nil {
		deps.LocalStore, err = paths.NewLocal(ctx, deps.LocalPaths, deps.Si, []string{"http://" + deps.ListenAddr + "/remote"})
		if err != nil {
			return err
		}
	}

	sa, err := StorageAuth(deps.Cfg.Apis.StorageRPCSecret)
	if err != nil {
		return xerrors.Errorf(`'%w' while parsing the config toml's 
	[Apis]
	StorageRPCSecret=%v
Get it with: jq .PrivateKey ~/.lotus-miner/keystore/MF2XI2BNNJ3XILLQOJUXMYLUMU`, err, deps.Cfg.Apis.StorageRPCSecret)
	}
	if deps.Stor == nil {
		deps.Stor = paths.NewRemote(deps.LocalStore, deps.Si, http.Header(sa), 10, &paths.DefaultPartialFileHandler{})
	}
	if deps.LW == nil {
		wstates := statestore.New(dssync.MutexWrap(ds.NewMapDatastore()))

		// todo localWorker isn't the abstraction layer we want to use here, we probably want to go straight to ffiwrapper
		//  maybe with a curio specific abstraction. LocalWorker does persistent call tracking which we probably
		//  don't need (ehh.. maybe we do, the async callback system may actually work decently well with harmonytask)
		deps.LW = sealer.NewLocalWorker(sealer.WorkerConfig{
			MaxParallelChallengeReads: deps.Cfg.Proving.ParallelCheckLimit,
		}, deps.Stor, deps.LocalStore, deps.Si, nil, wstates)
	}
	if deps.Maddrs == nil {
		deps.Maddrs = map[dtypes.MinerAddress]bool{}
	}
	if len(deps.Maddrs) == 0 {
		for _, s := range deps.Cfg.Addresses {
			for _, s := range s.MinerAddresses {
				addr, err := address.NewFromString(s)
				if err != nil {
					return err
				}
				deps.Maddrs[dtypes.MinerAddress(addr)] = true
			}
		}
	}

	if deps.ProofTypes == nil {
		deps.ProofTypes = map[abi.RegisteredSealProof]bool{}
	}
	if len(deps.ProofTypes) == 0 {
		for maddr := range deps.Maddrs {
			spt, err := modules.SealProofType(maddr, deps.Full)
			if err != nil {
				return err
			}
			deps.ProofTypes[spt] = true
		}
	}

	return nil
}

func LoadConfigWithUpgrades(text string, curioConfigWithDefaults *config.CurioConfig) (toml.MetaData, error) {
	// allow migration from old config format that was limited to 1 wallet setup.
	newText := strings.Join(lo.Map(strings.Split(text, "\n"), func(line string, _ int) string {
		if strings.EqualFold(line, "[addresses]") {
			return "[[addresses]]"
		}
		return line
	}), "\n")
	meta, err := toml.Decode(newText, &curioConfigWithDefaults)
	for i := range curioConfigWithDefaults.Addresses {
		if curioConfigWithDefaults.Addresses[i].PreCommitControl == nil {
			curioConfigWithDefaults.Addresses[i].PreCommitControl = []string{}
		}
		if curioConfigWithDefaults.Addresses[i].CommitControl == nil {
			curioConfigWithDefaults.Addresses[i].CommitControl = []string{}
		}
		if curioConfigWithDefaults.Addresses[i].TerminateControl == nil {
			curioConfigWithDefaults.Addresses[i].TerminateControl = []string{}
		}
	}
	return meta, err
}
func GetConfig(cctx *cli.Context, db *harmonydb.DB) (*config.CurioConfig, error) {
	curioConfig := config.DefaultCurioConfig()
	have := []string{}
	layers := append([]string{"base"}, cctx.StringSlice("layers")...) // Always stack on top of "base" layer
	for _, layer := range layers {
		text := ""
		err := db.QueryRow(cctx.Context, `SELECT config FROM harmony_config WHERE title=$1`, layer).Scan(&text)
		if err != nil {
			if strings.Contains(err.Error(), sql.ErrNoRows.Error()) {
				return nil, fmt.Errorf("missing layer '%s' ", layer)
			}
			if layer == "base" {
				return nil, errors.New(`curio defaults to a layer named 'base'. 
				Either use 'migrate' command or edit a base.toml and upload it with: curio config set base.toml`)
			}
			return nil, fmt.Errorf("could not read layer '%s': %w", layer, err)
		}

		meta, err := LoadConfigWithUpgrades(text, curioConfig)
		if err != nil {
			return curioConfig, fmt.Errorf("could not read layer, bad toml %s: %w", layer, err)
		}
		for _, k := range meta.Keys() {
			have = append(have, strings.Join(k, " "))
		}
		log.Infow("Using layer", "layer", layer, "config", curioConfig)
	}
	_ = have // FUTURE: verify that required fields are here.
	// If config includes 3rd-party config, consider JSONSchema as a way that
	// 3rd-parties can dynamically include config requirements and we can
	// validate the config. Because of layering, we must validate @ startup.
	return curioConfig, nil
}

func GetDefaultConfig(comment bool) (string, error) {
	c := config.DefaultCurioConfig()
	cb, err := config.ConfigUpdate(c, nil, config.Commented(comment), config.DefaultKeepUncommented(), config.NoEnv())
	if err != nil {
		return "", err
	}
	return string(cb), nil
}

func GetDepsCLI(ctx context.Context, cctx *cli.Context) (*Deps, error) {
	db, err := MakeDB(cctx)
	if err != nil {
		return nil, err
	}

	cfg, err := GetConfig(cctx, db)
	if err != nil {
		return nil, err
	}

	full, fullCloser, err := getFullNodeAPIV1Curio(cctx, cfg.Apis.ChainApiInfo)
	if err != nil {
		return nil, err
	}
	go func() {
		select {
		case <-ctx.Done():
			fullCloser()
		}
	}()

	return &Deps{
		Cfg:  cfg,
		DB:   db,
		Full: full,
	}, nil
}
