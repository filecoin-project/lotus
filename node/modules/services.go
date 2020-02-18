package modules

import (
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	inet "github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/sub"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/hello"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/peermgr"
)

const BlocksTopic = "/fil/blocks"
const MessagesTopic = "/fil/messages"

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

func RunPeerMgr(mctx helpers.MetricsCtx, lc fx.Lifecycle, pmgr *peermgr.PeerMgr) {
	go pmgr.Run(helpers.LifecycleCtx(mctx, lc))
}

func RunBlockSync(h host.Host, svc *blocksync.BlockSyncService) {
	h.SetStreamHandler(blocksync.BlockSyncProtocolID, svc.HandleStream)
}

func HandleIncomingBlocks(mctx helpers.MetricsCtx, lc fx.Lifecycle, ps *pubsub.PubSub, s *chain.Syncer, h host.Host) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	blocksub, err := ps.Subscribe(BlocksTopic)
	if err != nil {
		panic(err)
	}

	v := sub.NewBlockValidator(func(p peer.ID) {
		ps.BlacklistPeer(p)
		h.ConnManager().TagPeer(p, "badblock", -1000)
	})

	if err := ps.RegisterTopicValidator(BlocksTopic, v.Validate); err != nil {
		panic(err)
	}

	go sub.HandleIncomingBlocks(ctx, blocksub, s, h.ConnManager())
}

func HandleIncomingMessages(mctx helpers.MetricsCtx, lc fx.Lifecycle, ps *pubsub.PubSub, mpool *messagepool.MessagePool) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	msgsub, err := ps.Subscribe(MessagesTopic)
	if err != nil {
		panic(err)
	}

	v := func(ctx context.Context, pid peer.ID, msg *pubsub.Message) bool {
		m, err := types.DecodeSignedMessage(msg.GetData())
		if err != nil {
			log.Errorf("got incorrectly formatted Message: %s", err)
			return false
		}

		msg.ValidatorData = m
		return true
	}

	if err := ps.RegisterTopicValidator(MessagesTopic, v); err != nil {
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
