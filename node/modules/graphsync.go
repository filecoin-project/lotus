package modules

import (
	"io"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	graphsync "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/ipldbridge"
	gsnet "github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-graphsync/storeutil"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/host"
	"go.uber.org/fx"
)

// GraphsyncStorer creates a storer that stores data in the client blockstore
func GraphsyncStorer(clientBs dtypes.ClientBlockstore) dtypes.GraphsyncStorer {
	return dtypes.GraphsyncStorer(storeutil.StorerForBlockstore(clientBs))
}

// GraphsyncLoader creates a loader that reads from both the chain blockstore and the client blockstore
func GraphsyncLoader(clientBs dtypes.ClientBlockstore, chainBs dtypes.ChainBlockstore) dtypes.GraphsyncLoader {
	clientLoader := storeutil.LoaderForBlockstore(clientBs)
	chainLoader := storeutil.LoaderForBlockstore(chainBs)
	return func(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error) {
		reader, err := chainLoader(lnk, lnkCtx)
		if err != nil {
			return clientLoader(lnk, lnkCtx)
		}
		return reader, err
	}
}

// Graphsync creates a graphsync instance from the given loader and storer
func Graphsync(mctx helpers.MetricsCtx, lc fx.Lifecycle, loader dtypes.GraphsyncLoader, storer dtypes.GraphsyncStorer, h host.Host) dtypes.Graphsync {
	graphsyncNetwork := gsnet.NewFromLibp2pHost(h)
	ipldBridge := ipldbridge.NewIPLDBridge()
	gs := graphsync.New(helpers.LifecycleCtx(mctx, lc), graphsyncNetwork, ipldBridge, ipld.Loader(loader), ipld.Storer(storer))

	return gs
}
