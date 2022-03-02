package modules

import (
	"context"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	provider "github.com/filecoin-project/index-provider"
	"github.com/filecoin-project/index-provider/engine"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p-core/host"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type IdxProv struct {
	fx.In

	fx.Lifecycle
	Datastore dtypes.MetadataDS
	PeerID    peer.ID
	peerstore.Peerstore
}

func IndexProvider(cfg config.IndexProviderConfig) func(params IdxProv, marketHost host.Host, dt dtypes.ProviderDataTransfer, maddr dtypes.MinerAddress) (provider.Interface, error) {
	return func(args IdxProv, marketHost host.Host, dt dtypes.ProviderDataTransfer, maddr dtypes.MinerAddress) (provider.Interface, error) {
		ipds := namespace.Wrap(args.Datastore, datastore.NewKey("/index-provider"))

		pkey := args.Peerstore.PrivKey(args.PeerID)
		if pkey == nil {
			return nil, xerrors.Errorf("missing private key for node ID: %s", args.PeerID)
		}

		var retAdds []string
		for _, a := range marketHost.Addrs() {
			retAdds = append(retAdds, a.String())
		}

		// Get the miner ID and set as extra gossip data.
		// The extra data is required by the lotus-specific index-provider gossip message validators.
		ma := address.Address(maddr)
		log.Infof("Using extra gossip data in index provider engine: %s", ma)

		e, err := engine.New(cfg.Ingest, pkey, dt, marketHost, ipds, retAdds, engine.WithExtraGossipData(ma.Bytes()))
		if err != nil {
			return nil, xerrors.Errorf("creating indexer provider engine: %w", err)
		}

		args.Lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				// Note that the OnStart context is cancelled after startup. Its use in e.Start is
				// to start up gossipsub publishers and restore cache, all of  which are completed
				// before e.Start returns. Therefore, it is fine to reuse the give context.
				if err := e.Start(ctx); err != nil {
					return xerrors.Errorf("starting indexer provider engine: %w", err)
				}
				return nil
			},
			OnStop: func(_ context.Context) error {
				if err := e.Shutdown(); err != nil {
					return xerrors.Errorf("shutting down indexer provider engine: %w", err)
				}
				return nil
			},
		})
		return e, nil
	}
}
