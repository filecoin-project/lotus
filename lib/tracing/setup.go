package tracing

import (
	"os"
	"strings"

	"contrib.go.opencensus.io/exporter/jaeger"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/trace"
)

var log = logging.Logger("tracing")

const (
	// environment variable names
	envCollectorEndpoint = "LOTUS_JAEGER_COLLECTOR_ENDPOINT"
	envAgentEndpoint     = "LOTUS_JAEGER_AGENT_ENDPOINT"
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
func jaegerOptsFromEnv(opts *jaeger.Options) bool {
	var e string
	var ok bool
	if e, ok = os.LookupEnv(envJaegerUser); ok {
		if p, ok := os.LookupEnv(envJaegerCred); ok {
			opts.Username = e
			opts.Password = p
		} else {
			log.Warn("jaeger username supplied with no password. authentication will not be used.")
		}
	}
	if e, ok = os.LookupEnv(envCollectorEndpoint); ok {
		opts.CollectorEndpoint = e
		log.Infof("jaeger tracess will send to collector %s", e)
		return true
	}
	if e, ok = os.LookupEnv(envAgentEndpoint); ok {
		log.Infof("jaeger traces will be sent to agent %s", e)
		opts.AgentEndpoint = e
		return true
	}
	if e, ok = os.LookupEnv(envAgentHost); ok {
		if p, ok := os.LookupEnv(envAgentPort); ok {
			opts.AgentEndpoint = strings.Join([]string{e, p}, ":")
		} else {
			opts.AgentEndpoint = strings.Join([]string{e, "6831"}, ":")
		}
		log.Infof("jaeger traces will be sent to agent %s", opts.AgentEndpoint)
		return true
	}
	return false
}

func SetupJaegerTracing(serviceName string) *jaeger.Exporter {
	opts := jaeger.Options{}
	if !jaegerOptsFromEnv(&opts) {
		return nil
	}
	opts.ServiceName = serviceName
	je, err := jaeger.NewExporter(opts)
	if err != nil {
		log.Errorw("failed to create the jaeger exporter", "error", err)
		return nil
	}

	trace.RegisterExporter(je)
	trace.ApplyConfig(trace.Config{
		DefaultSampler: trace.AlwaysSample(),
	})
	return je
}
