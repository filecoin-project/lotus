package tracing

import (
	"context"
	"os"
	"strings"

	logging "github.com/ipfs/go-log/v2"
	"go.opentelemetry.io/otel/bridge/opencensus"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

var log = logging.Logger("tracing")

const (
	// environment variable names
	envOTLPExporterEndpoint = "LOTUS_OTEL_EXPORTER_ENDPOINT"
	envOTLPExporterInsecure = "LOTUS_OTEL_EXPORTER_INSECURE"
)

func SetupOTLPTracing(ctx context.Context, serviceName string) *tracesdk.TracerProvider {
	endpoint, ok := os.LookupEnv(envOTLPExporterEndpoint)
	if !ok || endpoint == "" {
		return nil
	}

	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
	}

	if strings.EqualFold(os.Getenv(envOTLPExporterInsecure), "true") {
		options = append(options, otlptracehttp.WithInsecure())
	}

	exp, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		log.Errorw("failed to create the OTLP trace exporter", "error", err)
		return nil
	}

	log.Infof("OTLP traces will be exported to %s", endpoint)

	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in an Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)
	opencensus.InstallTraceBridge(opencensus.WithTracerProvider(tp))
	return tp
}
