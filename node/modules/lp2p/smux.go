package lp2p

import (
	"os"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
)

func makeSmuxTransportOption() libp2p.Option {
	const yamuxID = "/yamux/1.0.0"

	ymxtpt := *yamux.DefaultTransport
	ymxtpt.AcceptBacklog = 512

	if os.Getenv("YAMUX_DEBUG") != "" {
		ymxtpt.LogOutput = os.Stderr
	}

	return libp2p.Muxer(yamuxID, &ymxtpt)
}

func SmuxTransport() func() (opts Libp2pOpts, err error) {
	return func() (opts Libp2pOpts, err error) {
		opts.Opts = append(opts.Opts, makeSmuxTransportOption())
		return
	}
}
