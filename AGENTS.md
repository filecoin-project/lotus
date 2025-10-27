# EIP-7702 Implementation Notebook (Lotus + Builtin Actors)

This notebook tracks the end‑to‑end EIP‑7702 implementation across Lotus (Go) and builtin‑actors (Rust), including current status, remaining tasks, and validation steps.

**Purpose**
- Provide a concise, actionable plan to complete EIP‑7702.
- Document current status, remaining work, and how to validate.

**Paired Repos**
- `./lotus` (this folder)
- `../builtin-actors` (paired repo in a neighboring folder)

**Branch Scope & Compatibility**
- This is an internal development branch with no external users or on‑chain compatibility requirements.
- We do not preserve backward compatibility in this branch. It is acceptable to simplify encodings and remove legacy paths.
- EVM-only on this branch. The EVM actor’s `ApplyAndCall` atomically applies authorizations and executes the outer call; no Delegator route is used.
- Encodings are canonicalized: wrapper/atomic CBOR only; legacy shapes removed from tests/helpers.

**Testing TODO (Highest Priority)**
- Cross‑repo scope: tests span `./lotus` and `../builtin-actors`. Keep encoding, gating, and gas constants aligned.
- Parser/encoding (Lotus):
  - Add tuple‑arity and yParity rejection cases for 0x04 RLP decode in `chain/types/ethtypes`.
  - Canonicalize to atomic CBOR only: `[ [ tuple... ], [ to(20), value, input ] ]`; remove legacy shapes.
  - Add `AuthorizationKeccak` (0x05 domain) vectors for stability across edge cases.
- Receipts attribution (Lotus):
  - Unit tests for delegated attribution from both `authorizationList` and synthetic `EIP7702Delegated(address)` event.
- Mempool (Lotus):
  - No 7702-specific ingress policies. Ensure standard mempool behavior remains stable.
- Gas estimation (Lotus):
  - Unit tests should focus on tuple counting and gating in `node/impl/eth/gas_7702_scaffold.go`.
  - Prefer behavioral checks: overhead only when list non‑empty; monotonic with tuple count; no overhead when feature disabled or target is not 7702 ApplyAndCall.
- E2E (Lotus):
  - Mirror geth’s `TestEIP7702` flow (apply two delegations, CALL→EOA executes delegate, storage updated) once the EVM actor ships ApplyAndCall.
- Actor validations (builtin‑actors/EVM):
  - Ensure chainId ∈ {0, local}, yParity ∈ {0,1}, non‑zero r/s, low‑s, ecrecover authority, nonce tracking; refunds and gas constants tests; enforce tuple cap.

**Audit Remediation Plan (Gemini)**

- Atomic 0x04 semantics
  - Implement atomic “apply authorizations + execute outer call” for type‑0x04 within a single transaction.
  - Builtin‑actors: `ApplyAndCall` in the EVM actor validates, applies pointer code, enforces nonces, and executes the outer call atomically.
  - Lotus: route 0x04 to EVM `ApplyAndCall` exclusively; no env toggles; gas estimation simulates tuple overhead behaviorally.
  - Tests: revert path reverts both delegation and call; mirror geth’s `TestEIP7702`.

- RLP per‑type decode limit
  - Replace global `maxListElements = 13` with a per‑call limit; apply 13 only for 0x04 parsing.
  - Add unit tests proving no regression for legacy/1559 decoders.

- Canonical CBOR (consensus safety)
  - Actor: accept wrapper `[ [ tuple, ... ] ]` only; remove dual‑shape detection in Go helper.
  - Negative tests: malformed top‑level shapes, wrong tuple arity, invalid yParity.

- EVM runtime hardening (EVM‑only)
  - `ApplyAndCall` in EVM actor (present). Runtime delegated CALL pointer semantics and event emission (present); EXTCODESIZE/HASH/COPY expose virtual 23‑byte pointer code for delegated EOAs.
  - Interpreter guards for authority context; depth limit on delegation; storage safety.
  - Authorization domain/signatures; tuple cap; refunds.

- Lotus follow‑ups
  - Receipts/RPC: ensure receipts/logs reflect atomic apply+call; maintain `delegatedTo` attribution (tuples or synthetic event). RPC reconstruction supports EVM `ApplyAndCall` route.
  - Gas estimation: model atomic flow; behavioral tests only until constants finalize. Overhead applied for EVM route.
  - Parser/encoding: add `AuthorizationKeccak` test vectors for preimage/hash stability.

- Networking and limits
  - Rely on actor‑level tuple cap; keep general `MaxMessageSize` as is. Optionally validate tuple count client‑side for fast‑fail.

- Gating and constants
  - Unify activation gating with a single `NV_EIP_7702` across both repos; keep constants in sync.

**Work Breakdown (Sequenced)**
1. RLP per‑type limit in Lotus + tests. (DONE)
2. Canonical CBOR wrapper only (actor + Go helpers) + negative tests. (DONE)
3. Implement atomic 0x04 semantics (EVM.ApplyAndCall + Lotus route + receipts) + e2e test. (ApplyAndCall, optional Lotus route, receipts unit tests DONE; E2E PENDING)
4. FVM hardening (InvokeAsEoa guards; pointer semantics; domain/signature checks; tuple cap) + tests. (PARTIAL)
5. Gas estimation alignment to actor constants; maintain behavioral tests until finalization. (DONE)
6. Doc updates and gating unification; add test vectors for `AuthorizationKeccak`. (IN PROGRESS)

For a detailed builtin‑actors test plan, see `BUILTIN_ACTORS_7702_TODO.md` (tracked here but applies to `../builtin-actors`). For Lotus‑specific tests, see `LOTUS_7702_TODO.md`. These lists are part of the highest‑priority testing work for the sprint.

References for parity:
- `geth_eip_7702.md` (diff: TestEIP7702, intrinsic gas, empty auth errors)
- `revm_eip_7702.md` (auth validity, gas constants, delegation code handling)

**Status Overview**
- Lotus/Go (done):
  - Typed 0x04 parsing/encoding with `authorizationList`; dispatch via `ParseEthTransaction`.
  - `EthTx` and receipts echo `authorizationList`; receipt adjuster surfaces `delegatedTo` from tuples or synthetic event.
  - Send path behind `-tags eip7702_enabled`: builds a Filecoin message targeting EVM.ApplyAndCall with CBOR‑encoded tuples.
  - Mempool policies (cross‑account invalidation; per‑EOA cap), gated by network version.
  - Gas estimation scaffold adds intrinsic overhead per tuple.
  - Branch note: EVM-only routing; Delegator is not used for send/execute path on this branch.
- Builtin‑actors/Rust (done):
  - EVM runtime maintains pointer mapping and authority nonces; ApplyAndCall validates and updates mapping; interpreter handles EXTCODE* and emits `EIP7702Delegated(address)`; gated by `NV_EIP_7702`.
- In‑progress alignment:
  - CBOR params: canonical wrapper only; decoder enforces wrapper shape.
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
  - Go helpers cleaned up; Delegator helpers removed.
  - `node/impl/eth/receipt_7702_scaffold.go` and `node/impl/eth/gas_7702_scaffold.go`:
    - No numeric changes; add checks that attribution/gas logic is guarded by presence of valid `authorizationList` and does not assume delegation unless `IsEip7702Code` or explicit tuple attribution is present.

- Builtin‑actors (Rust)
  - EVM runtime manages pointer semantics and nonces; `ApplyAndCall` validates, applies, and executes atomically.
  - `actors/evm/` runtime:
    - On CALL→EOA, if the target account’s code starts with `0xef0100`, treat it as delegated code: load bytecode from the embedded 20‑byte address for execution (`InvokeAsEoa`), keep the authority context and emit the `EIP7702Delegated(address)` event.
    - Ensure `EXTCODEHASH`, `EXTCODESIZE`, and code‑loading paths treat `0xef0100` accounts as “pointer” code (i.e., return the pointer code’s own hash/size when queried on the authority; only follow the pointer when executing). Follow the behavior in `revm_eip_7702.md`.
    - Implement `ApplyAndCall` that validates tuples (domain `0x05`, yParity ∈ {0,1}, non‑zero r/s, low‑s), enforces nonce equality, sets/clears pointer code, rejects non‑7702 pre‑existing code on authority, executes the outer call, emits the synthetic event, and reverts atomically on failure.
  - `runtime/src/features.rs`:
    - Gate all behavior behind the single `NV_EIP_7702` constant.

Required changes (tests):
- Lotus (Go)
  - `chain/types/ethtypes`: add unit tests for `AuthTupleHash` against known vectors; add negative tests for tuple arity and invalid `yParity`. (DONE)
  - `node/impl/eth` receipts: attribution tests from both `authorizationList` and synthetic `EIP7702Delegated(address)` log. (DONE)
  - `node/impl/eth` gas: behavioral tests ensuring overhead applies only when tuple list is non‑empty and grows with tuple count; include Delegator + EVM routes. (DONE)

- Builtin‑actors (Rust)
  - `actors/evm` tests:
    - CALL to EOA with `0xef0100` code executes delegate and updates storage; `EXTCODESIZE/HASH` reflect pointer code on the authority; synthetic event is emitted for attribution.
    - ApplyAndCall happy path and revert atomicity; negative vectors for invalid domain/yParity/high‑s/zero r,s/wrong chain id/nonce mismatch; refund/accounting as specified. (INITIAL)

Validation notes
- Keep the following aligned across repos and tests:
  - `SetCodeAuthorizationMagic = 0x05` (authorization domain)
  - `Eip7702BytecodeMagic = 0xef01` and `Eip7702BytecodeVersion = 0x00` (delegation indicator)
  - Gas constants: treat as placeholders until finalized; do only behavioral assertions in Lotus.
  - Activation gating: single NV constant used in both repos.

**What Remains**
- Gas constants/refunds: finalize authoritative costs in actor/runtime and mirror in Lotus estimation (behavioral tests only for now).
- E2E tests in Lotus once wasm bundle is buildable; validate: 0x04 applies delegations atomically and outer call executes (delegated where applicable); receipts/logs attribution; policies OK post‑activation.
- Optional receipt polish depending on explorer requirements.
- Pre‑review cleanup (scar‑less): remove Delegator and non‑atomic code paths; make EVM `ApplyAndCall` the sole entry; drop env toggles used for development; scrub docs/tests for single canonical flow.

---

### Actionable TODOs (Handoff Plan)

This section captures the concrete next steps confirmed in review, for handoff to the next implementer. Items are grouped by criticality and include implementation hints and ownership.

Decisions (confirmed)
- Atomicity: adopt spec‑compliant persistence — delegation mapping + nonce bumps MUST persist even if the outer call reverts.
- Pre‑existence policy: reject delegations where the authority resolves to an EVM contract actor (Filecoin analogue of “code must be empty/pointer”).
- Tuple cap: enforce a placeholder cap of 64 tuples in a single message (document as placeholder; tune/remove later).
- Mempool policy: no 7702‑specific ingress policies on this branch (documented deviation); standard rules apply.
- Refunds: staged approach — add plumbing + conservative caps now; switch to numeric constants once finalized.

CRITICAL (consensus/security/spec)
- ApplyAndCall atomicity (spec compliance)
  - builtin‑actors: DONE — mapping + nonces persist before outer call; always Exit OK with embedded status/return.
  - lotus: DONE — receipt.status is decoded from the embedded status for type‑0x04; attribution unchanged.
  - tests: IN PROGRESS — atomic revert persistence asserted at unit level; e2e to follow once wasm lands.
- Pre‑existence policy (do not “set code” on contracts)
  - builtin‑actors: In ApplyAndCall, for each authority, resolve to ID and reject when builtin type == EVM (USR_ILLEGAL_ARGUMENT). Add a negative test.
- Nested delegation depth limit
  - builtin‑actors: DONE — Enforced depth==1. When executing under InvokeAsEoa/authority context, delegation chains are not followed. ApplyAndCall-driven unit test present.
- Signature robustness (length + low‑s)
  - builtin‑actors: Ensure R/S are exactly 32 bytes (already enforced) and add explicit negative tests for invalid lengths. Keep low‑s and non‑zero checks.

HIGH PRIORITY (correctness/robustness)
- Refunds (staged)
  - builtin‑actors: STAGED — refund plumbing points present; constants/caps to be wired once finalized.
  - lotus: Keep estimation behavioral until constants finalize; wire estimates to refunds when available.
- Tuple cap (placeholder)
  - builtin‑actors: DONE — Enforces `len(params.list) <= 64`; boundary tests added. Placeholder noted in code comments.
- SELFDESTRUCT interaction
  - builtin‑actors: Add tests where delegated code executes SELFDESTRUCT; specify and assert expected behavior for authority mapping/storage and gas.
- Fuzzing
  - lotus: Add RLP fuzzing for 0x04 tx decode.
  - builtin‑actors: Add CBOR fuzzing harness for ApplyAndCall params.
- E2E lifecycle (post‑wasm)
  - lotus: Enable full flow once wasm includes ApplyAndCall: apply delegations, CALL→EOA executes delegate, storage persists under authority, event emitted, receipts reflect `authorizationList`, `delegatedTo`, and correct `status`.

MEDIUM PRIORITY (completeness/tests)
- EOA storage persistence: expand coverage
  - builtin‑actors: Add tests for (a) switching delegates (A→B then A→C) and verifying B’s storage persists; (b) clearing delegation and verifying storage remains accessible on re‑delegation.
- First‑time authority nonce handling
  - builtin‑actors: Add an explicit test that an absent authority is treated as nonce=0; applying nonce=0 succeeds and initializes the nonces map.
- Estimation parity
  - lotus: In E2E, compare `eth_estimateGas` results against mined consumption to ensure intrinsic overhead and (later) refunds behave reasonably.

Ownership and ordering
- builtin‑actors (consensus): atomicity (spec compliance), pre‑existence check, depth limit, tuple cap, refunds plumbing, signature tests, SELFDESTRUCT tests, fuzzer.
- lotus (client): receipt `status` from ApplyAndCall return, E2E lifecycle, RLP fuzzer, estimation wiring for refunds when ready.

Acceptance criteria (updated)
- A type‑0x04 tx persists delegation mapping + nonces even when the outer call reverts; Lotus sets receipt status to 0 accordingly.
- Pre‑existence check rejects authorities that are EVM contracts.
- No nested delegation chains are followed (depth=1 enforced).
- Tuple cap of 64 enforced (placeholder); large lists rejected early.
- Refund plumbing present with conservative caps; numeric constants can be dropped in once finalized.

**Quick Validation**
- Lotus fast path:
  - `go build ./chain/types/ethtypes`
  - `go test ./chain/types/ethtypes -run 7702 -count=1`
  - `go test ./node/impl/eth -run 7702 -count=1`
- Builtin‑actors (local toolchain permitting): `cargo test -p fil_actor_evm` (EVM actor changes).
- Lotus E2E (requires updated wasm bundle):
  - `go test ./itests -run Eth7702 -tags eip7702_enabled -count=1`

To route 0x04 transactions in development, build Lotus with `-tags eip7702_enabled`.

**Files & Areas**
- Lotus:
  - `chain/types/ethtypes/` (tx parsing/encoding; CBOR params; types)
  - `node/impl/eth/` (send path; gas estimation; receipts)
  - `chain/messagepool/` (generic mempool; no 7702-specific policies)
- Builtin‑actors:
  - `actors/evm/` (ApplyAndCall; CALL pointer semantics; EXTCODE* behavior; event emission)
  - `runtime/src/features.rs` (activation NV)

**Editing Strategy**
- Keep diffs small and scoped. Mirror existing style (e.g., 1559 code) where possible.
- When changing encodings, update encoder/decoder and tests; no backward‑compatibility is required on this branch. Drop legacy/dual shapes in favor of canonical forms.
- Unify activation gating across repos to a single NV constant and avoid hard‑coding disparate values.

**Commit Guidance**
- Commit in small, semantic units with clear messages; avoid batching unrelated changes.
- Prefer separate commits for code, tests, and docs when practical.
- Commit frequently to preserve incremental intent; summarize scope and rationale in the subject.
- Push after each atomic change so reviewers can follow intent and history stays readable.
- Keep history readable: no formatting‑only changes mixed with logic changes.
- Pair commits with pushes regularly to keep the remote branch current (e.g., push after each semantic commit or small group of related commits). Coordinate with PR reviews to avoid large, monolithic pushes.

**Pre‑Commit Checklist (Tooling/Formatting)**
- Lotus (this repo):
  - Run `go fmt ./...` to format Go code.
  - Run `make check` to execute static checks and linters.
- Builtin‑actors (paired repo at `../builtin-actors`):
  - Run `cargo fmt --all` to format Rust code consistently across crates.

Notes:
- Keep formatting‑only changes in their own commits where feasible. Avoid mixing formatting with logic changes to keep diffs focused and reviewable.

**Acceptance Criteria**
- A signed type‑0x04 tx decodes, constructs a Filecoin message calling EVM.ApplyAndCall, applies valid delegations atomically with the outer call, and subsequent CALL→EOA executes delegate code.
- JSON‑RPC returns `authorizationList` and `delegatedTo` where applicable.
- Gas estimation accounts for tuple overhead (behavioral assertions).

**Env/Flags**
- Build tag: `eip7702_enabled` enables the 7702 send‑path in Lotus.
- Env toggles removed; EVM‑only routing is the default.

**Review Readiness (Scar‑less PR Candidate)**
- EVM‑only: all 0x04 transactions route to EVM `ApplyAndCall`; no Delegator send/execute path remains.
- Atomic‑only: there are no non‑atomic paths or fallback code; tests assert atomic semantics for success and revert.
- Clean surface: no optional env toggles for routing or legacy decoder branches; documentation and tests reference a single, canonical flow.

**Migration / Compatibility**
- No migration required. The implementation is EVM‑only and atomic‑only.
- Delegator has been removed; ApplyAndCall is the sole entry point.
- Single NV gate: `NV_EIP_7702`. Domain: `0x05`. Pointer code magic/version: `0xef 0x01 0x00`.
