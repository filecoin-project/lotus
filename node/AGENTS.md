# Node assembly (node)

- One builder assembles full and lite nodes (`--lite`), gated by `ApplyIf(isFullNode, ...)` and `ApplyIf(isLiteNode, ...)` in `builder_chain.go`. A lite node doesn't store the chain: its Chain, State, Gas and Eth modules forward to a gateway. Work that is cheap on a full node ("warm a cache at startup") can be ruinous on a lite node, where every header read is a remote fetch. Lite mode is rare but in use; check which `ApplyIf` block new work lands in.
- Startup side effects belong in invoke keys (`builder.go`), where someone reading node startup will see them. An `OnStart` hook inside an API constructor runs once per constructor (v1 and v2 both build), and readers don't look for it there.
