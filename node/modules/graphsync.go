package modules

import (
	"context"
	"time"

	"github.com/ipfs/go-graphsync"
	graphsyncimpl "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-graphsync/storeutil"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.opencensus.io/stats"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

// Graphsync creates a graphsync instance from the given loader and storer
func Graphsync(parallelTransfersForStorage uint64, parallelTransfersForRetrieval uint64) func(mctx helpers.MetricsCtx, lc fx.Lifecycle, r repo.LockedRepo, clientBs dtypes.ClientBlockstore, chainBs dtypes.ExposedBlockstore, h host.Host) (dtypes.Graphsync, error) {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, r repo.LockedRepo, clientBs dtypes.ClientBlockstore, chainBs dtypes.ExposedBlockstore, h host.Host) (dtypes.Graphsync, error) {
		graphsyncNetwork := gsnet.NewFromLibp2pHost(h)
		lsys := storeutil.LinkSystemForBlockstore(clientBs)

		gs := graphsyncimpl.New(helpers.LifecycleCtx(mctx, lc),
			graphsyncNetwork,
			lsys,
			graphsyncimpl.RejectAllRequestsByDefault(),
			graphsyncimpl.MaxInProgressIncomingRequests(parallelTransfersForStorage),
			graphsyncimpl.MaxInProgressOutgoingRequests(parallelTransfersForRetrieval),
			graphsyncimpl.MaxLinksPerIncomingRequests(config.MaxTraversalLinks),
			graphsyncimpl.MaxLinksPerOutgoingRequests(config.MaxTraversalLinks))
		chainLinkSystem := storeutil.LinkSystemForBlockstore(chainBs)
		err := gs.RegisterPersistenceOption("chainstore", chainLinkSystem)
		if err != nil {
			return nil, err
		}
		gs.RegisterIncomingRequestHook(func(p peer.ID, requestData graphsync.RequestData, hookActions graphsync.IncomingRequestHookActions) {
			_, has := requestData.Extension("chainsync")
			if has {
				// TODO: we should confirm the selector is a reasonable one before we validate
				// TODO: this code will get more complicated and should probably not live here eventually
				hookActions.ValidateRequest()
				hookActions.UsePersistenceOption("chainstore")
			}
		})
		gs.RegisterOutgoingRequestHook(func(p peer.ID, requestData graphsync.RequestData, hookActions graphsync.OutgoingRequestHookActions) {
			_, has := requestData.Extension("chainsync")
			if has {
				hookActions.UsePersistenceOption("chainstore")
			}
		})

		graphsyncStats(mctx, lc, gs)

		return gs, nil
	}
}

func graphsyncStats(mctx helpers.MetricsCtx, lc fx.Lifecycle, gs dtypes.Graphsync) {
	stopStats := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				t := time.NewTicker(10 * time.Second)
				for {
					select {
					case <-t.C:

						st := gs.Stats()
						stats.Record(mctx, metrics.GraphsyncReceivingPeersCount.M(int64(st.OutgoingRequests.TotalPeers)))
						stats.Record(mctx, metrics.GraphsyncReceivingActiveCount.M(int64(st.OutgoingRequests.Active)))
						stats.Record(mctx, metrics.GraphsyncReceivingCountCount.M(int64(st.OutgoingRequests.Pending)))
						stats.Record(mctx, metrics.GraphsyncReceivingTotalMemoryAllocated.M(int64(st.IncomingResponses.TotalAllocatedAllPeers)))
						stats.Record(mctx, metrics.GraphsyncReceivingTotalPendingAllocations.M(int64(st.IncomingResponses.TotalPendingAllocations)))
						stats.Record(mctx, metrics.GraphsyncReceivingPeersPending.M(int64(st.IncomingResponses.NumPeersWithPendingAllocations)))
						stats.Record(mctx, metrics.GraphsyncSendingPeersCount.M(int64(st.IncomingRequests.TotalPeers)))
						stats.Record(mctx, metrics.GraphsyncSendingActiveCount.M(int64(st.IncomingRequests.Active)))
						stats.Record(mctx, metrics.GraphsyncSendingCountCount.M(int64(st.IncomingRequests.Pending)))
						stats.Record(mctx, metrics.GraphsyncSendingTotalMemoryAllocated.M(int64(st.OutgoingResponses.TotalAllocatedAllPeers)))
						stats.Record(mctx, metrics.GraphsyncSendingTotalPendingAllocations.M(int64(st.OutgoingResponses.TotalPendingAllocations)))
						stats.Record(mctx, metrics.GraphsyncSendingPeersPending.M(int64(st.OutgoingResponses.NumPeersWithPendingAllocations)))

					case <-stopStats:
						return
					}
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			close(stopStats)
			return nil
		},
	})
}
