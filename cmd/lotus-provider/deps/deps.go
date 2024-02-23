// Package deps provides the dependencies for the lotus provider node.
package deps

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gbrlsnchs/jwt/v3"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/api"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/alerting"
	"github.com/filecoin-project/lotus/journal/fsjournal"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/resources/storagemgr"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/provider"
	"github.com/filecoin-project/lotus/provider/multictladdr"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("lotus-provider/deps")

func MakeDB(cctx *cli.Context) (*harmonydb.DB, error) {
	dbConfig := config.HarmonyDB{
		Username: cctx.String("db-user"),
		Password: cctx.String("db-password"),
		Hosts:    strings.Split(cctx.String("db-host"), ","),
		Database: cctx.String("db-name"),
		Port:     cctx.String("db-port"),
	}
	return harmonydb.NewFromConfig(dbConfig)
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
	Cfg        *config.LotusProviderConfig
	DB         *harmonydb.DB
	Full       api.FullNode
	Verif      storiface.Verifier
	LW         *sealer.LocalWorker
	As         *multictladdr.MultiAddressSelector
	Maddrs     map[dtypes.MinerAddress]bool
	Stor       *paths.Remote
	Si         *paths.DBIndex
	LocalStore *paths.Local
	LocalPaths *paths.BasicLocalStorage
	ListenAddr string
	StorageMgr *storagemgr.StorageMgr
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
			if err := r.Init(repo.Provider); err != nil {
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
		deps.As, err = provider.AddressSelector(deps.Cfg.Addresses)()
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
		deps.Full, fullCloser, err = cliutil.GetFullNodeAPIV1LotusProvider(cctx, cfgApiInfo)
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
		//  maybe with a lotus-provider specific abstraction. LocalWorker does persistent call tracking which we probably
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

	if deps.StorageMgr == nil {
		lo, err := deps.LocalStore.Local(ctx)
		if err != nil {
			return xerrors.Errorf("could not get local storage: %w", err)
		}
		deps.StorageMgr = storagemgr.New(lo)
	}
	fmt.Println("last line of populate")

	return nil
}

var oldAddresses = regexp.MustCompile("(?i)^\\[addresses\\]$")

func LoadConfigWithUpgrades(text string, lp *config.LotusProviderConfig) (toml.MetaData, error) {
	// allow migration from old config format that was limited to 1 wallet setup.
	newText := oldAddresses.ReplaceAllString(text, "[[addresses]]")

	if text != newText {
		log.Warnw("Upgraded config!", "old", text, "new", newText)
	}

	meta, err := toml.Decode(newText, &lp)
	return meta, err
}
func GetConfig(cctx *cli.Context, db *harmonydb.DB) (*config.LotusProviderConfig, error) {
	lp := config.DefaultLotusProvider()
	have := []string{}
	layers := cctx.StringSlice("layers")
	for _, layer := range layers {
		text := ""
		err := db.QueryRow(cctx.Context, `SELECT config FROM harmony_config WHERE title=$1`, layer).Scan(&text)
		if err != nil {
			if strings.Contains(err.Error(), sql.ErrNoRows.Error()) {
				return nil, fmt.Errorf("missing layer '%s' ", layer)
			}
			if layer == "base" {
				return nil, errors.New(`lotus-provider defaults to a layer named 'base'. 
				Either use 'migrate' command or edit a base.toml and upload it with: lotus-provider config set base.toml`)
			}
			return nil, fmt.Errorf("could not read layer '%s': %w", layer, err)
		}

		meta, err := LoadConfigWithUpgrades(text, lp)
		if err != nil {
			return lp, fmt.Errorf("could not read layer, bad toml %s: %w", layer, err)
		}
		for _, k := range meta.Keys() {
			have = append(have, strings.Join(k, " "))
		}
		log.Infow("Using layer", "layer", layer, "config", lp)
	}
	_ = have // FUTURE: verify that required fields are here.
	// If config includes 3rd-party config, consider JSONSchema as a way that
	// 3rd-parties can dynamically include config requirements and we can
	// validate the config. Because of layering, we must validate @ startup.
	return lp, nil
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

	full, fullCloser, err := cliutil.GetFullNodeAPIV1LotusProvider(cctx, cfg.Apis.ChainApiInfo)
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
