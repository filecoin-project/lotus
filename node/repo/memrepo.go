package repo

import (
	"sync"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
)

type MemRepo struct {
	api struct {
		sync.Mutex
		ma    multiaddr.Multiaddr
		token []byte
	}

	repoLock chan struct{}
	token    *byte

	datastore datastore.Datastore
	configF   func() *config.Root
	keystore  map[string]types.KeyInfo
}

type lockedMemRepo struct {
	mem *MemRepo
	sync.RWMutex

	token *byte
}

func (lmem *lockedMemRepo) Path() string {
	return ""
}

var _ Repo = &MemRepo{}

// MemRepoOptions contains options for memory repo
type MemRepoOptions struct {
	Ds       datastore.Datastore
	ConfigF  func() *config.Root
	KeyStore map[string]types.KeyInfo
}

// NewMemory creates new memory based repo with provided options.
// opts can be nil, it  will be replaced with defaults.
// Any field in opts can be nil, they will be replaced by defaults.
func NewMemory(opts *MemRepoOptions) *MemRepo {
	if opts == nil {
		opts = &MemRepoOptions{}
	}
	if opts.ConfigF == nil {
		opts.ConfigF = config.Default
	}
	if opts.Ds == nil {
		opts.Ds = dssync.MutexWrap(datastore.NewMapDatastore())
	}
	if opts.KeyStore == nil {
		opts.KeyStore = make(map[string]types.KeyInfo)
	}

	return &MemRepo{
		repoLock: make(chan struct{}, 1),

		datastore: opts.Ds,
		configF:   opts.ConfigF,
		keystore:  opts.KeyStore,
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

func (mem *MemRepo) Lock() (LockedRepo, error) {
	select {
	case mem.repoLock <- struct{}{}:
	default:
		return nil, ErrRepoAlreadyLocked
	}
	mem.token = new(byte)

	return &lockedMemRepo{
		mem:   mem,
		token: mem.token,
	}, nil
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

func (lmem *lockedMemRepo) Datastore(ns string) (datastore.Batching, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}

	return namespace.Wrap(lmem.mem.datastore, datastore.NewKey(ns)), nil
}

func (lmem *lockedMemRepo) Config() (*config.Root, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}
	return lmem.mem.configF(), nil
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

// Get gets a key out of keystore and returns types.KeyInfo coresponding to named key
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
		return xerrors.Errorf("putting key '%s': %w", name, ErrKeyExists)
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
