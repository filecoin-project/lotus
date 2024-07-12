package tracing

import (
	"os"
	"strings"

	logging "github.com/ipfs/go-log/v2"
	"go.opentelemetry.io/otel/bridge/opencensus"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.uber.org/zap"
)

var log = logging.Logger("tracing")

const (
	// environment variable names
	envCollectorEndpoint = "LOTUS_JAEGER_COLLECTOR_ENDPOINT"
	envAgentHost         = "LOTUS_JAEGER_AGENT_HOST"
	envAgentPort         = "LOTUS_JAEGER_AGENT_PORT"
	envJaegerUser        = "LOTUS_JAEGER_USERNAME"
	envJaegerCred        = "LOTUS_JAEGER_PASSWORD"
)

// When sending directly to the collector, agent options are ignored.
// The collector endpoint is an HTTP or HTTPs URL.
// The agent endpoint is a thrift/udp protocol and should be given
// as a string like "hostname:port". The agent can also be configured
// with separate host and port variables.
func jaegerOptsFromEnv() jaeger.EndpointOption {
	var e string
	var ok bool

	if e, ok = os.LookupEnv(envCollectorEndpoint); ok {
		options := []jaeger.CollectorEndpointOption{jaeger.WithEndpoint(e)}
		if u, ok := os.LookupEnv(envJaegerUser); ok {
			if p, ok := os.LookupEnv(envJaegerCred); ok {
				options = append(options, jaeger.WithUsername(u))
				options = append(options, jaeger.WithPassword(p))
			} else {
				log.Warn("jaeger username supplied with no password. authentication will not be used.")
			}
		}
		log.Infof("jaeger tracess will send to collector %s", e)
		return jaeger.WithCollectorEndpoint(options...)
	}

	if e, ok = os.LookupEnv(envAgentHost); ok {
		options := []jaeger.AgentEndpointOption{jaeger.WithAgentHost(e), jaeger.WithLogger(zap.NewStdLog(log.Desugar()))}
		var ep string
		if p, ok := os.LookupEnv(envAgentPort); ok {
			options = append(options, jaeger.WithAgentPort(p))
			ep = strings.Join([]string{e, p}, ":")
		} else {
			ep = strings.Join([]string{e, "6831"}, ":")
		}
		log.Infof("jaeger traces will be sent to agent %s", ep)
		return jaeger.WithAgentEndpoint(options...)
	}
	return nil
}

func SetupJaegerTracing(serviceName string) *tracesdk.TracerProvider {
	jaegerEndpoint := jaegerOptsFromEnv()
	if jaegerEndpoint == nil {
		return nil
	}
	je, err := jaeger.New(jaegerEndpoint)
	if err != nil {
		log.Errorw("failed to create the jaeger exporter", "error", err)
		return nil
	}
	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(je),
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
