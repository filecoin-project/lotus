package modules

import (
	"context"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	provider "github.com/ipni/index-provider"
	"github.com/ipni/index-provider/engine"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type IdxProv struct {
	fx.In

	fx.Lifecycle
	Datastore dtypes.MetadataDS
}

func IndexProvider(cfg config.IndexProviderConfig) func(params IdxProv, marketHost host.Host, dt dtypes.ProviderDataTransfer, maddr dtypes.MinerAddress, ps *pubsub.PubSub, nn dtypes.NetworkName) (provider.Interface, error) {
	return func(args IdxProv, marketHost host.Host, dt dtypes.ProviderDataTransfer, maddr dtypes.MinerAddress, ps *pubsub.PubSub, nn dtypes.NetworkName) (provider.Interface, error) {
		topicName := cfg.TopicName
		// If indexer topic name is left empty, infer it from the network name.
		if topicName == "" {
			// Use the same mechanism as the Dependency Injection (DI) to construct the topic name,
			// so that we are certain it is consistent with the name allowed by the subscription
			// filter.
			//
			// See: lp2p.GossipSub.
			topicName = build.IndexerIngestTopic(nn)
			log.Debugw("Inferred indexer topic from network name", "topic", topicName)
		}

		ipds := namespace.Wrap(args.Datastore, datastore.NewKey("/index-provider"))
		addrs := marketHost.Addrs()
		addrsString := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addrsString = append(addrsString, addr.String())
		}
		var opts = []engine.Option{
			engine.WithDatastore(ipds),
			engine.WithHost(marketHost),
			engine.WithRetrievalAddrs(addrsString...),
			engine.WithEntriesCacheCapacity(cfg.EntriesCacheCapacity),
			engine.WithChainedEntries(cfg.EntriesChunkSize),
			engine.WithTopicName(topicName),
			engine.WithPurgeCacheOnStart(cfg.PurgeCacheOnStart),
		}

		llog := log.With(
			"idxProvEnabled", cfg.Enable,
			"pid", marketHost.ID(),
			"topic", topicName,
			"retAddrs", marketHost.Addrs())
		// If announcements to the network are enabled, then set options for datatransfer publisher.
		if cfg.Enable {
			// Join the indexer topic using the market's pubsub instance. Otherwise, the provider
			// engine would create its own instance of pubsub down the line in go-legs, which has
			// no validators by default.
			t, err := ps.Join(topicName)
			if err != nil {
				llog.Errorw("Failed to join indexer topic", "err", err)
				return nil, xerrors.Errorf("joining indexer topic %s: %w", topicName, err)
			}

			// Get the miner ID and set as extra gossip data.
			// The extra data is required by the lotus-specific index-provider gossip message validators.
			ma := address.Address(maddr)
			opts = append(opts,
				engine.WithPublisherKind(engine.DataTransferPublisher),
				engine.WithDataTransfer(dt),
				engine.WithExtraGossipData(ma.Bytes()),
				engine.WithTopic(t),
			)
			llog = llog.With("extraGossipData", ma, "publisher", "data-transfer")
		} else {
			opts = append(opts, engine.WithPublisherKind(engine.NoPublisher))
			llog = llog.With("publisher", "none")
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
