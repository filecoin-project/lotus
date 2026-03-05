# Tracing with OpenTelemetry

Lotus has tracing built into many of its internals. Traces are exported using the [OpenTelemetry](https://opentelemetry.io/) OTLP protocol over HTTP, which is supported by many backends including [Jaeger](https://www.jaegertracing.io/), [Grafana Tempo](https://grafana.com/oss/tempo/), and others.

## OpenCensus Bridge

Lotus uses an OpenCensus-to-OpenTelemetry bridge so that existing OpenCensus instrumentation is captured and exported via the OTLP exporter.

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `LOTUS_OTEL_EXPORTER_ENDPOINT` | OTLP HTTP endpoint (host:port). Tracing is disabled when unset. | `localhost:4318` |
| `LOTUS_OTEL_EXPORTER_INSECURE` | Set to `true` to disable TLS. | `true` |

The OpenTelemetry SDK also respects standard `OTEL_*` environment variables such as `OTEL_EXPORTER_OTLP_HEADERS` for authentication headers. See the [OpenTelemetry specification](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/) for details.

## Running Locally

To easily run and view tracing locally, first, install Jaeger. The easiest way to do this is to [download the binaries](https://www.jaegertracing.io/download/) and then run the `jaeger-all-in-one` binary. This will start up Jaeger, listen for OTLP traces on `localhost:4318`, and expose a web UI for viewing traces on `http://localhost:16686/`.

Now, to start sending traces from Lotus to Jaeger, set the environment variables and start the daemon.

```bash
export LOTUS_OTEL_EXPORTER_ENDPOINT=localhost:4318
export LOTUS_OTEL_EXPORTER_INSECURE=true
lotus daemon
```

Now, to view any generated traces, open up `http://localhost:16686/` in your browser.

## Adding Spans

To annotate a new codepath with spans, add the following lines to the top of the function you wish to trace:

```go
ctx, span := trace.StartSpan(ctx, "put function name here")
defer span.End()
```
