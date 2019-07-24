package modules

import (
	"context"
	"crypto/rand"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipfs/go-filestore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	"github.com/ipfs/go-merkledag"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/routing"
	record "github.com/libp2p/go-libp2p-record"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/filecoin-project/go-lotus/node/repo"
)

var log = logging.Logger("modules")

type Genesis func() (*chain.BlockHeader, error)

// RecordValidator provides namesys compatible routing record validator
func RecordValidator(ps peerstore.Peerstore) record.Validator {
	return record.NamespacedValidator{
		"pk": record.PublicKeyValidator{},
	}
}

func Bitswap(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, rt routing.Routing, bs blockstore.GCBlockstore) exchange.Interface {
	bitswapNetwork := network.NewFromIpfsHost(host, rt)
	exch := bitswap.New(helpers.LifecycleCtx(mctx, lc), bitswapNetwork, bs)
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return exch.Close()
		},
	})
	return exch
}

func SetGenesis(cs *chain.ChainStore, g Genesis) error {
	_, err := cs.GetGenesis()
	if err == nil {
		return nil // already set, noop
	}
	if err != datastore.ErrNotFound {
		return err
	}

	genesis, err := g()
	if err != nil {
		return err
	}

	return cs.SetGenesis(genesis)
}

func LockedRepo(lr repo.LockedRepo) func(lc fx.Lifecycle) repo.LockedRepo {
	return func(lc fx.Lifecycle) repo.LockedRepo {
		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				return lr.Close()
			},
		})

		return lr
	}
}

func KeyStore(lr repo.LockedRepo) (types.KeyStore, error) {
	return lr.KeyStore()
}

const JWTSecretName = "auth-jwt-private"

type APIAlg jwt.HMACSHA

type jwtPayload struct {
	Allow []string
}

func APISecret(keystore types.KeyStore, lr repo.LockedRepo) (*APIAlg, error) {
	key, err := keystore.Get(JWTSecretName)
	if err != nil {
		log.Warn("Generating new API secret")

		sk, err := ioutil.ReadAll(io.LimitReader(rand.Reader, 32))
		if err != nil {
			return nil, err
		}

		key = types.KeyInfo{
			Type:       "jwt-hmac-secret",
			PrivateKey: sk,
		}

		if err := keystore.Put(JWTSecretName, key); err != nil {
			return nil, xerrors.Errorf("writing API secret: %w", err)
		}

		// TODO: make this configurable
		p := jwtPayload{
			Allow: api.AllPermissions,
		}

		cliToken, err := jwt.Sign(&p, jwt.NewHS256(key.PrivateKey))
		if err != nil {
			return nil, err
		}

		if err := lr.SetAPIToken(cliToken); err != nil {
			return nil, err
		}
	}

	return (*APIAlg)(jwt.NewHS256(key.PrivateKey)), nil
}

func Datastore(r repo.LockedRepo) (datastore.Batching, error) {
	return r.Datastore("/metadata")
}

func Blockstore(r repo.LockedRepo) (blockstore.Blockstore, error) {
	blocks, err := r.Datastore("/blocks")
	if err != nil {
		return nil, err
	}

	bs := blockstore.NewBlockstore(blocks)
	return blockstore.NewIdStore(bs), nil
}

func ClientFstore(r repo.LockedRepo) (*filestore.Filestore, error) {
	clientds, err := r.Datastore("/client")
	if err != nil {
		return nil, err
	}
	blocks := namespace.Wrap(clientds, datastore.NewKey("blocks"))

	fm := filestore.NewFileManager(clientds, filepath.Dir(r.Path()))
	fm.AllowFiles = true
	// TODO: fm.AllowUrls (needs more code in client import)

	bs := blockstore.NewBlockstore(blocks)
	return filestore.NewFilestore(bs, fm), nil
}

func ClientDAG(lc fx.Lifecycle, fstore *filestore.Filestore) ipld.DAGService {
	ibs := blockstore.NewIdStore(fstore)
	bsvc := blockservice.New(ibs, offline.Exchange(ibs))
	dag := merkledag.NewDAGService(bsvc)

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return bsvc.Close()
		},
	})

	return dag
}

func ChainStore(lc fx.Lifecycle, bs blockstore.Blockstore, ds datastore.Batching) *chain.ChainStore {
	chain := chain.NewChainStore(bs, ds)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return chain.Load()
		},
	})

	return chain
}

func ErrorGenesis() Genesis {
	return func() (header *chain.BlockHeader, e error) {
		return nil, xerrors.New("No genesis block provided")
	}
}

func LoadGenesis(genBytes []byte) func() Genesis {
	return func() Genesis {
		return func() (header *chain.BlockHeader, e error) {
			return chain.DecodeBlock(genBytes)
		}
	}
}
