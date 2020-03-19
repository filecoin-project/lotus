package modules

import (
	"context"

	eventbus "github.com/libp2p/go-eventbus"
	event "github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/sub"
	"github.com/filecoin-project/lotus/lib/peermgr"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

func RunHello(mctx helpers.MetricsCtx, lc fx.Lifecycle, h host.Host, svc *hello.Service) error {
	h.SetStreamHandler(hello.ProtocolID, svc.HandleStream)

	sub, err := h.EventBus().Subscribe(new(event.EvtPeerIdentificationCompleted), eventbus.BufSize(1024))
	if err != nil {
		return xerrors.Errorf("failed to subscribe to event bus: %w", err)
	}

	go func() {
		for evt := range sub.Out() {
			pic := evt.(event.EvtPeerIdentificationCompleted)
			go func() {
				if err := svc.SayHello(helpers.LifecycleCtx(mctx, lc), pic.Peer); err != nil {
					log.Warnw("failed to say hello", "error", err)
					return
				}
			}()
		}
	}()
	return nil
}

func RunPeerMgr(mctx helpers.MetricsCtx, lc fx.Lifecycle, pmgr *peermgr.PeerMgr) {
	go pmgr.Run(helpers.LifecycleCtx(mctx, lc))
}

func RunBlockSync(h host.Host, svc *blocksync.BlockSyncService) {
	h.SetStreamHandler(blocksync.BlockSyncProtocolID, svc.HandleStream)
}

func HandleIncomingBlocks(mctx helpers.MetricsCtx, lc fx.Lifecycle, ps *pubsub.PubSub, s *chain.Syncer, h host.Host) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	blocksub, err := ps.Subscribe(build.BlocksTopic)
	if err != nil {
		panic(err)
	}

	v := sub.NewBlockValidator(func(p peer.ID) {
		ps.BlacklistPeer(p)
		h.ConnManager().TagPeer(p, "badblock", -1000)
	})

	if err := ps.RegisterTopicValidator(build.BlocksTopic, v.Validate); err != nil {
		panic(err)
	}

	go sub.HandleIncomingBlocks(ctx, blocksub, s, h.ConnManager())
}

func HandleIncomingMessages(mctx helpers.MetricsCtx, lc fx.Lifecycle, ps *pubsub.PubSub, mpool *messagepool.MessagePool) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	msgsub, err := ps.Subscribe(build.MessagesTopic)
	if err != nil {
		panic(err)
	}

	v := sub.NewMessageValidator(mpool)

	if err := ps.RegisterTopicValidator(build.MessagesTopic, v.Validate); err != nil {
		panic(err)
	}

	go sub.HandleIncomingMessages(ctx, mpool, msgsub)
}

func RunDealClient(mctx helpers.MetricsCtx, lc fx.Lifecycle, c storagemarket.StorageClient) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			c.Run(ctx)
			return nil
		},
		OnStop: func(context.Context) error {
			c.Stop()
			return nil
		},
	})
}

func NewLocalDiscovery(ds dtypes.MetadataDS) *discovery.Local {
	return discovery.NewLocal(ds)
}

func RetrievalResolver(l *discovery.Local) retrievalmarket.PeerResolver {
	return discovery.Multi(l)
}
