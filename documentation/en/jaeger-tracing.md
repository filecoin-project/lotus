# Jaeger Tracing

Lotus has tracing built into many of its internals. To view the traces, first download [Jaeger](https://www.jaegertracing.io/download/) (Choose the 'all-in-one' binary). Then run it somewhere, start up the lotus daemon, and open up localhost:16686 in your browser.

## Open Census

Lotus uses [OpenCensus](https://opencensus.io/) for tracing application flow. This generates spans through the execution of annotated code paths.

Currently it is set up to use Jaeger, though other tracing backends should be fairly easy to swap in.

## Running Locally

To easily run and view tracing locally, first, install jaeger. The easiest way to do this is to [download the binaries](https://www.jaegertracing.io/download/) and then run the `jaeger-all-in-one` binary. This will start up jaeger, listen for spans on `localhost:6831`, and expose a web UI for viewing traces on `http://localhost:16686/`.

Now, to start sending traces from Lotus to Jaeger, set the environment variable and start the daemon.

```bash
export LOTUS_JAEGER_AGENT_ENDPOINT=127.0.0.1:6831
lotus daemon
```

Alternatively, the agent endpoint can also be configured by a pair of environment variables to provide the host and port. The following snippet is functionally equivalent to the previous.

```bash
export LOTUS_JAEGER_AGENT_HOST=127.0.0.1
export LOTUS_JAEGER_AGENT_PORT=6831
lotus daemon
```

Now, to view any generated traces, open up `http://localhost:16686/` in your browser.

## Adding Spans

To annotate a new codepath with spans, add the following lines to the top of the function you wish to trace:

```go
ctx, span := trace.StartSpan(ctx, "put function name here")
defer span.End()
```
