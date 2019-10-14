## Tracing

Lotus uses [OpenCensus](https://opencensus.io/) for tracing application flow.
This generates spans
through the execution of annotated code paths.

Currently it is set up to use jaeger, though other tracing backends should be
fairly easy to swap in.

## Running Locally

To easily run and view tracing locally, first, install jaeger. The easiest way
to do this is to download the binaries from
https://www.jaegertracing.io/download/ and then run the `jaeger-all-in-one`
binary. This will start up jaeger, listen for spans on `localhost:6831`, and
expose a web UI for viewing traces on `http://localhost:16686/`.

Now, to start sending traces from lotus to jaeger, set the environment variable
`LOTUS_JAEGER` to `localhost:6831`, and start the `lotus daemon`.

Now, to view any generated traces, open up `http://localhost:16686/` in your
browser.

## Adding Spans
To annotate a new codepath with spans, add the following lines to the top of the function you wish to trace:

```go
ctx, span := trace.StartSpan(ctx, "put function name here")
defer span.End()
```
