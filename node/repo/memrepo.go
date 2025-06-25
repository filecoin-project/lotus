package repo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type MemRepo struct {
	api struct {
		sync.Mutex
		ma    multiaddr.Multiaddr
		token []byte
	}

	repoLock chan struct{}
	token    *byte

	datastore  datastore.Datastore
	keystore   map[string]types.KeyInfo
	blockstore blockstore.Blockstore

	sc      *storiface.StorageConfig
	tempDir string

	// holds the current config value
	config struct {
		sync.Mutex
		val interface{}
	}
}

type lockedMemRepo struct {
	mem *MemRepo
	t   RepoType
	sync.RWMutex

	token *byte
}

func (lmem *lockedMemRepo) RepoType() RepoType {
	return lmem.t
}

func (lmem *lockedMemRepo) GetStorage() (storiface.StorageConfig, error) {
	if err := lmem.checkToken(); err != nil {
		return storiface.StorageConfig{}, err
	}

	if lmem.mem.sc == nil {
		lmem.mem.sc = &storiface.StorageConfig{StoragePaths: []storiface.LocalPath{
			{Path: lmem.Path()},
		}}
	}

	return *lmem.mem.sc, nil
}

func (lmem *lockedMemRepo) SetStorage(c func(*storiface.StorageConfig)) error {
	if err := lmem.checkToken(); err != nil {
		return err
	}

	_, _ = lmem.GetStorage()

	c(lmem.mem.sc)
	return nil
}

func (lmem *lockedMemRepo) Stat(path string) (fsutil.FsStat, error) {
	return fsutil.Statfs(path)
}

func (lmem *lockedMemRepo) DiskUsage(path string) (int64, error) {
	si, err := fsutil.FileSize(path)
	if err != nil {
		return 0, err
	}
	return si.OnDisk, nil
}

func (lmem *lockedMemRepo) Path() string {
	lmem.Lock()
	defer lmem.Unlock()

	if lmem.mem.tempDir != "" {
		return lmem.mem.tempDir
	}

	t, err := os.MkdirTemp(os.TempDir(), "lotus-memrepo-temp-")
	if err != nil {
		panic(err) // only used in tests, probably fine
	}

	if lmem.t == StorageMiner || lmem.t == Worker {
		lmem.initSectorStore(t)
	}

	lmem.mem.tempDir = t
	return t
}

func (lmem *lockedMemRepo) initSectorStore(t string) {
	if err := config.WriteStorageFile(filepath.Join(t, fsStorageConfig), storiface.StorageConfig{
		StoragePaths: []storiface.LocalPath{
			{Path: t},
		}}); err != nil {
		panic(err)
	}

	b, err := json.MarshalIndent(&storiface.LocalStorageMeta{
		ID:       storiface.ID(uuid.New().String()),
		Weight:   10,
		CanSeal:  true,
		CanStore: true,
	}, "", "  ")
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(filepath.Join(t, "sectorstore.json"), b, 0644); err != nil {
		panic(err)
	}
}

var _ Repo = &MemRepo{}

// MemRepoOptions contains options for memory repo
type MemRepoOptions struct {
	Ds       datastore.Datastore
	KeyStore map[string]types.KeyInfo
}

// NewMemory creates new memory based repo with provided options.
// opts can be nil, it  will be replaced with defaults.
// Any field in opts can be nil, they will be replaced by defaults.
func NewMemory(opts *MemRepoOptions) *MemRepo {
	if opts == nil {
		opts = &MemRepoOptions{}
	}
	if opts.Ds == nil {
		opts.Ds = dssync.MutexWrap(datastore.NewMapDatastore())
	}
	if opts.KeyStore == nil {
		opts.KeyStore = make(map[string]types.KeyInfo)
	}

	return &MemRepo{
		repoLock:   make(chan struct{}, 1),
		blockstore: blockstore.WrapIDStore(blockstore.NewMemorySync()),
		datastore:  opts.Ds,
		keystore:   opts.KeyStore,
	}
}

func (mem *MemRepo) APIEndpoint() (multiaddr.Multiaddr, error) {
	mem.api.Lock()
	defer mem.api.Unlock()
	if mem.api.ma == nil {
		return nil, ErrNoAPIEndpoint
	}
	return mem.api.ma, nil
}

func (mem *MemRepo) APIToken() ([]byte, error) {
	mem.api.Lock()
	defer mem.api.Unlock()
	if mem.api.ma == nil {
		return nil, ErrNoAPIToken
	}
	return mem.api.token, nil
}

func (mem *MemRepo) Lock(t RepoType) (LockedRepo, error) {
	select {
	case mem.repoLock <- struct{}{}:
	default:
		return nil, ErrRepoAlreadyLocked
	}
	mem.token = new(byte)

	return &lockedMemRepo{
		mem:   mem,
		t:     t,
		token: mem.token,
	}, nil
}

func (mem *MemRepo) Cleanup() {
	mem.api.Lock()
	defer mem.api.Unlock()

	if mem.tempDir != "" {
		if err := os.RemoveAll(mem.tempDir); err != nil {
			log.Errorw("cleanup test memrepo", "error", err)
		}
		mem.tempDir = ""
	}
}

func (lmem *lockedMemRepo) Readonly() bool {
	return false
}

func (lmem *lockedMemRepo) checkToken() error {
	lmem.RLock()
	defer lmem.RUnlock()
	if lmem.mem.token != lmem.token {
		return ErrClosedRepo
	}
	return nil
}

func (lmem *lockedMemRepo) Close() error {
	if err := lmem.checkToken(); err != nil {
		return err
	}
	lmem.Lock()
	defer lmem.Unlock()

	if lmem.mem.token != lmem.token {
		return ErrClosedRepo
	}

	lmem.mem.token = nil
	lmem.mem.api.Lock()
	lmem.mem.api.ma = nil
	lmem.mem.api.Unlock()
	<-lmem.mem.repoLock // unlock
	return nil
}

func (lmem *lockedMemRepo) Datastore(_ context.Context, ns string) (datastore.Batching, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}

	return namespace.Wrap(lmem.mem.datastore, datastore.NewKey(ns)), nil
}

func (lmem *lockedMemRepo) Blockstore(ctx context.Context, domain BlockstoreDomain) (blockstore.Blockstore, error) {
	if domain != UniversalBlockstore {
		return nil, ErrInvalidBlockstoreDomain
	}
	return lmem.mem.blockstore, nil
}

func (lmem *lockedMemRepo) SplitstorePath() (string, error) {
	splitstorePath := filepath.Join(lmem.Path(), "splitstore")
	if err := os.MkdirAll(splitstorePath, 0755); err != nil {
		return "", err
	}
	return splitstorePath, nil
}

func (lmem *lockedMemRepo) ChainIndexPath() (string, error) {
	chainIndexPath := filepath.Join(lmem.Path(), "chainindex")
	if err := os.MkdirAll(chainIndexPath, 0755); err != nil {
		return "", err
	}
	return chainIndexPath, nil
}

func (lmem *lockedMemRepo) ListDatastores(ns string) ([]int64, error) {
	return nil, nil
}

func (lmem *lockedMemRepo) DeleteDatastore(ns string) error {
	/** poof **/
	return nil
}

func (lmem *lockedMemRepo) Config() (interface{}, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}

	lmem.mem.config.Lock()
	defer lmem.mem.config.Unlock()

	if lmem.mem.config.val == nil {
		lmem.mem.config.val = lmem.t.Config()
	}

	return lmem.mem.config.val, nil
}

func (lmem *lockedMemRepo) SetConfig(c func(interface{})) error {
	if err := lmem.checkToken(); err != nil {
		return err
	}

	lmem.mem.config.Lock()
	defer lmem.mem.config.Unlock()

	if lmem.mem.config.val == nil {
		lmem.mem.config.val = lmem.t.Config()
	}

	c(lmem.mem.config.val)

	return nil
}

func (lmem *lockedMemRepo) SetAPIEndpoint(ma multiaddr.Multiaddr) error {
	if err := lmem.checkToken(); err != nil {
		return err
	}
	lmem.mem.api.Lock()
	lmem.mem.api.ma = ma
	lmem.mem.api.Unlock()
	return nil
}

func (lmem *lockedMemRepo) SetAPIToken(token []byte) error {
	if err := lmem.checkToken(); err != nil {
		return err
	}
	lmem.mem.api.Lock()
	lmem.mem.api.token = token
	lmem.mem.api.Unlock()
	return nil
}

func (lmem *lockedMemRepo) KeyStore() (types.KeyStore, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}
	return lmem, nil
}

// Implement KeyStore on the same instance

// List lists all the keys stored in the KeyStore
func (lmem *lockedMemRepo) List() ([]string, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}
	lmem.RLock()
	defer lmem.RUnlock()

	res := make([]string, 0, len(lmem.mem.keystore))
	for k := range lmem.mem.keystore {
		res = append(res, k)
	}
	return res, nil
}

// Get gets a key out of keystore and returns types.KeyInfo corresponding to named key
func (lmem *lockedMemRepo) Get(name string) (types.KeyInfo, error) {
	if err := lmem.checkToken(); err != nil {
		return types.KeyInfo{}, err
	}
	lmem.RLock()
	defer lmem.RUnlock()

	key, ok := lmem.mem.keystore[name]
	if !ok {
		return types.KeyInfo{}, xerrors.Errorf("getting key '%s': %w", name, types.ErrKeyInfoNotFound)
	}
	return key, nil
}

// Put saves key info under given name
func (lmem *lockedMemRepo) Put(name string, key types.KeyInfo) error {
	if err := lmem.checkToken(); err != nil {
		return err
	}
	lmem.Lock()
	defer lmem.Unlock()

	_, isThere := lmem.mem.keystore[name]
	if isThere {
		return xerrors.Errorf("putting key '%s': %w", name, types.ErrKeyExists)
	}

	lmem.mem.keystore[name] = key
	return nil
}

func (lmem *lockedMemRepo) Delete(name string) error {
	if err := lmem.checkToken(); err != nil {
		return err
	}
	lmem.Lock()
	defer lmem.Unlock()

	_, isThere := lmem.mem.keystore[name]
	if !isThere {
		return xerrors.Errorf("deleting key '%s': %w", name, types.ErrKeyInfoNotFound)
	}
	delete(lmem.mem.keystore, name)
	return nil
}
