package lp2p

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var otelmeter = otel.Meter("libp2p")

var attrPeerID = attribute.Key("peer")
var attrProtocolID = attribute.Key("protocol")
var attrDirectionInbound = attribute.String("direction", "inbound")
var attrDirectionOutbound = attribute.String("direction", "outbound")

var otelmetrics = struct {
	bandwidth metric.Int64ObservableGauge
}{
	bandwidth: must(otelmeter.Int64ObservableGauge("lotus_libp2p_bandwidth_total",
		metric.WithDescription("Libp2p stream traffic."),
		metric.WithUnit("By"),
	)),
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
