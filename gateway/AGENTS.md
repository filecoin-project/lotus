# Gateway and eth RPC bounds (gateway)

The gateway is the curated v1 and v2 reverse proxy commonly exposed to the public internet in front of a full node.

- A gateway method must preserve the layered controls: the curated proxy surface, weighted basic/wallet/chain/state rate-limit tokens, global and per-connection limiting, and method-specific lookback, confidence, filter, subscription and trace-range bounds.
- Range limits differ by entry point: the gateway caps event ranges at 360 epochs (`DefaultEventFilterMaxHeightRange`), the node defaults to 2880 (`MaxFilterHeightRange`). Fixed per-query costs (anchors, lookups) don't shrink with the cap.
- HTTP `ReadTimeout` doesn't cancel handlers once the body is read; a `context canceled` in handler logs comes from the client or a proxy.
- Gateway caches hold immutable mappings only: block hash to tipset key, tipset key to height. A transaction hash identifies content, not its inclusion block, and a reorg can move it. Caching an inclusion height can authorise a different block from the one a later backend lookup traces; check reorg validity and whether execution is pinned to the checked block before caching transaction locations.
- Message-search lookback (`maxMessageLookbackEpochs`, epochs back from the search's starting tipset: head, or the `from` tipset the caller supplies) and tipset lookback (`maxLookbackDuration`, by wall-clock age) are separate bounds. `checkEpoch` enforces the latter only. A cached lookup must preserve both checks as the chain advances.
- Rate-limit tokens are coarse expense classes, not a count of backend RPCs. Global and per-connection limiters both use `MaxRateLimitTokens` as burst capacity, so a single charge above it fails rather than waits when throttling is on. Raising a method's weight means reviewing that constant across all methods. Calling another public proxy method internally charges its limiter too; share backend helpers when composing one request.
