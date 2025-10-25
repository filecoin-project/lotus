# Lotus EIP-7702 Testing TODO

This list tracks Lotus-side tests for EIP-7702, mirroring geth/revm behavior. It complements BUILTIN_ACTORS_7702_TODO.md for the paired repo.

Priority order: P0 (now), P1 (soon), P2 (later)

P0 — Parsers and encoding
- RLP 0x04 parser/encoder
  - Round-trip encoding/decoding; multi-authorization list; empty `authorizationList` rejected; non-empty `accessList` rejected; invalid outer `v` rejected; invalid auth `yParity` rejected; wrong tuple arity rejected; signature initialization from 65-byte r||s||v.
- CBOR params (ApplyAndCall): [ [tuple...], [to(20), value, input] ]
  - Encoder produces wrapper `[ list ]` of 6-tuples; cross-package compat with actor decoder; legacy flat array accepted by decoder.

P0 — SignedMessage view + receipts
- Eth view reconstruction
- `EthTransactionFromSignedFilecoinMessage` reconstructs 0x04 transactions for EVM.ApplyAndCall; echoes `authorizationList`.
- Receipts attribution
  - `adjustReceiptForDelegation` sets `delegatedTo` from `authorizationList`.
  - When absent, extracts `delegatedTo` from synthetic EVM log topic keccak("EIP7702Delegated(address)") with 20-byte data.

P0 — Mempool policies (gated by NV)
- Cross-account invalidation on ApplyDelegations
  - Evict stale pending messages for affected authorities at/below expected nonce.
  - Multi-authority eviction.
- Per-EOA cap enforced after activation.

P0 — Gas accounting (scaffold)
- Counting + gating only (no absolute overhead assertions)
  - `countAuthInDelegatorParams` handles wrapper and legacy shapes.
  - Defer asserting numeric results from intrinsic overhead until authoritative constants land in actors/runtime; re‑enable/extend tests when finalized.

P1 — JSON-RPC plumbing
- eth_getTransactionReceipt
  - Returns `authorizationList` and `delegatedTo` fields; verify stability across block receipts retrieval.
- EthGetBlockReceipts/EthGetTransactionReceipt flows call `adjustReceiptForDelegation`.

P1 — Estimation integration
- EthEstimateGas
- For messages to EVM.ApplyAndCall with N tuples, expected gas includes intrinsic overhead (behavioral placeholder).

P1 — E2E tests (gated behind eip7702_enabled; run when wasm bundle includes EVM ApplyAndCall)
- Send-path routing
- Minimal 0x04 tx constructs ApplyAndCall params; mined receipt echoes `authorizationList` and `delegatedTo`.
- Delegated execution
  - CALL→EOA executes delegate code (via actor/runtime), storage/logs reflect delegation.

P2 — Edge/fuzz
- Fuzz RLP parsing for malformed tuples/fields.
- Large authorizationList sizes for performance regressions.

Notes
- Keep gating aligned to a single NV constant shared with builtin-actors. Update gas constants/refunds in lockstep.
