package repo

import (
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/node/config"
)

var (
	ErrNoAPIEndpoint     = xerrors.New("API not running (no endpoint)")
	ErrRepoAlreadyLocked = xerrors.New("repo is already locked")
	ErrClosedRepo        = xerrors.New("repo is no longer open")
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

	// Wallet returns store of private keys for Filecoin transactions
	Wallet() (interface{}, error)
}
