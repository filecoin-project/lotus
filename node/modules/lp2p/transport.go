package lp2p

import (
	"context"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	tls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	lmetrics "github.com/filecoin-project/lotus/metrics"
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

func BandwidthCounter() (opts Libp2pOpts, reporter metrics.Reporter) {
	reporter = metrics.NewBandwidthCounter()
	reporter = &metricBandwithReporter{reporter}
	opts.Opts = append(opts.Opts, libp2p.BandwidthReporter(reporter))
	return opts, reporter
}

type metricBandwithReporter struct {
	metrics.Reporter
}

func (mbr *metricBandwithReporter) LogSentMessageStream(bytes int64, proto protocol.ID, id peer.ID) {
	mbr.Reporter.LogSentMessageStream(bytes, proto, id)

	if len(proto) == 0 {
		proto = "unknown"
	}
	stats.RecordWithTags(context.TODO(), []tag.Mutator{
		tag.Upsert(lmetrics.Direction, "outbound"),
		tag.Upsert(lmetrics.ProtocolID, string(proto))},
		lmetrics.Libp2pTrafficBytes.M(bytes))
}
func (mbr *metricBandwithReporter) LogRecvMessageStream(bytes int64, proto protocol.ID, id peer.ID) {
	mbr.Reporter.LogRecvMessageStream(bytes, proto, id)

	if len(proto) == 0 {
		proto = "unknown"
	}
	stats.RecordWithTags(context.TODO(), []tag.Mutator{
		tag.Upsert(lmetrics.Direction, "inbound"),
		tag.Upsert(lmetrics.ProtocolID, string(proto))},
		lmetrics.Libp2pTrafficBytes.M(bytes))
}
