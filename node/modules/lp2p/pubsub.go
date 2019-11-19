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

func withTracer(host host.Host, opts []pubsub.Option) []pubsub.Option {
	pi, err := peer.AddrInfoFromP2pAddr(ma.StringCast("/ip4/147.75.67.199/tcp/4001/p2p/QmTd6UvR47vUidRNZ1ZKXHrAFhqTJAD27rKL9XYghEKgKX"))
	if err != nil {
		panic(err)
	}

	tr, err := pubsub.NewRemoteTracer(context.TODO(), host, *pi)
	if err != nil {
		panic(err)
	}

	return append(opts, pubsub.WithEventTracer(tr))
}

func FloodSub(pubsubOptions ...pubsub.Option) interface{} {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host) (service *pubsub.PubSub, err error) {
		return pubsub.NewFloodSub(helpers.LifecycleCtx(mctx, lc), host, pubsubOptions...)
	}
}

func GossipSub(pubsubOptions ...pubsub.Option) interface{} {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host) (service *pubsub.PubSub, err error) {
		return pubsub.NewGossipSub(helpers.LifecycleCtx(mctx, lc), host, withTracer(host, pubsubOptions)...)
	}
}
