package repo

import (
	"errors"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/config"
)

var (
	ErrNoAPIEndpoint     = errors.New("API not running (no endpoint)")
	ErrRepoAlreadyLocked = errors.New("repo is already locked")
	ErrClosedRepo        = errors.New("repo is no longer open")

	ErrKeyExists   = errors.New("key already exists")
	ErrKeyNotFound = errors.New("key not found")
)

type Repo interface {
	// APIEndpoint returns multiaddress for communication with Lotus API
	APIEndpoint() (multiaddr.Multiaddr, error)

	// Lock locks the repo for exclusive use.
	Lock() (LockedRepo, error)
}

type LockedRepo interface {
	// Close closes repo and removes lock.
	Close() error

	// Returns datastore defined in this repo.
	Datastore(namespace string) (datastore.Batching, error)

	// Returns config in this repo
	Config() (*config.Root, error)

	// Libp2pIdentity returns private key for libp2p indentity
	Libp2pIdentity() (crypto.PrivKey, error)

	// SetAPIEndpoint sets the endpoint of the current API
	// so it can be read by API clients
	SetAPIEndpoint(multiaddr.Multiaddr) error

	// KeyStore returns store of private keys for Filecoin transactions
	KeyStore() (types.KeyStore, error)

	// Path returns absolute path of the repo (or empty string if in-memory)
	Path() string
}
