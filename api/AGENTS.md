# JSON-RPC errors are wire API (api/api_errors.go)

Every part of an error response is consumed outside Lotus: the code, the message text, the shape of `data`, and any extra field. Treat a change to any of them as breaking, and check who reads it before choosing a shape.

- Downstream code matches message text, e.g. Curio string-matches the `eth_estimateGas` gas-cap error to drive PDP settlement, so rewording it breaks settlement. Keep messages stable, and give consumers a code or typed error to match.
- Only `code`, `message` and `data` are standard. go-jsonrpc can also send `meta` (from `JSONRPCError.Meta`, or any error with `MarshalJSON`), and strict clients reject the whole response, losing the code and message too, e.g. Rust clients built on jsonrpsee, including Forest. Machine-readable detail goes in `data`.
- `data` has established shapes, e.g. the bare epoch for -32002 and revert bytes as hex for execution reverts. Changing a shape breaks clients that parse it.
- go-jsonrpc maps one code to one registered Go type (`RPCErrors.Register`) and matches only the top-level error, so call sites return the typed error unwrapped. Several causes sharing a code (EIP-1474 -32002) can't be told apart after decoding unless the wire carries the cause. Before adding a discriminator, check whether anything reads it after a full round trip. Registered errors are decoded and re-encoded at each hop, including the gateway, and the type's `ToJSONRPCError` decides what survives (`ErrInvalidParams` drops `data`).
