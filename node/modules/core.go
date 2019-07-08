package modules

import (
	"context"

	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/routing"
	record "github.com/libp2p/go-libp2p-record"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
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
