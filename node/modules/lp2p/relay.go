package lp2p

import (
	"fmt"

	"github.com/libp2p/go-libp2p"
	coredisc "github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/routing"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

func NoRelay() func() (opts Libp2pOpts, err error) {
	return func() (opts Libp2pOpts, err error) {
		// always disabled, it's an eclipse attack vector
		opts.Opts = append(opts.Opts, libp2p.DisableRelay())
		return
	}
}

func Discovery(router BaseIpfsRouting) (coredisc.Discovery, error) {
	// TODO: should be use baseRouting or can we use higher level router here?

	crouter, ok := router.(routing.ContentRouting)
	if !ok {
		return nil, fmt.Errorf("no suitable routing for discovery")
	}

	return routingdisc.NewRoutingDiscovery(crouter), nil
}
