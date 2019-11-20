package lp2p

import (
	"context"

	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"

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
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host) (service *pubsub.PubSub, err error) {
		return pubsub.NewGossipSub(helpers.LifecycleCtx(mctx, lc), host, paresOpts(host, pubsubOptions)...)
	}
}

func paresOpts(host host.Host, in []PubsubOpt) []pubsub.Option {
	out := make([]pubsub.Option, len(in))
	for k, v := range in {
		out[k] = v(host)
	}
	return out
}
