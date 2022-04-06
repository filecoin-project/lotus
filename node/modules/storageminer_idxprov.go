package modules

import (
	"context"

	"github.com/filecoin-project/go-address"
	provider "github.com/filecoin-project/index-provider"
	"github.com/filecoin-project/index-provider/engine"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p-core/host"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type IdxProv struct {
	fx.In

	fx.Lifecycle
	Datastore dtypes.MetadataDS
}

func IndexProvider(cfg config.IndexProviderConfig) func(params IdxProv, marketHost host.Host, dt dtypes.ProviderDataTransfer, maddr dtypes.MinerAddress) (provider.Interface, error) {
	return func(args IdxProv, marketHost host.Host, dt dtypes.ProviderDataTransfer, maddr dtypes.MinerAddress) (provider.Interface, error) {
		ipds := namespace.Wrap(args.Datastore, datastore.NewKey("/index-provider"))
		var opts = []engine.Option{
			engine.WithDatastore(ipds),
			engine.WithHost(marketHost),
			engine.WithRetrievalAddrs(marketHost.Addrs()...),
			engine.WithEntriesCacheCapacity(cfg.EntriesCacheCapacity),
			engine.WithEntriesChunkSize(cfg.EntriesChunkSize),
			engine.WithTopicName(cfg.TopicName),
			engine.WithPurgeCacheOnStart(cfg.PurgeCacheOnStart),
		}

		llog := log.With("idxProvEnabled", cfg.Enable, "pid", marketHost.ID(), "retAddrs", marketHost.Addrs())
		// If announcements to the network are enabled, then set options for datatransfer publisher.
		if cfg.Enable {
			// Get the miner ID and set as extra gossip data.
			// The extra data is required by the lotus-specific index-provider gossip message validators.
			ma := address.Address(maddr)
			opts = append(opts,
				engine.WithPublisherKind(engine.DataTransferPublisher),
				engine.WithDataTransfer(dt),
				engine.WithExtraGossipData(ma.Bytes()))
			llog = llog.With("extraGossipData", ma)
		} else {
			opts = append(opts, engine.WithPublisherKind(engine.NoPublisher))
		}

		// Instantiate the index provider engine.
		e, err := engine.New(opts...)
		if err != nil {
			return nil, xerrors.Errorf("creating indexer provider engine: %w", err)
		}
		llog.Info("Instantiated index provider engine")

		args.Lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				// Note that the OnStart context is cancelled after startup. Its use in e.Start is
				// to start up gossipsub publishers and restore cache, all of  which are completed
				// before e.Start returns. Therefore, it is fine to reuse the give context.
				if err := e.Start(ctx); err != nil {
					return xerrors.Errorf("starting indexer provider engine: %w", err)
				}
				log.Infof("Started index provider engine")
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
