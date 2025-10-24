# EIP-7702 Implementation Notebook (Lotus + Builtin Actors)

This notebook tracks the end‑to‑end EIP‑7702 implementation across Lotus (Go) and builtin‑actors (Rust), including current status, remaining tasks, and validation steps.

**Purpose**
- Provide a concise, actionable plan to complete EIP‑7702.
- Document current status, remaining work, and how to validate.

**Paired Repos**
- `./lotus` (this folder)
- `../builtin-actors` (paired repo in a neighboring folder)

**Testing TODO (Highest Priority)**
- Cross‑repo scope: tests span `./lotus` and `../builtin-actors`. Keep encoding, gating, and gas constants aligned.
- Parser/encoding (lotus):
  - Add tuple‑arity and yParity rejection cases for 0x04 RLP decode in `chain/types/ethtypes`.
  - Canonicalize to wrapper CBOR only: actor and Go helpers accept `[ [ tuple, ... ] ]` exclusively; remove legacy array‑of‑tuples acceptance.
  - Cross‑package CBOR compatibility between encoder and actor remains via wrapper form.
- Receipts attribution (lotus):
  - Unit tests for delegated attribution in `node/impl/eth/receipt_7702_scaffold.go` from both `authorizationList` and synthetic log topic.
- Mempool (lotus):
  - No 7702-specific ingress policies. Ensure standard mempool behavior remains stable.
 - Gas estimation (lotus):
  - Unit tests should focus on tuple counting and gating in `node/impl/eth/gas_7702_scaffold.go`.
  - Do not assert absolute numeric gas values (e.g., base overhead 21000, `PerAuthBaseCost`, `PerEmptyAccountCost`) in unit tests until actor/runtime constants are finalized and mirrored across repos.
  - Prefer behavioral checks: overhead is only applied when `AuthorizationList` is non‑empty; overhead increases monotonically with tuple count; no overhead when feature is disabled or target is not Delegator.
- E2E (lotus):
  - Mirror geth’s `TestEIP7702` flow (apply two delegations, CALL→EOA executes delegate, storage updated) in `itests` once the Delegator actor is in the bundle.
- Actor validations (builtin‑actors):
  - Ensure chainId ∈ {0, local}, yParity ∈ {0,1}, non‑zero r/s, low‑s, ecrecover authority, nonce tracking; add refunds and gas constants tests.

**Audit Remediation Plan (Gemini)**

- Atomic 0x04 semantics
  - Implement atomic “apply authorizations + execute outer call” for type‑0x04 within a single transaction.
  - Builtin‑actors: add an atomic entry (e.g., `ApplyAndCall`) or ensure Delegator writes occur just before the call and revert atomically on failure.
  - Lotus: route 0x04 to atomic path in `ToSignedFilecoinMessage`; update gas estimation to simulate both phases atomically.
  - Tests: revert path reverts both delegation and call; mirror geth’s `TestEIP7702`.

- RLP per‑type decode limit
  - Replace global `maxListElements = 13` with a per‑call limit; apply 13 only for 0x04 parsing.
  - Add unit tests proving no regression for legacy/1559 decoders.

- Canonical CBOR (consensus safety)
  - Actor: accept wrapper `[ [ tuple, ... ] ]` only; remove dual‑shape detection in Go helper.
  - Negative tests: malformed top‑level shapes, wrong tuple arity, invalid yParity.

- FVM runtime hardening
  - InvokeAsEoa: add storage‑context stack invariants, re‑entrancy guard, and delegation depth limit; restrict `PutStorageRoot` to EVM internal context.
  - Pointer code semantics: CALL executes delegate; EXTCODESIZE/HASH reflect pointer code; emit `EIP7702Delegated(address)`.
  - Authorization domain/signatures: enforce `keccak256(0x05 || rlp([chain_id,address,nonce]))`, low‑s, non‑zero r/s, yParity ∈ {0,1}, nonce equality.
  - DoS guard: consensus cap on tuples per message (e.g., 50–100); document constant and align estimation.

- Lotus follow‑ups
  - Receipts/RPC: ensure receipts/logs reflect atomic apply+call; maintain `delegatedTo` attribution (tuples or synthetic event).
  - Gas estimation: model atomic flow; maintain behavioral tests only until constants finalize.
  - Parser/encoding: add `AuthorizationKeccak` test vectors for preimage/hash stability.

- Networking and limits
  - Rely on actor‑level tuple cap; keep general `MaxMessageSize` as is. Optionally validate tuple count client‑side for fast‑fail.

- Gating and constants
  - Unify activation gating with a single `NV_EIP_7702` across both repos; keep constants in sync.

**Work Breakdown (Sequenced)**
1. RLP per‑type limit in Lotus + tests.
2. Canonical CBOR wrapper only (actor + Go helpers) + negative tests.
3. Implement atomic 0x04 semantics (actors + Lotus route + receipts) + e2e test.
4. FVM hardening (InvokeAsEoa guards; pointer semantics; domain/signature checks; tuple cap) + tests.
5. Gas estimation alignment to actor constants; maintain behavioral tests until finalization.
6. Doc updates and gating unification; add test vectors for `AuthorizationKeccak`.

For a detailed builtin‑actors test plan, see `BUILTIN_ACTORS_7702_TODO.md` (tracked here but applies to `../builtin-actors`). For Lotus‑specific tests, see `LOTUS_7702_TODO.md`. These lists are part of the highest‑priority testing work for the sprint.

References for parity:
- `geth_eip_7702.md` (diff: TestEIP7702, intrinsic gas, empty auth errors)
- `revm_eip_7702.md` (auth validity, gas constants, delegation code handling)

**Status Overview**
- Lotus/Go (done):
  - Typed 0x04 parsing/encoding with `authorizationList`; dispatch via `ParseEthTransaction`.
  - `EthTx` and receipts echo `authorizationList`; receipt adjuster surfaces `delegatedTo` from tuples or synthetic event.
  - Send path wired behind `-tags eip7702_enabled`: builds a Filecoin message targeting Delegator.ApplyDelegations with CBOR‑encoded tuples.
  - Mempool policies (cross‑account invalidation; per‑EOA cap), gated by network version.
  - Gas estimation scaffold adds intrinsic overhead per tuple.
  - Go‑side Delegator helpers: tuple decode/validation (chainId, yParity, low‑s), state apply with nonce checks (for tests).
- Builtin‑actors/Rust (done):
  - Delegator actor with HAMT‑backed mapping and authority nonces; methods: Constructor, ApplyDelegations (decode, validate, ecrecover, nonce check, write), LookupDelegate, Get/PutStorageRoot.
  - EVM runtime CALL→EOA hook: consults Delegator; executes delegate via `InvokeAsEoa`; emits `EIP7702Delegated(address)` for attribution; gated by `NV_EIP_7702`.
- In‑progress alignment:
  - CBOR params shape aligned to actor: wrapper tuple `ApplyDelegationsParams{ list: Vec<DelegationParam> }`.
  - Lotus decoder and gas counter accept both top‑level array and wrapper forms for robustness.
  - Default Delegator actor address when feature is enabled is `ID:18` (env override supported).
  - Gating unified to a single NV constant placeholder to avoid drift.

**Spec Gap: MAGIC/Version Prefix Compliance**
- Problem: We currently omit the required MAGIC prefixing in two places required by the spec and parity references (`eip7702.md`, `revm_eip_7702.md`).
  - Authorization tuple signing domain: inner tuple signatures must be over `keccak256(0x05 || rlp([chain_id, address, nonce]))` where `0x05` is the authorization domain separator (aka `SetCodeAuthorizationMagic`).
  - Delegation indicator bytecode: when applying a delegation, the authority account code must be set to `0xef 0x01 0x00 || <20‑byte delegate address>`, where `0xef01` is the EIP‑7702 bytecode MAGIC and `0x00` is the version.

Required changes (implementation):
- Lotus (Go)
  - `chain/types/ethtypes/`:
    - Add constants: `SetCodeTxType = 0x04`, `SetCodeAuthorizationMagic = 0x05`, `Eip7702BytecodeMagic = 0xef01`, `Eip7702BytecodeVersion = 0x00`.
    - Implement `AuthTupleHash(chainID, address, nonce) = keccak256(0x05 || rlp([chainID, address, nonce]))` and use it wherever we validate or preview inner signatures (tests and helpers only; authoritative validation happens in actor).
    - Extend RLP decode tests to cover tuple arity/yParity rejection and ensure the above hash is stable across edge cases (zero/Big endian, etc.).
  - `chain/actors/builtin/delegator/` (Go helpers):
    - Update signature recovery/validation helpers to use `SetCodeAuthorizationMagic` domain separation when computing the preimage for `ecrecover`.
    - Add `IsEip7702Code([]byte) bool` that returns true iff code has prefix `0xef0100` and length 23.
  - `node/impl/eth/receipt_7702_scaffold.go` and `node/impl/eth/gas_7702_scaffold.go`:
    - No numeric changes; add checks that attribution/gas logic is guarded by presence of valid `authorizationList` and does not assume delegation unless `IsEip7702Code` or explicit tuple attribution is present.

- Builtin‑actors (Rust)
  - `actors/delegator/`:
    - In `ApplyDelegations`, compute the signed message as `keccak256(0x05 || rlp([chain_id, address, nonce]))` before `ecrecover`. Enforce `y_parity ∈ {0,1}`, non‑zero `r/s`, low‑`s` and nonce equality.
    - On success, set authority code to EIP‑7702 bytecode: `0xef 0x01 0x00 || delegate_address` (23 bytes total). If `delegate_address == 0x0`, clear code (remove delegation).
    - Reject any pre‑existing non‑empty, non‑EIP‑7702 code on the authority.
    - Provide a small `is_eip7702_code(&[u8]) -> bool` helper in the crate to centralize prefix/version checks.
  - `actors/evm/` runtime:
    - On CALL→EOA, if the target account’s code starts with `0xef0100`, treat it as delegated code: load bytecode from the embedded 20‑byte address for execution (`InvokeAsEoa`), keep the authority context and emit the `EIP7702Delegated(address)` event.
    - Ensure `EXTCODEHASH`, `EXTCODESIZE`, and code‑loading paths treat `0xef0100` accounts as “pointer” code (i.e., return the pointer code’s own hash/size when queried on the authority; only follow the pointer when executing). Follow the behavior in `revm_eip_7702.md`.
  - `runtime/src/features.rs`:
    - Gate all behavior behind the single `NV_EIP_7702` constant.

Required changes (tests):
- Lotus (Go)
  - `chain/types/ethtypes`: add unit tests for `AuthTupleHash` against known vectors; add negative tests for tuple arity and invalid `yParity`.
  - `chain/actors/builtin/delegator`: unit tests that the Go helpers recover the same authority as the Rust actor when using the `0x05` domain prefix, enforce low‑s, and reject zero `r/s`.
  - `node/impl/eth/receipt_7702_scaffold.go`: add attribution tests from both `authorizationList` and synthetic `EIP7702Delegated(address)` log.
  - `node/impl/eth/gas_7702_scaffold.go`: behavioral tests only (no absolute gas constants) ensuring overhead applies only when `authorizationList` is non‑empty and grows monotonically with tuple count.

- Builtin‑actors (Rust)
  - `actors/delegator` tests:
    - Valid path: applies delegation, sets code to `0xef0100||addr`, increments nonce, and `LookupDelegate` returns the delegate address.
    - Invalid signature domain: same tuple signed without the `0x05` prefix must be rejected.
    - Invalid `y_parity`/high‑`s`/zero `r,s`/wrong chain id/nonce mismatch must be rejected.
    - Pre‑existing non‑EIP‑7702 code on authority must be rejected.
    - Refund/accounting tests for `PER_EMPTY_ACCOUNT_COST − PER_AUTH_BASE_COST` when authority exists in trie.
  - `actors/evm` tests:
    - CALL to EOA with `0xef0100` code executes delegate and updates storage; `EXTCODESIZE/HASH` reflect pointer code on the authority; synthetic event is emitted for attribution.

Validation notes
- Keep the following aligned across repos and tests:
  - `SetCodeAuthorizationMagic = 0x05` (authorization domain)
  - `Eip7702BytecodeMagic = 0xef01` and `Eip7702BytecodeVersion = 0x00` (delegation indicator)
  - Gas constants: treat as placeholders until finalized; do only behavioral assertions in Lotus.
  - Activation gating: single NV constant used in both repos.

**What Remains**
- Gas constants/refunds: finalize authoritative costs in actor/runtime and mirror in Lotus estimation (placeholders currently used in Lotus).
- E2E tests in Lotus once wasm bundle is buildable in this environment; validate: 0x04 tx applies delegations; CALL→EOA executes delegate; receipts/logs attribution; policies behave as expected post‑activation.
- Optional receipt polish depending on explorer requirements.

**Quick Validation**
- Lotus fast path:
  - `go build ./chain/types/ethtypes`
  - `go test ./chain/types/ethtypes -run 7702 -count=1`
  - `go test ./chain/actors/builtin/delegator -count=1`
- Builtin‑actors (local toolchain permitting): `cargo test`.
- Lotus E2E (requires FFI + Delegator in bundle):
  - `go test ./itests -run Eth7702 -tags eip7702_enabled -count=1`
  - Test `TestEth7702_SendRoutesToDelegator` is implemented and currently skipped by default until the Delegator actor is included in the network bundle in this environment.

To route 0x04 transactions, build Lotus with `-tags eip7702_enabled` and set `LOTUS_ETH_7702_DELEGATOR_ADDR` (defaults to `ID:18` when enabled).

**Files & Areas**
- Lotus:
  - `chain/types/ethtypes/` (tx parsing/encoding; CBOR params; types)
  - `node/impl/eth/` (send path; gas estimation; receipts)
  - `chain/messagepool/` (generic mempool; no 7702-specific policies)
  - `chain/actors/builtin/delegator/` (Go helpers for validation/testing)
- Builtin‑actors:
  - `actors/delegator/` (actor implementation; tests)
  - `actors/evm/` (runtime CALL delegation; `InvokeAsEoa`)
  - `runtime/src/features.rs` (activation NV)

**Editing Strategy**
- Keep diffs small and scoped. Mirror existing style (e.g., 1559 code) where possible.
- When changing encodings, update both encoder/decoder and tests; prefer backward‑compatible decoders.
- Unify activation gating across repos to a single NV constant and avoid hard‑coding disparate values.

**Commit Guidance**
- Commit in small, semantic units with clear messages; avoid batching unrelated changes.
- Prefer separate commits for code, tests, and docs when practical.
- Commit frequently to preserve incremental intent; summarize scope and rationale in the subject.
- Push after each atomic change so reviewers can follow intent and history stays readable.
- Keep history readable: no formatting‑only changes mixed with logic changes.
- Pair commits with pushes regularly to keep the remote branch current (e.g., push after each semantic commit or small group of related commits). Coordinate with PR reviews to avoid large, monolithic pushes.

**Acceptance Criteria**
- A signed type‑0x04 tx decodes, constructs a Filecoin message calling Delegator.ApplyDelegations, applies valid delegations, and subsequent CALL→EOA executes delegate code.
- JSON‑RPC returns `authorizationList` and `delegatedTo` where applicable.
 - Gas estimation accounts for tuple overhead.

**Env/Flags**
- Build tag: `eip7702_enabled` enables send‑path in Lotus.
- Env: `LOTUS_ETH_7702_DELEGATOR_ADDR` for Delegator actor address (defaults to `ID:18` when enabled).
