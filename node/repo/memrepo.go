package repo

import (
	"crypto/rand"
	"sync"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-lotus/node/config"
)

type MemRepo struct {
	api struct {
		sync.Mutex
		ma multiaddr.Multiaddr
	}

	repoLock chan struct{}
	token    *byte

	datastore datastore.Datastore
	configF   func() *config.Root
	libp2pKey crypto.PrivKey
	wallet    interface{}
}

type lockedMemRepo struct {
	mem *MemRepo
	sync.RWMutex

	token *byte
}

var _ Repo = &MemRepo{}

// MemRepoOptions contains options for memory repo
type MemRepoOptions struct {
	Ds        datastore.Datastore
	ConfigF   func() *config.Root
	Libp2pKey crypto.PrivKey
	Wallet    interface{}
}

func genLibp2pKey() (crypto.PrivKey, error) {
	pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	return pk, nil
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
	if opts.Libp2pKey == nil {
		pk, err := genLibp2pKey()
		if err != nil {
			panic(err)
		}
		opts.Libp2pKey = pk
	}

	return &MemRepo{
		repoLock: make(chan struct{}, 1),

		datastore: opts.Ds,
		configF:   opts.ConfigF,
		libp2pKey: opts.Libp2pKey,
		wallet:    opts.Wallet,
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

func (lmem *lockedMemRepo) Libp2pIdentity() (crypto.PrivKey, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}
	return lmem.mem.libp2pKey, nil
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

func (lmem *lockedMemRepo) Wallet() (interface{}, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}
	return lmem.mem.wallet, nil
}
