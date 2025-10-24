# Builtin-Actors EIP-7702 Testing TODO

Scope: ../builtin-actors (paired repo), tracked here for sprint execution. Keep encodings, NV gating, and gas constants in sync with Lotus.

Priority order: P0 (blocking launch), P1 (strongly recommended), P2 (nice-to-have)

P0 — Delegator actor core
- ApplyDelegations happy path
  - Single tuple applies mapping and bumps nonce; mapping persists across calls.
  - Multiple tuples in one call across multiple authorities; each mapping set; each nonce bumped.
- ApplyDelegations invalids (reject with USR_ILLEGAL_ARGUMENT)
  - Empty list.
  - chainId not in {0, local}.
  - yParity not in {0,1}.
  - r = 0 or s = 0.
  - High-s (s > secp256k1n/2).
  - Nonce mismatch (expected ≠ provided) for an existing authority.
  - Signature recovery fails (malformed r/s/v or wrong digest).
- Tuple decoding shape (DAG-CBOR)
  - Wrapper `ApplyDelegationsParams{ list: Vec<DelegationParam> }` decodes.
  - Round-trip: encode params → decode → equal.
- LookupDelegate
  - Returns None before apply; Some(delegate) after apply.
- Storage roots
  - PutStorageRoot only callable by EVM actor type; others forbidden.
  - GetStorageRoot None before put; returns exact CID after put. Use DAG-CBOR where specified.

P0 — EVM runtime delegation
- NV gating
  - Pre-activation (network_version < NV_EIP_7702): CALL to EOA does not consult Delegator.
  - Post-activation: CALL to EOA consults Delegator and executes delegate code via InvokeAsEoa.
- Delegated execution
  - Delegate code that writes a storage slot; CALL→EOA executes delegate; assert storage changed at delegate’s address.
  - Attribution log: EVM emits an ETH-style log `topic0 = keccak("EIP7702Delegated(address)")`, `data = delegate (20b)`.
  - Ensure nested delegate call path works (delegate A calls into delegate B) if supported; otherwise document limitation.

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
- Cross-compat with Lotus
  - CBOR/tuple shape matches Lotus encoder; round-trip vectors from Lotus decode here.
  - FRC-42 method numbers for Lookup/Get/Put stable and used by EVM.

P2 — Edge and fuzz
- Fuzz tuple decoding for arity/type issues.
- Large auth lists (stress HAMT operations) within block gas limits.
- Malicious inputs: overlong leading zeros in r/s, odd sizes, etc.

Test locations (suggested)
- actors/delegator/tests/
  - apply_and_lookup_mapping_happy.rs (P0)
  - apply_invalid_cases.rs (P0)
  - tuple_shape_roundtrip.rs (P0)
  - nonce_accounting.rs (P1)
  - hamt_persistence.rs (P1)
- actors/evm/tests/
  - eoa_invoke_delegation_nv.rs (P0)
  - eoa_invoke_delegation_storage.rs (P0)
  - eip7702_delegated_log.rs (P0)
- cross-repo vectors (stored in Lotus, consumed here)
  - lotus/chain/types/ethtypes/… encoded CBOR samples for decoder tests (P1)

Notes
- Keep NV_EIP_7702 in `runtime/src/features.rs` as the single source of truth; mirror in Lotus gating.
- If gas constants change, update both repos and adjust tests accordingly.
