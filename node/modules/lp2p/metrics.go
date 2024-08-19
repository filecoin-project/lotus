package lp2p

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var otelmeter = otel.Meter("libp2p")

var attrIdentity = attribute.Key("identity")
var attrProtocolID = attribute.Key("protocol")
var attrDirectionInbound = attribute.String("direction", "inbound")
var attrDirectionOutbound = attribute.String("direction", "outbound")

var otelmetrics = struct {
	bandwidth metric.Int64ObservableCounter
}{
	bandwidth: must(otelmeter.Int64ObservableCounter("lotus_libp2p_bandwidth",
		metric.WithDescription("Libp2p stream bandwidth."),
		metric.WithUnit("By"),
	)),
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
