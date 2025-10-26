# Lotus EIP-7702 Testing TODO

This list tracks Lotus-side tests for EIP-7702, mirroring geth/revm behavior. It complements BUILTIN_ACTORS_7702_TODO.md for the paired repo.

Priority order: P0 (now), P1 (soon), P2 (later)

P0 — Critical decisions & safety
- Mempool policy resolution (DECIDED: document deviation)
  - For this branch, no 7702-specific ingress policies will be implemented (no cross-account invalidation or per-EOA pending caps). This is documented in AGENTS.md and changelog; standard mempool rules continue to apply. No action needed.

P0 — Parsers and encoding
- RLP 0x04 parser/encoder
  - Round-trip encoding/decoding; multi-authorization list; empty `authorizationList` rejected; non-empty `accessList` rejected; invalid outer `v` rejected; invalid auth `yParity` rejected; wrong tuple arity rejected; signature initialization from 65-byte r||s||v.
- CBOR params (ApplyAndCall): [ [tuple...], [to(20), value, input] ]
  - Encoder produces wrapper `[ list ]` of 6-tuples; cross-package compat with actor decoder; canonical wrapper-only (legacy shapes removed).

P0 — SignedMessage view + receipts
– Eth view reconstruction (DONE)
– `EthTransactionFromSignedFilecoinMessage` reconstructs 0x04 transactions for EVM.ApplyAndCall; echoes `authorizationList`.
– Receipts attribution (DONE)
  - `adjustReceiptForDelegation` sets `delegatedTo` from `authorizationList`.
  - When absent, extracts `delegatedTo` from synthetic EVM log topic keccak("EIP7702Delegated(address)") with 20-byte data.

P0 — Mempool policies (gated by NV) (N/A on this branch)
– Cross-account invalidation on ApplyDelegations
  - Not implemented on this branch; standard mempool rules apply.
– Per-EOA cap
  - Not implemented on this branch; decision documented in AGENTS.md/changelog.

P0 — Gas accounting (scaffold) (DONE)
– Counting + gating only (no absolute overhead assertions)
  - `countAuthInDelegatorParams` handles canonical wrapper shape.
  - Defer asserting numeric results from intrinsic overhead until authoritative constants land in actors/runtime; re‑enable/extend tests when finalized.

P1 — JSON-RPC plumbing (DONE)
- eth_getTransactionReceipt
  - Returns `authorizationList` and `delegatedTo` fields; stability covered in unit tests.
- EthGetBlockReceipts/EthGetTransactionReceipt flows call `adjustReceiptForDelegation`.
- EthTransaction reconstruction robustness (DONE)
  - Strict decoder added for ApplyAndCall params in `EthTransactionFromSignedFilecoinMessage`; negative test for malformed CBOR present.

P1 — Estimation integration (DONE)
- EthEstimateGas
- For messages to EVM.ApplyAndCall with N tuples, expected gas includes intrinsic overhead (behavioral placeholder); tuple counting/gating tested.

P1 — E2E tests (gated behind eip7702_enabled; run when wasm bundle includes EVM ApplyAndCall)
- Send-path routing
- Minimal 0x04 tx constructs ApplyAndCall params; mined receipt echoes `authorizationList` and `delegatedTo`.
- Delegated execution
  - CALL→EOA executes delegate code (via actor/runtime), storage/logs reflect delegation.
 - Persistent storage
   - Delegated execution writes persist to the authority’s storage across transactions.
 - Atomicity/revert semantics
   - Validate whether delegation/nonces persist or roll back on outer call revert, aligned with the chosen spec behavior.

P2 — Edge/fuzz
- Fuzz RLP parsing for malformed tuples/fields (escalate breadth and coverage).
- Large authorizationList sizes for performance regressions.

P1 — RLP robustness (DONE)
- Negative tests added for canonical integer encodings (leading zeros) in tuple fields (chain_id and yParity) and parser enforces canonical forms.

P1 — Additional negative tests (DONE)
– `EthTransactionFromSignedFilecoinMessage` CBOR reconstruction: malformed tuple arity, empty list, invalid address length; strict decoder rejects shape/type mismatches.

Notes
- Keep gating aligned to a single NV constant shared with builtin-actors. Update gas constants/refunds in lockstep.
