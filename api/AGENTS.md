# JSON-RPC errors are wire API (api/api_errors.go)

Every part of an error response is consumed outside Lotus: the code, the message text, the shape of `data`, and any extra field. Treat a change to any of them as breaking, and check who reads it before choosing a shape.

- Message text is matched by downstream Go code. Curio's `isGasEstimateOutOfGas` string-matches `eth_estimateGas` errors, and rewording the gas-cap message (#13911) stopped Curio's PDP settlement until curio#1633. Keep messages stable, and give consumers a code or typed error to match.
- Only `code`, `message` and `data` are standard. go-jsonrpc can also send `meta` (from `JSONRPCError.Meta`, or any error with `MarshalJSON`), and strict clients reject the whole response: jsonrpsee's `ErrorObject` denies unknown fields, so Forest and other Rust clients lose the code and message too. Machine-readable detail goes in `data`.
- `data` has established shapes: the bare epoch for -32002 (#13644), revert bytes as hex for execution reverts. Changing a shape breaks clients that parse it.
- go-jsonrpc maps one code to one registered Go type (`RPCErrors.Register`) and matches only the top-level error, so call sites return the typed error unwrapped. Several causes sharing a code (EIP-1474 -32002) can't be told apart after decoding unless the wire carries the cause. Before adding a discriminator, check whether anything reads it after a full round trip. Registered errors are decoded and re-encoded at each hop, including the gateway, and the type's `ToJSONRPCError` decides what survives (`ErrInvalidParams` drops `data`).
