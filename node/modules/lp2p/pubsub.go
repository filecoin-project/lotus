package lp2p

import (
	"context"
	"time"

	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type PubsubOpt func(host.Host) pubsub.Option

func PubsubTracer() PubsubOpt {
	return func(host host.Host) pubsub.Option {
		pi, err := peer.AddrInfoFromP2pAddr(ma.StringCast("/ip4/147.75.67.199/tcp/4001/p2p/QmTd6UvR47vUidRNZ1ZKXHrAFhqTJAD27rKL9XYghEKgKX"))
		if err != nil {
			panic(err)
		}

		tr, err := pubsub.NewRemoteTracer(context.TODO(), host, *pi)
		if err != nil {
			panic(err)
		}

		return pubsub.WithEventTracer(tr)
	}
}

func GossipSub(pubsubOptions ...PubsubOpt) interface{} {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, nn dtypes.NetworkName) (service *pubsub.PubSub, err error) {
		v11Options := []pubsub.Option{
			// Gossipsubv1.1 configuration
			pubsub.WithFloodPublish(true),
			pubsub.WithPeerScore(
				&pubsub.PeerScoreParams{
					// TODO: we want to assign heavy positive scores to bootstrappers and also plug the
					//       application specific score to the node itself in order to provide feedback
					//       to the pubsub system based on observed behaviour
					AppSpecificScore:  func(p peer.ID) float64 { return 0 },
					AppSpecificWeight: 1,

					// This sets the IP colocation threshold to 1 peer per
					IPColocationFactorThreshold: 1,
					IPColocationFactorWeight:    -100,
					// TODO we want to whitelist IPv6 /64s that belong to datacenters etc
					// IPColocationFactorWhitelist: map[string]struct{}{},

					DecayInterval: time.Second,
					DecayToZero:   0.01,

					// this retains non-positive scores for 6 hours
					RetainScore: 6 * time.Hour,

					// topic parameters
					Topics: map[string]*pubsub.TopicScoreParams{
						build.BlocksTopic(nn): &pubsub.TopicScoreParams{
							// expected 10 blocks/min
							TopicWeight: 0.1, // max is 250, max mesh penalty is -10, single invalid message is -100

							// 1 tick per second, maxes at 1 after 1 hour
							TimeInMeshWeight:  0.00027, // ~1/3600
							TimeInMeshQuantum: time.Second,
							TimeInMeshCap:     1,

							// deliveries decay after 1 hour, cap at 100 blocks
							FirstMessageDeliveriesWeight: 25,      // max value is 2500
							FirstMessageDeliveriesDecay:  0.99972, // 1 hour
							FirstMessageDeliveriesCap:    100,     // 100 blocks

							// deliveries decay after 1 hour, penalty activates at 1 minute and expects ~0.4 blocks
							MeshMessageDeliveriesWeight:     -576,    // max penalty is -100
							MeshMessageDeliveriesDecay:      0.99972, // 1 hour
							MeshMessageDeliveriesCap:        100,     // 100 blocks
							MeshMessageDeliveriesThreshold:  0.41666, // 5 / 12 blocks/min
							MeshMessageDeliveriesWindow:     10 * time.Millisecond,
							MeshMessageDeliveriesActivation: time.Minute,

							// decays after 15 min
							MeshFailurePenaltyWeight: -576,
							MeshFailurePenaltyDecay:  0.99888,

							// invalid messages decay after 1 hour
							InvalidMessageDeliveriesWeight: -1000,
							InvalidMessageDeliveriesDecay:  0.99972,
						},
						build.MessagesTopic(nn): &pubsub.TopicScoreParams{
							// expected > 1 tx/second
							TopicWeight: 0.05, // max is 50, max mesh penalty is -5, single invalid message is -50

							// 1 tick per second, maxes at 1 hour
							TimeInMeshWeight:  0.0002778, // ~1/3600
							TimeInMeshQuantum: time.Second,
							TimeInMeshCap:     1,

							// deliveries decay after 10min, cap at 1000 tx
							FirstMessageDeliveriesWeight: 1, // max value is 1000
							FirstMessageDeliveriesDecay:  0.99833,
							FirstMessageDeliveriesCap:    1000,

							// deliveries decay after 10min, penalty activates at 1 min and expects 5 txs
							MeshMessageDeliveriesWeight:     -4, // max penalty is -100
							MeshMessageDeliveriesDecay:      0.99833,
							MeshMessageDeliveriesCap:        1000,
							MeshMessageDeliveriesThreshold:  5,
							MeshMessageDeliveriesWindow:     10 * time.Millisecond,
							MeshMessageDeliveriesActivation: time.Minute,

							// decays after 5min
							MeshFailurePenaltyWeight: -4,
							MeshFailurePenaltyDecay:  0.99666,

							// invalid messages decay after 1 hour
							InvalidMessageDeliveriesWeight: -1000,
							InvalidMessageDeliveriesDecay:  0.99972,
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

		// TODO: we want to enable Peer eXchange on bootstrappers
		// v11Options = append(v11Options, pubsub.WithPeerExchange(XXX))

		// TODO: we want to hook the peer score inspector so that we can gain visibility
		//       in peer scores for debugging purposes -- this can be trigged by metrics collection
		// v11Options = append(v11Options, pubsub.WithPeerScoreInspect(XXX, time.Second))

		options := append(v11Options, paresOpts(host, pubsubOptions)...)

		return pubsub.NewGossipSub(helpers.LifecycleCtx(mctx, lc), host, options...)
	}
}

func paresOpts(host host.Host, in []PubsubOpt) []pubsub.Option {
	out := make([]pubsub.Option, len(in))
	for k, v := range in {
		out[k] = v(host)
	}
	return out
}
