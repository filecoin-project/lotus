package lp2p

import (
	"context"
	"time"

	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	blake2b "github.com/minio/blake2b-simd"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

func init() {
	// configure larger overlay parameters
	pubsub.GossipSubD = 8
	pubsub.GossipSubDscore = 6
	pubsub.GossipSubDlo = 6
	pubsub.GossipSubDhi = 12
	pubsub.GossipSubDlazy = 12
	pubsub.GossipSubDirectConnectInitialDelay = 30 * time.Second
}

func GossipSub(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, nn dtypes.NetworkName, bp dtypes.BootstrapPeers, cfg *config.Pubsub) (service *pubsub.PubSub, err error) {
	bootstrappers := make(map[peer.ID]struct{})
	for _, pi := range bp {
		bootstrappers[pi.ID] = struct{}{}
	}
	isBootstrapNode := cfg.Bootstrapper

	options := []pubsub.Option{
		// Gossipsubv1.1 configuration
		pubsub.WithFloodPublish(true),
		pubsub.WithPeerScore(
			&pubsub.PeerScoreParams{
				AppSpecificScore: func(p peer.ID) float64 {
					// return a heavy positive score for bootstrappers so that we don't unilaterally prune
					// them and accept PX from them.
					// we don't do that in the bootstrappers themselves to avoid creating a closed mesh
					// between them (however we might want to consider doing just that)
					_, ok := bootstrappers[p]
					if ok && !isBootstrapNode {
						return 2500
					}

					// TODO: we want to  plug the application specific score to the node itself in order
					//       to provide feedback to the pubsub system based on observed behaviour
					return 0
				},
				AppSpecificWeight: 1,

				// This sets the IP colocation threshold to 1 peer per
				IPColocationFactorThreshold: 1,
				IPColocationFactorWeight:    -100,
				// TODO we want to whitelist IPv6 /64s that belong to datacenters etc
				// IPColocationFactorWhitelist: map[string]struct{}{},

				DecayInterval: pubsub.DefaultDecayInterval,
				DecayToZero:   pubsub.DefaultDecayToZero,

				// this retains non-positive scores for 6 hours
				RetainScore: 6 * time.Hour,

				// topic parameters
				Topics: map[string]*pubsub.TopicScoreParams{
					build.BlocksTopic(nn): {
						// expected 10 blocks/min
						TopicWeight: 0.1, // max is 50, max mesh penalty is -10, single invalid message is -100

						// 1 tick per second, maxes at 1 after 1 hour
						TimeInMeshWeight:  0.00027, // ~1/3600
						TimeInMeshQuantum: time.Second,
						TimeInMeshCap:     1,

						// deliveries decay after 1 hour, cap at 100 blocks
						FirstMessageDeliveriesWeight: 5, // max value is 500
						FirstMessageDeliveriesDecay:  pubsub.ScoreParameterDecay(time.Hour),
						FirstMessageDeliveriesCap:    100, // 100 blocks in an hour

						// tracks deliveries in the last minute
						// penalty activates at 1 minute and expects ~0.4 blocks
						MeshMessageDeliveriesWeight:     -576, // max penalty is -100
						MeshMessageDeliveriesDecay:      pubsub.ScoreParameterDecay(time.Minute),
						MeshMessageDeliveriesCap:        10,      // 10 blocks in a minute
						MeshMessageDeliveriesThreshold:  0.41666, // 10/12/2 blocks/min
						MeshMessageDeliveriesWindow:     10 * time.Millisecond,
						MeshMessageDeliveriesActivation: time.Minute,

						// decays after 15 min
						MeshFailurePenaltyWeight: -576,
						MeshFailurePenaltyDecay:  pubsub.ScoreParameterDecay(15 * time.Minute),

						// invalid messages decay after 1 hour
						InvalidMessageDeliveriesWeight: -1000,
						InvalidMessageDeliveriesDecay:  pubsub.ScoreParameterDecay(time.Hour),
					},
					build.MessagesTopic(nn): {
						// expected > 1 tx/second
						TopicWeight: 0.05, // max is 25, max mesh penalty is -5, single invalid message is -100

						// 1 tick per second, maxes at 1 hour
						TimeInMeshWeight:  0.0002778, // ~1/3600
						TimeInMeshQuantum: time.Second,
						TimeInMeshCap:     1,

						// deliveries decay after 10min, cap at 1000 tx
						FirstMessageDeliveriesWeight: 0.5, // max value is 500
						FirstMessageDeliveriesDecay:  pubsub.ScoreParameterDecay(10 * time.Minute),
						FirstMessageDeliveriesCap:    1000,

						// tracks deliveries in the last minute
						// penalty activates at 1 min and expects 2.5 txs
						MeshMessageDeliveriesWeight:     -16, // max penalty is -100
						MeshMessageDeliveriesDecay:      pubsub.ScoreParameterDecay(time.Minute),
						MeshMessageDeliveriesCap:        100, // 100 txs in a minute
						MeshMessageDeliveriesThreshold:  2.5, // 60/12/2 txs/minute
						MeshMessageDeliveriesWindow:     10 * time.Millisecond,
						MeshMessageDeliveriesActivation: time.Minute,

						// decays after 5min
						MeshFailurePenaltyWeight: -16,
						MeshFailurePenaltyDecay:  pubsub.ScoreParameterDecay(5 * time.Minute),

						// invalid messages decay after 1 hour
						InvalidMessageDeliveriesWeight: -2000,
						InvalidMessageDeliveriesDecay:  pubsub.ScoreParameterDecay(time.Hour),
					},
				},
			},
			&pubsub.PeerScoreThresholds{
				GossipThreshold:             -500,
				PublishThreshold:            -1000,
				GraylistThreshold:           -2500,
				AcceptPXThreshold:           1000,
				OpportunisticGraftThreshold: 2.5,
			},
		),
	}

	// enable Peer eXchange on bootstrappers
	if isBootstrapNode {
		// turn off the mesh in bootstrappers -- only do gossip and PX
		pubsub.GossipSubD = 0
		pubsub.GossipSubDscore = 0
		pubsub.GossipSubDlo = 0
		pubsub.GossipSubDhi = 0
		pubsub.GossipSubDlazy = 1024
		pubsub.GossipSubGossipFactor = 0.5
		// turn on PX
		options = append(options, pubsub.WithPeerExchange(true))
	}

	// direct peers
	if cfg.DirectPeers != nil {
		var directPeerInfo []peer.AddrInfo

		for _, addr := range cfg.DirectPeers {
			a, err := ma.NewMultiaddr(addr)
			if err != nil {
				return nil, err
			}

			pi, err := peer.AddrInfoFromP2pAddr(a)
			if err != nil {
				return nil, err
			}

			directPeerInfo = append(directPeerInfo, *pi)
		}

		options = append(options, pubsub.WithDirectPeers(directPeerInfo))
	}

	// tracer
	if cfg.RemoteTracer != "" {
		a, err := ma.NewMultiaddr(cfg.RemoteTracer)
		if err != nil {
			return nil, err
		}

		pi, err := peer.AddrInfoFromP2pAddr(a)
		if err != nil {
			return nil, err
		}

		tr, err := pubsub.NewRemoteTracer(context.TODO(), host, *pi)
		if err != nil {
			return nil, err
		}

		trw := newTracerWrapper(tr)
		options = append(options, pubsub.WithEventTracer(trw))
	}

	// TODO: we want to hook the peer score inspector so that we can gain visibility
	//       in peer scores for debugging purposes -- this might be trigged by metrics collection
	// options = append(options, pubsub.WithPeerScoreInspect(XXX, time.Second))

	return pubsub.NewGossipSub(helpers.LifecycleCtx(mctx, lc), host, options...)
}

func HashMsgId(m *pubsub_pb.Message) string {
	hash := blake2b.Sum256(m.Data)
	return string(hash[:])
}

func newTracerWrapper(tr pubsub.EventTracer) pubsub.EventTracer {
	return &tracerWrapper{tr: tr}
}

type tracerWrapper struct {
	tr pubsub.EventTracer
}

func (trw *tracerWrapper) Trace(evt *pubsub_pb.TraceEvent) {
	// this filters the trace events reported to the remote tracer to include only
	// JOIN/LEAVE/GRAFT/PRUNE/PUBLISH/DELIVER. This significantly reduces bandwidth usage and still
	// collects enough data to recover the state of the mesh and compute message delivery latency
	// distributions.
	// TODO: hook all events into local metrics for inspection through the dashboard
	switch evt.GetType() {
	case pubsub_pb.TraceEvent_PUBLISH_MESSAGE:
		trw.tr.Trace(evt)
	case pubsub_pb.TraceEvent_DELIVER_MESSAGE:
		trw.tr.Trace(evt)
	case pubsub_pb.TraceEvent_JOIN:
		trw.tr.Trace(evt)
	case pubsub_pb.TraceEvent_LEAVE:
		trw.tr.Trace(evt)
	case pubsub_pb.TraceEvent_GRAFT:
		trw.tr.Trace(evt)
	case pubsub_pb.TraceEvent_PRUNE:
		trw.tr.Trace(evt)
	}
}
