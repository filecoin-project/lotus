package repo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/ipfs/go-datastore"
	fslock "github.com/ipfs/go-fs-lock"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/multiformats/go-base32"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	badgerbs "github.com/filecoin-project/lotus/blockstore/badger"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/system"
)

const (
	fsAPI           = "api"
	fsAPIToken      = "token"
	fsConfig        = "config.toml"
	fsStorageConfig = "storage.json"
	fsDatastore     = "datastore"
	fsLock          = "repo.lock"
	fsKeystore      = "keystore"
	fsChainIndex    = "chainindex"
)

func NewRepoTypeFromString(t string) RepoType {
	switch t {
	case "FullNode":
		return FullNode
	case "StorageMiner":
		return StorageMiner
	case "Worker":
		return Worker
	case "Wallet":
		return Wallet
	default:
		panic("unknown RepoType")
	}
}

type RepoType interface {
	Type() string
	Config() interface{}

	// APIFlags returns flags passed on the command line with the listen address
	// of the API server (only used by the tests), in the order of precedence they
	// should be applied for the requested kind of node.
	APIFlags() []string

	RepoFlags() []string

	// APIInfoEnvVars returns the environment variables to use in order of precedence
	// to determine the API endpoint of the specified node type.
	//
	// It returns the current variables and deprecated ones separately, so that
	// the user can log a warning when deprecated ones are found to be in use.
	APIInfoEnvVars() (string, []string, []string)
}

var FullNode fullNode

type fullNode struct {
}

func (fullNode) Type() string {
	return "FullNode"
}

func (fullNode) Config() interface{} {
	return config.DefaultFullNode()
}

func (fullNode) APIFlags() []string {
	return []string{"api-url"}
}

func (fullNode) RepoFlags() []string {
	return []string{"repo"}
}

func (fullNode) APIInfoEnvVars() (primary string, fallbacks []string, deprecated []string) {
	return "FULLNODE_API_INFO", nil, nil
}

var StorageMiner storageMiner

type storageMiner struct{}

func (storageMiner) Type() string {
	return "StorageMiner"
}

func (storageMiner) Config() interface{} {
	return config.DefaultStorageMiner()
}

func (storageMiner) APIFlags() []string {
	return []string{"miner-api-url"}
}

func (storageMiner) RepoFlags() []string {
	return []string{"miner-repo"}
}

func (storageMiner) APIInfoEnvVars() (primary string, fallbacks []string, deprecated []string) {
	// TODO remove deprecated deprecation period
	return "MINER_API_INFO", nil, []string{"STORAGE_API_INFO"}
}

type worker struct {
}

var Worker worker

func (worker) Type() string {
	return "Worker"
}

func (worker) Config() interface{} {
	return &struct{}{}
}

func (worker) APIFlags() []string {
	return []string{"worker-api-url"}
}

func (worker) RepoFlags() []string {
	return []string{"worker-repo"}
}

func (worker) APIInfoEnvVars() (primary string, fallbacks []string, deprecated []string) {
	return "WORKER_API_INFO", nil, nil
}

var Wallet wallet

type wallet struct {
}

func (wallet) Type() string {
	return "Wallet"
}

func (wallet) Config() interface{} {
	return &struct{}{}
}

func (wallet) APIFlags() []string {
	panic("not supported")
}

func (wallet) RepoFlags() []string {
	panic("not supported")
}

func (wallet) APIInfoEnvVars() (primary string, fallbacks []string, deprecated []string) {
	panic("not supported")
}

var log = logging.Logger("repo")

var ErrRepoExists = xerrors.New("repo exists")

// FsRepo is struct for repo, use NewFS to create
type FsRepo struct {
	path       string
	configPath string
}

var _ Repo = &FsRepo{}

// NewFS creates a repo instance based on a path on file system
func NewFS(path string) (*FsRepo, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, err
	}

	return &FsRepo{
		path:       path,
		configPath: filepath.Join(path, fsConfig),
	}, nil
}

func (fsr *FsRepo) SetConfigPath(cfgPath string) {
	fsr.configPath = cfgPath
}

func (fsr *FsRepo) Exists() (bool, error) {
	_, err := os.Stat(filepath.Join(fsr.path, fsDatastore))
	notexist := os.IsNotExist(err)
	if notexist {
		err = nil

		_, err = os.Stat(filepath.Join(fsr.path, fsKeystore))
		notexist = os.IsNotExist(err)
		if notexist {
			err = nil
		}
	}
	return !notexist, err
}

func (fsr *FsRepo) Init(t RepoType) error {
	exist, err := fsr.Exists()
	if err != nil {
		return err
	}
	if exist {
		return nil
	}

	log.Infof("Initializing repo at '%s'", fsr.path)
	err = os.MkdirAll(fsr.path, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	if err := fsr.initConfig(t); err != nil {
		return xerrors.Errorf("init config: %w", err)
	}

	return fsr.initKeystore()

}

func (fsr *FsRepo) initConfig(t RepoType) error {
	_, err := os.Stat(fsr.configPath)
	if err == nil {
		// exists
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	c, err := os.Create(fsr.configPath)
	if err != nil {
		return err
	}

	comm, err := config.ConfigComment(t.Config())
	if err != nil {
		return xerrors.Errorf("comment: %w", err)
	}
	_, err = c.Write(comm)
	if err != nil {
		return xerrors.Errorf("write config: %w", err)
	}

	if err := c.Close(); err != nil {
		return xerrors.Errorf("close config: %w", err)
	}
	return nil
}

func (fsr *FsRepo) initKeystore() error {
	kstorePath := filepath.Join(fsr.path, fsKeystore)
	if _, err := os.Stat(kstorePath); err == nil {
		return ErrRepoExists
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Mkdir(kstorePath, 0700)
}

// APIEndpoint returns endpoint of API in this repo
func (fsr *FsRepo) APIEndpoint() (multiaddr.Multiaddr, error) {
	p := filepath.Join(fsr.path, fsAPI)

	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, xerrors.Errorf("No file (%s): %w", p, ErrNoAPIEndpoint)
	} else if err != nil {
		return nil, err
	}
	defer f.Close() //nolint: errcheck // Read only op

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, xerrors.Errorf("failed to read %q: %w", p, err)
	}
	strma := string(data)
	strma = strings.TrimSpace(strma)

	apima, err := multiaddr.NewMultiaddr(strma)
	if err != nil {
		return nil, err
	}
	return apima, nil
}

func (fsr *FsRepo) APIToken() ([]byte, error) {
	p := filepath.Join(fsr.path, fsAPIToken)
	f, err := os.Open(p)

	if os.IsNotExist(err) {
		return nil, ErrNoAPIEndpoint
	} else if err != nil {
		return nil, err
	}
	defer f.Close() //nolint: errcheck // Read only op

	tb, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return bytes.TrimSpace(tb), nil
}

// Lock acquires exclusive lock on this repo
func (fsr *FsRepo) Lock(repoType RepoType) (LockedRepo, error) {
	locked, err := fslock.Locked(fsr.path, fsLock)
	if err != nil {
		return nil, xerrors.Errorf("could not check lock status: %w", err)
	}
	if locked {
		return nil, ErrRepoAlreadyLocked
	}

	closer, err := fslock.Lock(fsr.path, fsLock)
	if err != nil {
		return nil, xerrors.Errorf("could not lock the repo: %w", err)
	}
	return &fsLockedRepo{
		path:       fsr.path,
		configPath: fsr.configPath,
		repoType:   repoType,
		closer:     closer,
	}, nil
}

// LockRO is like Lock, except datastores will work in read-only mode
func (fsr *FsRepo) LockRO(repoType RepoType) (LockedRepo, error) {
	lr, err := fsr.Lock(repoType)
	if err != nil {
		return nil, err
	}

	lr.(*fsLockedRepo).readonly = true
	return lr, nil
}

type fsLockedRepo struct {
	path       string
	configPath string
	repoType   RepoType
	closer     io.Closer
	readonly   bool

	ds     map[string]datastore.Batching
	dsErr  error
	dsOnce sync.Once

	bs     blockstore.Blockstore
	bsErr  error
	bsOnce sync.Once
	ssPath string
	ssErr  error
	ssOnce sync.Once

	chainIndexPath string
	chainIndexErr  error
	chainIndexOnce sync.Once

	storageLk sync.Mutex
	configLk  sync.Mutex
}

func (fsr *fsLockedRepo) RepoType() RepoType {
	return fsr.repoType
}

func (fsr *fsLockedRepo) Readonly() bool {
	return fsr.readonly
}

func (fsr *fsLockedRepo) Path() string {
	return fsr.path
}

func (fsr *fsLockedRepo) Close() error {
	err := os.Remove(fsr.join(fsAPI))

	if err != nil && !os.IsNotExist(err) {
		return xerrors.Errorf("could not remove API file: %w", err)
	}
	if fsr.ds != nil {
		for _, ds := range fsr.ds {
			if err := ds.Close(); err != nil {
				return xerrors.Errorf("could not close datastore: %w", err)
			}
		}
	}

	// type assertion will return ok=false if fsr.bs is nil altogether.
	if c, ok := fsr.bs.(io.Closer); ok && c != nil {
		if err := c.Close(); err != nil {
			return xerrors.Errorf("could not close blockstore: %w", err)
		}
	}

	err = fsr.closer.Close()
	fsr.closer = nil
	return err
}

// Blockstore returns a blockstore for the provided data domain.
func (fsr *fsLockedRepo) Blockstore(ctx context.Context, domain BlockstoreDomain) (blockstore.Blockstore, error) {
	if domain != UniversalBlockstore {
		return nil, ErrInvalidBlockstoreDomain
	}

	fsr.bsOnce.Do(func() {
		path := fsr.join(filepath.Join(fsDatastore, "chain"))
		readonly := fsr.readonly

		if err := os.MkdirAll(path, 0755); err != nil {
			fsr.bsErr = err
			return
		}

		opts, err := BadgerBlockstoreOptions(domain, path, readonly)
		if err != nil {
			fsr.bsErr = err
			return
		}

		if system.BadgerFsyncDisable {
			opts.SyncWrites = false
		}

		bs, err := badgerbs.Open(opts)
		if err != nil {
			fsr.bsErr = err
			return
		}
		fsr.bs = blockstore.WrapIDStore(bs)
	})

	return fsr.bs, fsr.bsErr
}

func (fsr *fsLockedRepo) SplitstorePath() (string, error) {
	fsr.ssOnce.Do(func() {
		path := fsr.join(filepath.Join(fsDatastore, "splitstore"))

		if err := os.MkdirAll(path, 0755); err != nil {
			fsr.ssErr = err
			return
		}

		fsr.ssPath = path
	})

	return fsr.ssPath, fsr.ssErr
}

func (fsr *fsLockedRepo) ChainIndexPath() (string, error) {
	fsr.chainIndexOnce.Do(func() {
		path := fsr.join(fsChainIndex)

		if err := os.MkdirAll(path, 0755); err != nil {
			fsr.chainIndexErr = err
			return
		}

		fsr.chainIndexPath = path
	})

	return fsr.chainIndexPath, fsr.chainIndexErr
}

// join joins path elements with fsr.path
func (fsr *fsLockedRepo) join(paths ...string) string {
	return filepath.Join(append([]string{fsr.path}, paths...)...)
}

func (fsr *fsLockedRepo) stillValid() error {
	if fsr.closer == nil {
		return ErrClosedRepo
	}
	return nil
}

func (fsr *fsLockedRepo) Config() (interface{}, error) {
	fsr.configLk.Lock()
	defer fsr.configLk.Unlock()

	return fsr.loadConfigFromDisk()
}

func (fsr *fsLockedRepo) loadConfigFromDisk() (interface{}, error) {
	var opts []config.LoadCfgOpt
	if fsr.repoType == FullNode {
		opts = append(opts, config.SetCanFallbackOnDefault(config.NoDefaultForSplitstoreTransition))
		opts = append(opts, config.SetValidate(config.ValidateSplitstoreSet))
	}
	opts = append(opts, config.SetDefault(func() (interface{}, error) {
		return fsr.repoType.Config(), nil
	}))
	return config.FromFile(fsr.configPath, opts...)
}

func (fsr *fsLockedRepo) SetConfig(c func(interface{})) error {
	if err := fsr.stillValid(); err != nil {
		return err
	}

	fsr.configLk.Lock()
	defer fsr.configLk.Unlock()

	cfg, err := fsr.loadConfigFromDisk()
	if err != nil {
		return err
	}

	// mutate in-memory representation of config
	c(cfg)

	// buffer into which we write TOML bytes
	buf := new(bytes.Buffer)

	// encode now-mutated config as TOML and write to buffer
	err = toml.NewEncoder(buf).Encode(cfg)
	if err != nil {
		return err
	}

	// write buffer of TOML bytes to config file
	err = os.WriteFile(fsr.configPath, buf.Bytes(), 0644)
	if err != nil {
		return err
	}

	return nil
}

func (fsr *fsLockedRepo) GetStorage() (storiface.StorageConfig, error) {
	fsr.storageLk.Lock()
	defer fsr.storageLk.Unlock()

	return fsr.getStorage(nil)
}

func (fsr *fsLockedRepo) getStorage(def *storiface.StorageConfig) (storiface.StorageConfig, error) {
	c, err := config.StorageFromFile(fsr.join(fsStorageConfig), def)
	if err != nil {
		return storiface.StorageConfig{}, err
	}
	return *c, nil
}

func (fsr *fsLockedRepo) SetStorage(c func(*storiface.StorageConfig)) error {
	fsr.storageLk.Lock()
	defer fsr.storageLk.Unlock()

	sc, err := fsr.getStorage(&storiface.StorageConfig{})
	if err != nil {
		return xerrors.Errorf("get storage: %w", err)
	}

	c(&sc)

	return config.WriteStorageFile(fsr.join(fsStorageConfig), sc)
}

func (fsr *fsLockedRepo) Stat(path string) (fsutil.FsStat, error) {
	return fsutil.Statfs(path)
}

func (fsr *fsLockedRepo) DiskUsage(path string) (int64, error) {
	si, err := fsutil.FileSize(path)
	if err != nil {
		return 0, err
	}
	return si.OnDisk, nil
}

func (fsr *fsLockedRepo) SetAPIEndpoint(ma multiaddr.Multiaddr) error {
	if err := fsr.stillValid(); err != nil {
		return err
	}
	return os.WriteFile(fsr.join(fsAPI), []byte(ma.String()), 0644)
}

func (fsr *fsLockedRepo) SetAPIToken(token []byte) error {
	if err := fsr.stillValid(); err != nil {
		return err
	}
	return os.WriteFile(fsr.join(fsAPIToken), token, 0600)
}

func (fsr *fsLockedRepo) KeyStore() (types.KeyStore, error) {
	if err := fsr.stillValid(); err != nil {
		return nil, err
	}
	return fsr, nil
}

var kstrPermissionMsg = "permissions of key: '%s' are too relaxed, " +
	"required: 0600, got: %#o"

// List lists all the keys stored in the KeyStore
func (fsr *fsLockedRepo) List() ([]string, error) {
	if err := fsr.stillValid(); err != nil {
		return nil, err
	}

	kstorePath := fsr.join(fsKeystore)
	dir, err := os.Open(kstorePath)
	if err != nil {
		return nil, xerrors.Errorf("opening dir to list keystore: %w", err)
	}
	defer dir.Close() //nolint:errcheck
	files, err := dir.Readdir(-1)
	if err != nil {
		return nil, xerrors.Errorf("reading keystore dir: %w", err)
	}
	keys := make([]string, 0, len(files))
	for _, f := range files {
		if f.Mode()&0077 != 0 {
			return nil, xerrors.Errorf(kstrPermissionMsg, f.Name(), f.Mode())
		}
		name, err := base32.RawStdEncoding.DecodeString(f.Name())
		if err != nil {
			return nil, xerrors.Errorf("decoding key: '%s': %w", f.Name(), err)
		}
		keys = append(keys, string(name))
	}
	return keys, nil
}

// Get gets a key out of keystore and returns types.KeyInfo corresponding to named key
func (fsr *fsLockedRepo) Get(name string) (types.KeyInfo, error) {
	if err := fsr.stillValid(); err != nil {
		return types.KeyInfo{}, err
	}

	encName := base32.RawStdEncoding.EncodeToString([]byte(name))
	keyPath := fsr.join(fsKeystore, encName)

	fstat, err := os.Stat(keyPath)
	if os.IsNotExist(err) {
		return types.KeyInfo{}, xerrors.Errorf("opening key '%s': %w", name, types.ErrKeyInfoNotFound)
	} else if err != nil {
		return types.KeyInfo{}, xerrors.Errorf("opening key '%s': %w", name, err)
	}

	if fstat.Mode()&0077 != 0 {
		return types.KeyInfo{}, xerrors.Errorf(kstrPermissionMsg, name, fstat.Mode())
	}

	file, err := os.Open(keyPath)
	if err != nil {
		return types.KeyInfo{}, xerrors.Errorf("opening key '%s': %w", name, err)
	}
	defer file.Close() //nolint: errcheck // read only op

	data, err := io.ReadAll(file)
	if err != nil {
		return types.KeyInfo{}, xerrors.Errorf("reading key '%s': %w", name, err)
	}

	var res types.KeyInfo
	err = json.Unmarshal(data, &res)
	if err != nil {
		return types.KeyInfo{}, xerrors.Errorf("decoding key '%s': %w", name, err)
	}

	return res, nil
}

const KTrashPrefix = "trash-"

// Put saves key info under given name
func (fsr *fsLockedRepo) Put(name string, info types.KeyInfo) error {
	return fsr.put(name, info, 0)
}

func (fsr *fsLockedRepo) put(rawName string, info types.KeyInfo, retries int) error {
	if err := fsr.stillValid(); err != nil {
		return err
	}

	name := rawName
	if retries > 0 {
		name = fmt.Sprintf("%s-%d", rawName, retries)
	}

	encName := base32.RawStdEncoding.EncodeToString([]byte(name))
	keyPath := fsr.join(fsKeystore, encName)

	_, err := os.Stat(keyPath)
	if err == nil && strings.HasPrefix(name, KTrashPrefix) {
		// retry writing the trash-prefixed file with a number suffix
		return fsr.put(rawName, info, retries+1)
	} else if err == nil {
		return xerrors.Errorf("checking key before put '%s': %w", name, types.ErrKeyExists)
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("checking key before put '%s': %w", name, err)
	}

	keyData, err := json.Marshal(info)
	if err != nil {
		return xerrors.Errorf("encoding key '%s': %w", name, err)
	}

	err = os.WriteFile(keyPath, keyData, 0600)
	if err != nil {
		return xerrors.Errorf("writing key '%s': %w", name, err)
	}
	return nil
}

func (fsr *fsLockedRepo) Delete(name string) error {
	if err := fsr.stillValid(); err != nil {
		return err
	}

	encName := base32.RawStdEncoding.EncodeToString([]byte(name))
	keyPath := fsr.join(fsKeystore, encName)

	_, err := os.Stat(keyPath)
	if os.IsNotExist(err) {
		return xerrors.Errorf("checking key before delete '%s': %w", name, types.ErrKeyInfoNotFound)
	} else if err != nil {
		return xerrors.Errorf("checking key before delete '%s': %w", name, err)
	}

	err = os.Remove(keyPath)
	if err != nil {
		return xerrors.Errorf("deleting key '%s': %w", name, err)
	}
	return nil
}
