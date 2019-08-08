package modules

import (
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/deals"
	"github.com/filecoin-project/go-lotus/chain/sub"
	"github.com/filecoin-project/go-lotus/node/hello"
	"github.com/filecoin-project/go-lotus/node/modules/helpers"
)

func RunHello(mctx helpers.MetricsCtx, lc fx.Lifecycle, h host.Host, svc *hello.Service) {
	h.SetStreamHandler(hello.ProtocolID, svc.HandleStream)

	bundle := inet.NotifyBundle{
		ConnectedF: func(_ inet.Network, c inet.Conn) {
			go func() {
				if err := svc.SayHello(helpers.LifecycleCtx(mctx, lc), c.RemotePeer()); err != nil {
					log.Warnw("failed to say hello", "error", err)
					return
				}
			}()
		},
	}
	h.Network().Notify(&bundle)
}

func RunBlockSync(h host.Host, svc *chain.BlockSyncService) {
	h.SetStreamHandler(chain.BlockSyncProtocolID, svc.HandleStream)
}

func HandleIncomingBlocks(mctx helpers.MetricsCtx, lc fx.Lifecycle, pubsub *pubsub.PubSub, s *chain.Syncer) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	blocksub, err := pubsub.Subscribe("/fil/blocks")
	if err != nil {
		panic(err)
	}

	go sub.HandleIncomingBlocks(ctx, blocksub, s)
}

func HandleIncomingMessages(mctx helpers.MetricsCtx, lc fx.Lifecycle, pubsub *pubsub.PubSub, mpool *chain.MessagePool) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	msgsub, err := pubsub.Subscribe("/fil/messages")
	if err != nil {
		panic(err)
	}

	go sub.HandleIncomingMessages(ctx, mpool, msgsub)
}

func RunDealClient(lc fx.Lifecycle, c *deals.Client) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			c.Run()
			return nil
		},
		OnStop: func(context.Context) error {
			c.Stop()
			return nil
		},
	})
}
