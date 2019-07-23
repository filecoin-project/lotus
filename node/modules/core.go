package modules

import (
	"context"
	"github.com/gbrlsnchs/jwt/v3"
	"golang.org/x/xerrors"
	"path/filepath"

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

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
	"github.com/filecoin-project/go-lotus/node/repo"
)

var log = logging.Logger("modules")

type Genesis *chain.BlockHeader

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
	return cs.SetGenesis(g)
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

func APISecret(keystore types.KeyStore) (*APIAlg, error) {
	key, err := keystore.Get(JWTSecretName)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get JWT secret: %w", err)
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
