package repo

import (
	"crypto/rand"
	"sync"

	"github.com/filecoin-project/go-lotus/node/config"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	crypto "github.com/libp2p/go-libp2p-crypto"
	"github.com/multiformats/go-multiaddr"
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

type MemRepoOptions struct {
	ds        datastore.Datastore
	configF   func() *config.Root
	libp2pKey crypto.PrivKey
	wallet    interface{}
}

func NewMemory(opts *MemRepoOptions) *MemRepo {
	if opts == nil {
		opts = &MemRepoOptions{}
	}
	if opts.configF == nil {
		opts.configF = config.Default
	}
	if opts.ds == nil {
		opts.ds = dssync.MutexWrap(datastore.NewMapDatastore())
	}
	if opts.libp2pKey == nil {
		pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			panic(err)
		}
		opts.libp2pKey = pk
	}

	return &MemRepo{
		repoLock: make(chan struct{}, 1),

		datastore: opts.ds,
		configF:   opts.configF,
		libp2pKey: opts.libp2pKey,
		wallet:    opts.wallet,
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
	return nil

}

func (lmem *lockedMemRepo) Datastore() (datastore.Datastore, error) {
	if err := lmem.checkToken(); err != nil {
		return nil, err
	}
	return lmem.mem.datastore, nil
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
