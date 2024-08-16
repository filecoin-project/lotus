package metrics

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

func init() {
	// Set up otel to prometheus reporting so that F3 metrics are reported via lotus
	// prometheus metrics. This bridge eventually gets picked up by opencensus
	// exporter as HTTP handler. This by default registers an otel collector against
	// the global prometheus registry. In the future, we should clean up metrics in
	// Lotus and move it all to use otel. For now, bridge away.
	if bridge, err := prometheus.New(); err != nil {
		log.Errorf("could not create the otel prometheus exporter: %v", err)
	} else {
		provider := metric.NewMeterProvider(metric.WithReader(bridge))
		otel.SetMeterProvider(provider)
	}
}
