# Builtin-Actors EIP-7702 Testing TODO (EVM‑Only)

Scope: ../builtin-actors (paired repo), tracked here for sprint execution. Keep encodings, NV gating, and gas constants in sync with Lotus.

Priority order: P0 (blocking launch), P1 (strongly recommended), P2 (nice-to-have)

P0 — EVM actor ApplyAndCall core
- ApplyAndCall happy path (atomic apply+call)
  - Single tuple applies mapping and bumps nonce; outer call executes; mapping persists.
  - Multiple tuples across multiple authorities; each mapping set; each nonce bumped.
- ApplyAndCall invalids (reject with USR_ILLEGAL_ARGUMENT)
  - Empty list.
  - chainId not in {0, local}.
  - yParity not in {0,1}.
  - r = 0 or s = 0.
  - High‑s (s > secp256k1n/2).
  - Nonce mismatch (expected ≠ provided) for an existing authority.
  - Signature recovery fails (malformed r/s/v or wrong digest).
- Tuple decoding shape (DAG‑CBOR)
  - Atomic params `[ [ tuple... ], [ to(20), value, input ] ]` decode.
  - Round‑trip: encode params → decode → equal.

P0 — EVM interpreter delegation
- NV gating
  - Pre‑activation (network_version < NV_EIP_7702): CALL to EOA does not consult delegation map.
  - Post‑activation: CALL to EOA consults internal map and executes delegate code under authority context.
- Delegated execution
  - Delegate code writes storage; CALL→EOA executes delegate; assert storage changed at authority’s address.
  - Attribution log: emit `EIP7702Delegated(address)`.
  - Nested delegation behavior covered or explicitly limited.

P1 — Authorization semantics and state
- chainId handling
  - Accept chainId = 0 (global) and local ChainID; reject any other value.
- Nonce accounting
  - First-time authority (absent nonce) is treated as 0; apply nonce=0 succeeds; subsequent apply requires increment.
  - Multiple tuples for the same authority in one message: either forbid as invalid or specify semantics (test accordingly).
- Map/nonce HAMT integrity
  - Flushing updates roots; reloading state yields same mappings and nonces.

P1 — Gas and refunds (align with revm/geth)
- Per-authorization overhead and base cost
  - Defer asserting absolute numeric charges (e.g., PER_AUTH_BASE_COST, PER_EMPTY_ACCOUNT_COST) until constants are finalized; focus current tests on behavior (paths invoked, no double‑charging across tuples).
- Refund behavior
  - Validate behavioral semantics (e.g., refunds occur in the right conditions) without relying on exact gas numbers; switch to numeric assertions once constants stabilize.

P1 — Encoding and interop
- Cross‑compat with Lotus
  - CBOR atomic params match Lotus encoder; round‑trip vectors decode here.

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
- cross-repo vectors (stored in Lotus, consumed here)
  - lotus/chain/types/ethtypes/… encoded CBOR samples for decoder tests (P1)

Notes
- Keep NV_EIP_7702 in `runtime/src/features.rs` as the single source of truth; mirror in Lotus gating.
- If gas constants change, update both repos and adjust tests accordingly.
