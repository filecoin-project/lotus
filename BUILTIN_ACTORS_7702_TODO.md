# Builtin-Actors EIP-7702 Testing TODO (EVM‑Only)

Scope: ../builtin-actors (paired repo), tracked here for sprint execution. Keep encodings, NV gating, and gas constants in sync with Lotus.

Priority order: P0 (blocking launch), P1 (strongly recommended), P2 (nice-to-have)

P0 — Critical gaps (spec + safety)
- Persistent delegated storage context (DONE)
  - `InvokeAsEoa` executes against the Authority’s persistent storage root and flushes changes; storage roots managed per authority in EVM state. Test added.
- Atomicity semantics decision (DECIDED: atomic rollback; tests added)
  - On outer call revert, delegation/nonce updates do not persist. Added test to confirm nonce unchanged after failure.
- Intrinsic gas charging (per-tuple) (DONE)
  - Per-authorization intrinsic gas charged before validation; behavioral test added.

P0 — EVM actor ApplyAndCall core (DONE)
– ApplyAndCall happy path (atomic apply+call)
  - Covered via success path in atomicity test second run; mapping/nonce behavior validated.
– ApplyAndCall invalids (reject with USR_ILLEGAL_ARGUMENT)
  - Empty list, invalid chainId, invalid yParity, zero r/s, high‑s; nonce mismatch already exercised; duplicates rejected.
– Tuple decoding shape (DAG‑CBOR)
  - Canonical atomic params decode; added round‑trip test.

P0 — EVM interpreter delegation (DONE)
– NV gating
  - Pre‑activation: CALL to EOA does not consult delegation map; post‑activation: consults mapping and executes delegate code under authority context.
– Delegated execution
  - Delegate code writes storage; CALL→EOA executes delegate; storage persisted (covered via InvokeAsEoa + ApplyAndCall tests). Event emission path present; nested delegation depth limited in interpreter.

P1 — Authorization semantics and state
- chainId handling
  - Accept chainId = 0 (global) and local ChainID; reject any other value.
- Nonce accounting
  - First-time authority (absent nonce) is treated as 0; apply nonce=0 succeeds; subsequent apply requires increment.
  - Multiple tuples for the same authority in one message: either forbid as invalid or specify semantics (test accordingly).
- Duplicate authorities behavior (DECIDED: reject; DONE)
  - Duplicate authority entries in one message are rejected. Test added.
- Map/nonce HAMT integrity
  - Flushing updates roots; reloading state yields same mappings and nonces.

P1 — Gas and refunds (align with revm/geth)
- Per-authorization overhead and base cost
  - Defer asserting absolute numeric charges (e.g., PER_AUTH_BASE_COST, PER_EMPTY_ACCOUNT_COST) until constants are finalized; focus current tests on behavior (paths invoked, no double‑charging across tuples).
- Refund behavior
  - Validate behavioral semantics (e.g., refunds occur in the right conditions) without relying on exact gas numbers; switch to numeric assertions once constants stabilize.
- Intrinsic gas OOG
  - Behavioral coverage present via charge expectations; OOG failure not asserted in unit tests.

P1 — Encoding and interop (DONE)
– Cross‑compat with Lotus
  - CBOR atomic params match Lotus; round‑trip vectors added.

P2 — Edge and fuzz
- Fuzz tuple decoding for arity/type issues.
- Large auth lists (stress HAMT operations) within block gas limits.
- Malicious inputs: overlong leading zeros in r/s, odd sizes, etc.

 Test locations (suggested)
- actors/evm/tests/
  - apply_and_call_happy.rs (P0)
  - apply_and_call_invalid.rs (P0)
  - apply_and_call_tuple_roundtrip.rs (P0)
  - delegation_nonce_accounting.rs (P1)
  - eoa_call_pointer_semantics.rs (P0)
  - eip7702_delegated_log.rs (P0)
  - delegated_storage_persistence.rs (P0) — added
  - apply_and_call_atomicity_revert.rs (P0) — added
  - apply_and_call_intrinsic_gas.rs (P1) — added
  - apply_and_call_duplicates.rs (P1) — added
- cross-repo vectors (stored in Lotus, consumed here)
  - lotus/chain/types/ethtypes/… encoded CBOR samples for decoder tests (P1)

Notes
- Keep NV_EIP_7702 in `runtime/src/features.rs` as the single source of truth; mirror in Lotus gating.
- If gas constants change, update both repos and adjust tests accordingly.
