package lp2p

import (
	"context"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	tls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/fx"
)

var DefaultTransports = simpleOpt(libp2p.DefaultTransports)
var QUIC = simpleOpt(libp2p.Transport(libp2pquic.NewTransport))

func Security(enabled, preferTLS bool) interface{} {
	if !enabled {
		return func() (opts Libp2pOpts) {
			// TODO: shouldn't this be Errorf to guarantee visibility?
			log.Warnf(`Your lotus node has been configured to run WITHOUT ENCRYPTED CONNECTIONS.
		You will not be able to connect to any nodes configured to use encrypted connections`)
			opts.Opts = append(opts.Opts, libp2p.NoSecurity)
			return opts
		}
	}
	return func() (opts Libp2pOpts) {
		if preferTLS {
			opts.Opts = append(opts.Opts, libp2p.ChainOptions(libp2p.Security(tls.ID, tls.New), libp2p.Security(noise.ID, noise.New)))
		} else {
			opts.Opts = append(opts.Opts, libp2p.ChainOptions(libp2p.Security(noise.ID, noise.New), libp2p.Security(tls.ID, tls.New)))
		}
		return opts
	}
}

func BandwidthCounter(lc fx.Lifecycle, id peer.ID) (opts Libp2pOpts, reporter metrics.Reporter, err error) {
	reporter = metrics.NewBandwidthCounter()
	opts.Opts = append(opts.Opts, libp2p.BandwidthReporter(reporter))

	// Register it with open telemetry. We report by-callback instead of implementing a custom
	// bandwidth counter to avoid allocating every time we read/write to a stream (and to stay
	// out of the hot path).
	//
	// Identity is required to ensure this observer observes with unique attributes.
	identityAttr := attrIdentity.String(id.String())
	registration, err := otelmeter.RegisterCallback(func(ctx context.Context, obs metric.Observer) error {
		for p, bw := range reporter.GetBandwidthByProtocol() {
			if p == "" {
				p = "<unknown>"
			}
			protoAttr := attrProtocolID.String(string(p))
			obs.ObserveInt64(otelmetrics.bandwidth, bw.TotalOut,
				metric.WithAttributes(identityAttr, protoAttr, attrDirectionOutbound))
			obs.ObserveInt64(otelmetrics.bandwidth, bw.TotalIn,
				metric.WithAttributes(identityAttr, protoAttr, attrDirectionInbound))
		}
		return nil
	}, otelmetrics.bandwidth)
	if err != nil {
		return Libp2pOpts{}, nil, err
	}
	lc.Append(fx.StopHook(registration.Unregister))

	return opts, reporter, nil
}
