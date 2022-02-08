package modules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-graphsync"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p"
	ci "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	dtimpl "github.com/filecoin-project/go-data-transfer/impl"
	dtnet "github.com/filecoin-project/go-data-transfer/network"
	dtgstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	provider "github.com/filecoin-project/index-provider"
	"github.com/filecoin-project/index-provider/engine"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p-core/host"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/markets/idxprov"
	marketevents "github.com/filecoin-project/lotus/markets/loggers"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	"github.com/filecoin-project/lotus/node/repo"
)

type IdxProv struct {
	fx.In

	helpers.MetricsCtx
	fx.Lifecycle
	Repo      repo.LockedRepo
	Datastore dtypes.MetadataDS
	PeerID    peer.ID
	peerstore.Peerstore
}

func IndexProviderHost(cfg config.IndexProviderConfig) func(IdxProv) (idxprov.Host, error) {
	return func(args IdxProv) (idxprov.Host, error) {
		pkey := args.Peerstore.PrivKey(args.PeerID)
		if pkey == nil {
			return nil, fmt.Errorf("missing private key for node ID: %s", args.PeerID.Pretty())
		}

		h, err := createIndexProviderHost(args.MetricsCtx, args.Lifecycle, pkey, args.Peerstore, cfg.ListenAddresses, cfg.AnnounceAddresses)
		if err != nil {
			return nil, xerrors.Errorf("creating indexer provider host: %w", err)
		}

		return h, nil
	}
}

func IndexProvider(cfg config.IndexProviderConfig) func(params IdxProv, marketHost host.Host, h idxprov.Host, maddr dtypes.MinerAddress) (provider.Interface, error) {
	return func(args IdxProv, marketHost host.Host, h idxprov.Host, maddr dtypes.MinerAddress) (provider.Interface, error) {
		ipds := namespace.Wrap(args.Datastore, datastore.NewKey("/index-provider"))

		pkey := args.Peerstore.PrivKey(args.PeerID)
		if pkey == nil {
			return nil, fmt.Errorf("missing private key for node ID: %s", args.PeerID.Pretty())
		}

		dt, err := newIndexProviderDataTransfer(cfg, args.MetricsCtx, args.Lifecycle, args.Repo, h, ipds)
		if err != nil {
			return nil, err
		}

		var maddrs []string
		for _, a := range marketHost.Addrs() {
			maddrs = append(maddrs, a.String())
		}

		// Get the miner ID and set as extra gossip data.
		// The extra data is required by the lotus-specific index-provider gossip message validators.
		ma := address.Address(maddr)
		log.Info("Using extra gossip data in index provider engine: %s", ma.String())

		e, err := engine.New(cfg.Ingest, pkey, dt, h, ipds, maddrs, engine.WithExtraGossipData(ma.Bytes()))
		if err != nil {
			return nil, xerrors.Errorf("creating indexer provider engine: %w", err)
		}

		args.Lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				// Start the engine
				err := e.Start(ctx)
				if err != nil {
					return xerrors.Errorf("starting indexer provider engine: %s", err)
				}

				return nil
			},
			OnStop: func(ctx context.Context) error {
				return e.Shutdown()
			},
		})

		return e, nil
	}
}

func createIndexProviderHost(mctx helpers.MetricsCtx, lc fx.Lifecycle, pkey ci.PrivKey, pstore peerstore.Peerstore, listenAddrs []string, announceAddrs []string) (host.Host, error) {
	addrsFactory, err := lp2p.MakeAddrsFactory(announceAddrs, nil)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.Identity(pkey),
		libp2p.Peerstore(pstore),
		libp2p.ListenAddrStrings(listenAddrs...),
		libp2p.AddrsFactory(addrsFactory),
		libp2p.Ping(true),
		libp2p.UserAgent("lotus-indexer-provider-" + build.UserVersion()),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return h.Close()
		},
	})

	return h, nil
}

func newIndexProviderDataTransfer(cfg config.IndexProviderConfig, mctx helpers.MetricsCtx, lc fx.Lifecycle, r repo.LockedRepo, h host.Host, ds datastore.Batching) (datatransfer.Manager, error) {
	net := dtnet.NewFromLibp2pHost(h)

	// Set up graphsync
	gs := newIndexProviderGraphsync(cfg, mctx, lc, h)

	// Set up data transfer
	dtDs := namespace.Wrap(ds, datastore.NewKey("/datatransfer/transfers"))
	transport := dtgstransport.NewTransport(h.ID(), gs)
	dtPath := filepath.Join(r.Path(), "indexer-provider", "data-transfer")
	err := os.MkdirAll(dtPath, 0755) //nolint: gosec
	if err != nil && !os.IsExist(err) {
		return nil, xerrors.Errorf("creating indexer provider data transfer dir %s: %w", dtPath, err)
	}

	dt, err := dtimpl.NewDataTransfer(dtDs, net, transport)
	if err != nil {
		return nil, xerrors.Errorf("creating indexer provider data transfer module: %w", err)
	}

	dt.OnReady(marketevents.ReadyLogger("indexer-provider data transfer"))
	lc.Append(fx.Hook{
		OnStart: dt.Start,
		OnStop:  dt.Stop,
	})
	return dt, nil
}

func newIndexProviderGraphsync(cfg config.IndexProviderConfig, mctx helpers.MetricsCtx, lc fx.Lifecycle, h host.Host) graphsync.GraphExchange {
	graphsyncNetwork := gsnet.NewFromLibp2pHost(h)
	return graphsyncimpl.New(helpers.LifecycleCtx(mctx, lc),
		graphsyncNetwork,
		cidlink.DefaultLinkSystem(),
		graphsyncimpl.MaxInProgressIncomingRequests(cfg.MaxSimultaneousTransfers),
		graphsyncimpl.MaxLinksPerIncomingRequests(config.MaxTraversalLinks),
		graphsyncimpl.MaxLinksPerOutgoingRequests(config.MaxTraversalLinks))
}
