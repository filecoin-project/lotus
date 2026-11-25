# API surface (api/)

Lotus serves its full-node API at `/rpc/v0`, `/rpc/v1` and `/rpc/v2` (`node/rpc.go`). Clients, gateways and tools such as Curio build against each version, so a signature or behaviour change reaches software you can't see.

## Where a method goes

- v0 (`api/v0api/`) is frozen. Change it only as a last resort, such as for a security fix.
- v1 (`api/api_full.go`) is the stable API most clients use. Avoid adding methods, and don't change existing signatures: a new argument, or a concrete Go type swapped for a dynamic JSON one, breaks clients even when the JSON looks similar. When a method's model stops matching the protocol, add a replacement beside it and leave the old one working. `StateMinerInitialPledgeForSector` replaced `StateMinerInitialPledgeCollateral`, whose `SectorPreCommitInfo` input stopped describing pledge once DDO (FIP-0076) let data arrive at prove-commit. Replacements drift too: from nv29 (FIP-0118) every sector gets full quality-adjusted power, so callers pass the whole sector size as its `verifiedSize`.
- v2 (`api/v2api/full.go`) takes new methods unless a strong reason demands v1. A v1 addition should look like v1: positional arguments and, for state reads, a trailing `types.TipSetKey`.
- Design v2 methods to extend. Methods that read chain state take a `types.TipSetSelector` (tag, height with an anchor, or key) and resolve it through `ChainModuleV2.ChainGetTipSet`, which owns tag, anchor and finality semantics; a height selector with a nil anchor resolves against `finalized`, not head. Pass other inputs as an options struct, so later fields arrive as optional additions rather than new arguments or methods. Arguments stay positional JSON-RPC params, as in v1.
- The `Eth*` methods are served on v1 and v2 from separate module instances (`node/builder_chain.go`, `node/modules/eth.go`) and must behave identically, including `latest` (the head's parent, since execution is deferred), `safe` and `finalized` (`documentation/en/finality.md`). The one intended divergence is `LOTUS_ETH_V1_DISABLE_F3_FINALITY_RESOLUTION=1`, a temporary operator escape hatch that reverts v1 to EC-only finality and a 30-epoch `safe`.

## Errors are wire API (api/api_errors.go)

Every part of an error response is consumed outside Lotus: the code, the message text, the shape of `data`, and any extra field. Treat a change to any of them as breaking, and check who reads it before choosing a shape.

- Downstream code matches message text, e.g. Curio string-matches the `eth_estimateGas` gas-cap error to drive PDP settlement, so rewording it breaks settlement. Keep messages stable, and give consumers a code or typed error to match.
- Only `code`, `message` and `data` are standard. go-jsonrpc can also send `meta` (from `JSONRPCError.Meta`, or any error with `MarshalJSON`), and strict clients reject the whole response, losing the code and message too, e.g. Rust clients built on jsonrpsee, including Forest. Machine-readable detail goes in `data`.
- `data` has established shapes, e.g. an object with `epoch` and `reason` for -32002 and revert bytes as hex for execution reverts. Changing a shape breaks clients that parse it. The -32002 decoder also accepts a legacy bare epoch with an optional reason in `meta`.
- go-jsonrpc maps one code to one registered Go type (`RPCErrors.Register`) and matches only the top-level error, so call sites return the typed error unwrapped. Several causes sharing a code (EIP-1474 -32002) can't be told apart after decoding unless the wire carries the cause. Before adding a discriminator, check whether anything reads it after a full round trip. Registered errors are decoded and re-encoded at each hop, including the gateway, and the type's `ToJSONRPCError` decides what survives (`ErrInvalidParams` drops `data`).
