package modules

import (
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	eventbus "github.com/libp2p/go-eventbus"
	event "github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/discovery"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/beacon/drand"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/sub"
	"github.com/filecoin-project/lotus/chain/types"
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
					protos, _ := h.Peerstore().GetProtocols(pic.Peer)
					agent, _ := h.Peerstore().Get(pic.Peer, "AgentVersion")
					if protosContains(protos, hello.ProtocolID) {
						log.Warnw("failed to say hello", "error", err, "peer", pic.Peer, "supported", protos, "agent", agent)
					} else {
						log.Debugw("failed to say hello", "error", err, "peer", pic.Peer, "supported", protos, "agent", agent)
					}
					return
				}
			}()
		}
	}()
	return nil
}

func protosContains(protos []string, search string) bool {
	for _, p := range protos {
		if p == search {
			return true
		}
	}
	return false
}

func RunPeerMgr(mctx helpers.MetricsCtx, lc fx.Lifecycle, pmgr *peermgr.PeerMgr) {
	go pmgr.Run(helpers.LifecycleCtx(mctx, lc))
}

func RunBlockSync(h host.Host, svc *blocksync.BlockSyncService) {
	h.SetStreamHandler(blocksync.BlockSyncProtocolID, svc.HandleStream)
}

func waitForSync(stmgr *stmgr.StateManager, epochs int, subscribe func()) {
	nearsync := uint64(epochs) * uint64(build.BlockDelaySecs) * uint64(time.Second) //nolint

	// early check, are we synced at start up?
	ts := stmgr.ChainStore().GetHeaviestTipSet()
	timestamp := ts.MinTimestamp()
	now := uint64(build.Clock.Now().UnixNano())
	if timestamp > now-nearsync {
		subscribe()
		return
	}

	// we are not synced, subscribe to head changes and wait for sync
	stmgr.ChainStore().SubscribeHeadChanges(func(rev, app []*types.TipSet) error {
		if len(app) == 0 {
			return nil
		}

		latest := app[0].MinTimestamp()
		for _, ts := range app[1:] {
			timestamp := ts.MinTimestamp()
			if timestamp > latest {
				latest = timestamp
			}
		}

		now := uint64(build.Clock.Now().UnixNano())
		if latest > now-nearsync {
			subscribe()
			return store.ErrNotifieeDone
		}

		return nil
	})
}

func HandleIncomingBlocks(mctx helpers.MetricsCtx, lc fx.Lifecycle, ps *pubsub.PubSub, s *chain.Syncer, bserv dtypes.ChainBlockService, chain *store.ChainStore, stmgr *stmgr.StateManager, h host.Host, nn dtypes.NetworkName, bootstrapper dtypes.Bootstrapper) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	v := sub.NewBlockValidator(
		h.ID(), chain, stmgr,
		func(p peer.ID) {
			ps.BlacklistPeer(p)
			h.ConnManager().TagPeer(p, "badblock", -1000)
		})

	if err := ps.RegisterTopicValidator(build.BlocksTopic(nn), v.Validate); err != nil {
		panic(err)
	}

	subscribe := func() {
		log.Infof("subscribing to pubsub topic %s", build.BlocksTopic(nn))

		blocksub, err := ps.Subscribe(build.BlocksTopic(nn)) //nolint
		if err != nil {
			panic(err)
		}

		go sub.HandleIncomingBlocks(ctx, blocksub, s, bserv, h.ConnManager())
	}

	if bootstrapper {
		subscribe()
		return
	}

	// wait until we are synced within 10 blocks
	waitForSync(stmgr, 10, subscribe)
}

func HandleIncomingMessages(mctx helpers.MetricsCtx, lc fx.Lifecycle, ps *pubsub.PubSub, stmgr *stmgr.StateManager, mpool *messagepool.MessagePool, h host.Host, nn dtypes.NetworkName, bootstrapper dtypes.Bootstrapper) {
	ctx := helpers.LifecycleCtx(mctx, lc)

	v := sub.NewMessageValidator(h.ID(), mpool)

	if err := ps.RegisterTopicValidator(build.MessagesTopic(nn), v.Validate); err != nil {
		panic(err)
	}

	subscribe := func() {
		log.Infof("subscribing to pubsub topic %s", build.MessagesTopic(nn))

		msgsub, err := ps.Subscribe(build.MessagesTopic(nn)) //nolint
		if err != nil {
			panic(err)
		}

		go sub.HandleIncomingMessages(ctx, mpool, msgsub)
	}

	if bootstrapper {
		subscribe()
		return
	}

	// wait until we are synced within 1 block
	waitForSync(stmgr, 1, subscribe)
}

func NewLocalDiscovery(ds dtypes.MetadataDS) *discovery.Local {
	return discovery.NewLocal(namespace.Wrap(ds, datastore.NewKey("/deals/local")))
}

func RetrievalResolver(l *discovery.Local) retrievalmarket.PeerResolver {
	return discovery.Multi(l)
}

type RandomBeaconParams struct {
	fx.In

	PubSub      *pubsub.PubSub `optional:"true"`
	Cs          *store.ChainStore
	DrandConfig dtypes.DrandConfig
}

func BuiltinDrandConfig() dtypes.DrandConfig {
	return build.DrandConfig()
}

func RandomBeacon(p RandomBeaconParams, _ dtypes.AfterGenesisSet) (beacon.RandomBeacon, error) {
	gen, err := p.Cs.GetGenesis()
	if err != nil {
		return nil, err
	}

	//return beacon.NewMockBeacon(build.BlockDelaySecs * time.Second)
	return drand.NewDrandBeacon(gen.Timestamp, build.BlockDelaySecs, p.PubSub, p.DrandConfig)
}
