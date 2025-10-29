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
  - Unit tests for delegated attribution from both `authorizationList` and synthetic `Delegated(address)` event.
- Mempool (Lotus):
  - No 7702-specific ingress policies. Ensure standard mempool behavior remains stable.
- Gas estimation (Lotus):
  - Gas model note (FVM): Lotus runs EVM as a Wasm actor on top of Filecoin’s gas system. We do not mirror Ethereum’s intrinsic gas accounting or constants in this branch. Estimation is behavioral and implementation‑defined.
  - Unit tests must avoid pinning numeric gas constants or absolute gas usage. Focus on tuple counting and gating in `node/impl/eth/gas_7702_scaffold.go`.
  - Prefer behavioral checks only: overhead applied when the authorization list is non‑empty; overhead grows monotonically with tuple count; no overhead when the feature is disabled or when the target is not EVM `ApplyAndCall`.
- E2E (Lotus):
  - Mirror geth’s `TestEIP7702` flow (apply two delegations, CALL→EOA executes delegate, storage updated) once the EVM actor ships ApplyAndCall.
- Actor validations (builtin‑actors/EVM):
  - Ensure chainId ∈ {0, local}, yParity ∈ {0,1}, non‑zero r/s, low‑s, ecrecover authority, nonce tracking; refunds and gas constants tests; enforce tuple cap.
- Additional review‑driven tests (builtin‑actors/EVM):
  - Event: topic hash for `Delegated(address)` and value = authority address (EOA), not delegate.
  - CALL revert path: propagate revert data via `state.return_data` and memory copy.
  - ApplyAndCall: fail outer call on failed value transfer to delegated EOA.
  - Negative validations: invalid `chainId`, invalid `yParity`, zero/over‑32 R/S, high‑S, nonce mismatch, duplicates, tuple cap >64.
  - Pre‑existence policy: reject when authority resolves to EVM contract.
  - R/S padding interop: accept 1..31‑byte values (left‑padded), reject >32‑byte values.

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

Gas Model (Lotus on FVM)
- EVM executes inside a Wasm actor; Filecoin consensus gas rules apply. We intentionally do not try to replicate Ethereum’s gas schedule here.
- Lotus `eth_estimateGas` behavior for 7702 is best‑effort and behavioral: we add an intrinsic overhead per authorization tuple for user experience. Exact constants are placeholders and may change.
- Tests must not assert exact gas usage or effective gas price; they should only assert the presence/absence of tuple overhead and its monotonicity with tuple count.
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

Detailed test plans are included below: see Builtin‑Actors Test Plan and Lotus Test Plan. These lists are part of the highest‑priority testing work for the sprint.

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
  - EVM runtime maintains pointer mapping and authority nonces; ApplyAndCall validates and updates mapping; interpreter handles EXTCODE* and emits `Delegated(address)`; gated by `NV_EIP_7702`.
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
    - On CALL→EOA, if the target account’s code starts with `0xef0100`, treat it as delegated code: load bytecode from the embedded 20‑byte address for execution (`InvokeAsEoa`), keep the authority context and emit the `Delegated(address)` event.
    - Ensure `EXTCODEHASH`, `EXTCODESIZE`, and code‑loading paths treat `0xef0100` accounts as “pointer” code (i.e., return the pointer code’s own hash/size when queried on the authority; only follow the pointer when executing). Follow the behavior in `revm_eip_7702.md`.
    - Implement `ApplyAndCall` that validates tuples (domain `0x05`, yParity ∈ {0,1}, non‑zero r/s, low‑s), enforces nonce equality, sets/clears pointer code, rejects non‑7702 pre‑existing code on authority, executes the outer call, emits the synthetic event, and reverts atomically on failure.
  - `runtime/src/features.rs`:
    - Gate all behavior behind the single `NV_EIP_7702` constant.

Required changes (tests):
- Lotus (Go)
  - `chain/types/ethtypes`: add unit tests for `AuthTupleHash` against known vectors; add negative tests for tuple arity and invalid `yParity`. (DONE)
  - `node/impl/eth` receipts: attribution tests from both `authorizationList` and synthetic `Delegated(address)` log. (DONE)
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
- Event compliance: topic = `keccak("Delegated(address)")` and the emitted address is the authority (EOA).
 - Pointer semantics: for delegated authority A→B, `EXTCODESIZE(A) == 23`, `EXTCODECOPY(A,0,0,23)` returns `0xEF 0x01 0x00 || <B(20)>`, and `EXTCODEHASH(A)` equals `keccak(pointer_code)`.
 - Delegated CALL revert data: CALL returns 0; `RETURNDATASIZE` equals revert payload length; `RETURNDATACOPY` truncates/returns per requested size with zero_fill=false semantics.

**Quick Validation**
- Lotus fast path:
  - `go build ./chain/types/ethtypes`
  - `go test ./chain/types/ethtypes -run 7702 -count=1`
  - `go test ./node/impl/eth -run 7702 -count=1`
- Builtin‑actors (local toolchain permitting): `cargo test -p fil_actor_evm` (EVM actor changes).
  - Includes pointer semantics and delegated revert‑data tests: `eoa_call_pointer_semantics.rs`, `delegated_call_revert_data.rs`.
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
  - MANDATORY pre‑push checks for CI stability (run in this order):
    - `make gen` — regenerate code, CBOR, inline/bundle, and fix imports.
    - `go fmt ./...` — format all Go code.
  - Do not commit/push to Lotus unless all three pass locally. CI rejects pushes that skip these steps.
- Builtin‑actors (paired repo at `../builtin-actors`):
  - Run `cargo fmt --all` to format Rust code consistently across crates.
  - Run `make check` to run clippy and enforce `-D warnings` across all crates and targets.

Notes:
- Keep formatting‑only changes in their own commits where feasible. Avoid mixing formatting with logic changes to keep diffs focused and reviewable.

**Acceptance Criteria**
- A signed type‑0x04 tx decodes, constructs a Filecoin message calling EVM.ApplyAndCall, applies valid delegations atomically with the outer call, and subsequent CALL→EOA executes delegate code.
- JSON‑RPC returns `authorizationList` and `delegatedTo` where applicable.
- Gas estimation accounts for tuple overhead (behavioral assertions).

**Env/Flags**
- Build tag: `eip7702_enabled` enables the 7702 send‑path in Lotus.
- Env toggles removed; EVM‑only routing is the default.

**Review Remediation TODOs (Detailed)**

- CRITICAL: CBOR Interoperability — R/S padding mismatch
  - Problem: Lotus encodes `r/s` as minimally‑encoded big‑endian; actor currently requires exactly 32‑byte values.
  - builtin‑actors (Actors/EVM):
    - Update `actors/evm/src/lib.rs`:
      - In `recover_authority`, if `len(r) > 32 || len(s) > 32` → illegal_argument; else left‑pad each to 32 bytes, then build `sig = r||s||v`.
      - In `validate_tuple`, change length validation to allow `≤32` and reject `>32` with precise errors.
      - In `is_high_s`, assume 32‑byte input (padded earlier) and assert length as needed.
    - Tests: add positive vectors for 1..31‑byte `r/s` (padded in actor) and negative for `>32`.
  - lotus (client): no encoder changes; add interop tests ensuring minimally‑encoded `r/s` round‑trip and are accepted by actor.
  - Acceptance: actor accepts ≤32‑byte `r/s`, rejects >32 with clear error; recovered authority matches padded case.

- HIGH: RLP Parsing Potential Overflow (Lotus)
  - Risk: `chain_id` and `nonce` fields in the 0x04 RLP tuple must support full `uint64` range.
  - lotus implementation: ensure `chain_id` and `nonce` are parsed using unsigned integer parsing that supports up to `math.MaxUint64` and avoid intermediate `int64` coercions.
  - Files: `chain/types/ethtypes/eth_7702_transactions.go` (tuple decode helpers).
  - Tests: extend decoder tests with values above `MaxInt64` and near `MaxUint64` to prove no overflow; retain canonical arity/yParity rejection tests.

- MEDIUM: Misleading error on signature length (Actors)
  - builtin‑actors: refactor error reporting in `validate_tuple` to check `r/s` length before low‑s/zero checks; return precise messages: “r length exceeds 32”, “s length exceeds 32”, “zero r/s”, “invalid y_parity”. Keep low‑s check on padded 32‑byte `s`.
  - Tests: negative vectors asserting error reasons for length >32 and invalid yParity.

- HIGH: Insufficient Actor Validation Tests
  - Add a dedicated `actors/evm/tests/apply_and_call_invalid.rs` suite covering:
    - Invalid `chainId` (not 0 or local).
    - Invalid `yParity` (>1).
    - Zero R or zero S.
    - High‑S rejection.
    - Nonce mismatch.
    - Pre‑existence policy violation (authority is an EVM contract).
    - Duplicate authority in a single message.

- MEDIUM: Missing Corner Case Tests (Actors)
  - SELFDESTRUCT no‑op in delegated context:
    - Build delegate bytecode that executes SELFDESTRUCT when called under InvokeAsEoa; assert no authority state/balance change; pointer mapping preserved; event emission intact.
  - Storage isolation/persistence on delegate changes:
    - A→B write storage; switch A→C and verify C cannot read B’s storage; clear A→0; re‑delegate A→B and verify B’s storage persists.
  - First‑time nonce handling:
    - Absent authority defaults to nonce=0; applying nonce=0 succeeds; next attempt with nonce=0 fails with nonce mismatch.

- LOW: Fuzzing and Vectors
  - lotus: add RLP fuzz harness for 0x04 decode focused on tuple arity/yParity/value sizes and malformed tails; ensure no panics and proper erroring.
  - builtin‑actors: add CBOR fuzz harness for `ApplyAndCallParams` that mutates wrapper shape, tuple arity, and byte sizes for `r/s`.
  - lotus: add AuthorizationKeccak test vectors for `keccak256(0x05 || rlp([chain_id,address,nonce]))` covering boundary values.

- Gas Model reminders (Lotus on FVM)
  - Do not pin exact gas constants or absolute gas usage in tests. Keep tests behavioral: overhead only when tuples present, monotonic with tuple count, and disabled when feature off or non‑ApplyAndCall targets.

Ownership and Acceptance (for this section)
- builtin‑actors: implement R/S padding, improve error clarity, add negative and corner‑case tests, add CBOR fuzz harness.
- lotus: implement RLP overflow robustness, add AuthorizationKeccak vectors, add RLP fuzz harness.
- Acceptance: all new tests pass; fuzz harnesses run without panics; interop for minimally‑encoded R/S validated; behavioral gas tests remain green.

**Builtin‑Actors Review (Action Items)**

This section captures additional items from the comprehensive review of `builtin-actors.eip7702.diff` and aligns them with this notebook.

- CRITICAL — Event semantics and topic (spec compliance)
  - Event name/signature: use `Delegated(address)` for the topic hash, not `EIP7702Delegated(address)`.
    - Files: `actors/evm/src/interpreter/instructions/call.rs`, `actors/evm/src/lib.rs`.
    - Action: change the string used for the topic hash to `b"Delegated(address)"`.
    - Lotus follow‑up: update `adjustReceiptForDelegation` to recognize the `Delegated(address)` topic.
    - Status: DONE — centralized as a shared constant and used in both emission sites.
  - Emitted address value: log the authority (EOA) address, not the delegate contract address.
    - Files: `actors/evm/src/interpreter/instructions/call.rs`, `actors/evm/src/lib.rs`.
    - Action: swap the value encoded in the event data from the delegate to the destination/authority.
    - Status: DONE — tests updated to assert authority address.

- HIGH — Behavioral correctness
  - Revert data propagation on delegated CALL failures
    - File: `actors/evm/src/interpreter/instructions/call.rs`.
    - Action: when `InvokeAsEoa` returns an error, set `state.return_data` from the error’s data and copy it to memory so callers can `RETURNDATACOPY`.
    - Status: DONE — delegated CALL path writes revert bytes to return buffer and copies to memory.
  - Value transfer result handling in `ApplyAndCall`
    - File: `actors/evm/src/lib.rs`.
    - Action: check the result of `system.transfer` for delegated EOA targets; if it fails, set `status: 0` and return immediately (mirrors CALL behavior).
    - Status: DONE — short‑circuit implemented and covered by tests.

- HIGH — Tests to add (consensus‑critical and behavioral)
  - `actors/evm/tests/apply_and_call_invalid.rs`: invalid `chainId`, `yParity > 1`, zero R/S, high‑S, nonce mismatch, duplicate authorities, exceeding 64‑tuple cap.
  - Pre‑existence policy: reject when authority resolves to an EVM contract actor (expect `USR_ILLEGAL_ARGUMENT`).
  - R/S padding interop: accept 1..31‑byte R/S (left‑padded) and reject >32 bytes.
  - Event correctness: assert topic = `keccak("Delegated(address)")` and that the indexed/address corresponds to the authority (EOA), not the delegate contract. (DONE)
  - Pointer semantics: `actors/evm/tests/eoa_call_pointer_semantics.rs` validates EXTCODESIZE=23, EXTCODECOPY exact bytes, and EXTCODEHASH of pointer code. (DONE)
  - Delegated CALL revert data: `actors/evm/tests/delegated_call_revert_data.rs` validates RETURNDATASIZE and RETURNDATACOPY truncation/full‑copy semantics. (DONE)

- MEDIUM — Interpreter corner cases (tests)
  - SELFDESTRUCT is a no‑op in delegated context; authority mapping/storage and balances unaffected; event emission intact.
  - Storage persistence/isolation across delegate changes: A→B write, A→C can’t read B; clear A→0; re‑delegate A→B and B’s storage persists.
  - Depth limit: nested delegations (depth > 1) are not followed under authority context.
  - First‑time nonce handling: absent authority treated as nonce=0; applying nonce=0 initializes nonces map; subsequent nonce=0 fails.

- MEDIUM — Robustness (internal invariants)
  - Return data deserialization must not silently fall back.
    - Files: `actors/evm/src/interpreter/instructions/call.rs`, `actors/evm/src/lib.rs`.
    - Action: replace `.unwrap_or_else(|_| r.data)` with mandatory decode; on failure, return `ActorError::illegal_state`.
    - Status: TODO
  - Avoid `unwrap()` in `extcodehash` path.
    - File: `actors/evm/src/interpreter/instructions/ext.rs`.
    - Action: handle `BytecodeHash::try_from(...)` errors by returning `ActorError::illegal_state` (or add a clear `expect` message at minimum).
    - Status: TODO

- LOW — Code quality improvements
  - Remove redundant R/S length checks from `recover_authority` (already validated in `validate_tuple`).
  - Strengthen `is_high_s` signature to `&[u8; 32]` to avoid runtime asserts.
  - Replace `expect` in actor code paths (e.g., resolved EVM address) with explicit error returns.
  - Downgrade normal execution‑path logging to `debug/trace` for failed transfers and delegate call failures.
  - Precompute `N/2` as a constant used by `is_high_s` (avoid recomputing at runtime).
    - File: `actors/evm/src/lib.rs`.
    - Action: introduce `N_DIV_2: [u8; 32]` (or equivalent) and compare against that constant.
    - Status: TODO
  - Centralize `InvokeContractReturn` type definition.
    - Files: `actors/evm/src/interpreter/instructions/call.rs`, `actors/evm/src/lib.rs`.
    - Action: move the struct to a shared module (e.g., `actors/evm/src/types.rs`) and reuse; remove local duplicates.
    - Status: TODO
  - Consolidate `Delegated(address)` event emission logic to a helper to remove duplication across success arms.
    - Files: `actors/evm/src/interpreter/instructions/call.rs`, `actors/evm/src/lib.rs`.
    - Action: factor event emission into a single helper and invoke once on success (regardless of return data presence).
    - Status: TODO

**Review Readiness (Scar‑less PR Candidate)**
- EVM‑only: all 0x04 transactions route to EVM `ApplyAndCall`; no Delegator send/execute path remains.
- Atomic‑only: there are no non‑atomic paths or fallback code; tests assert atomic semantics for success and revert.
- Clean surface: no optional env toggles for routing or legacy decoder branches; documentation and tests reference a single, canonical flow.

**Migration / Compatibility**
- No migration required. The implementation is EVM‑only and atomic‑only.
- Delegator has been removed; ApplyAndCall is the sole entry point.
- Single NV gate: `NV_EIP_7702`. Domain: `0x05`. Pointer code magic/version: `0xef 0x01 0x00`.

---

**Builtin‑Actors Test Plan (EVM‑Only)**

Scope: `../builtin-actors` (paired repo), tracked here for sprint execution. Keep encodings, NV gating, and gas constants in sync with Lotus.

Priority: P0 (blocking), P1 (recommended), P2 (nice‑to‑have)

P0 — Critical (spec + safety)
- Persistent delegated storage context (DONE)
  - `InvokeAsEoa` executes against the authority’s persistent storage and flushes changes; storage roots managed per authority in EVM state.
- Atomicity semantics (spec‑compliant persistence)
  - Delegation mapping and nonce bumps persist even if the outer call reverts. Tests assert persistence on revert and embedded status/return in ApplyAndCall result.
- Intrinsic gas charging (per tuple) (DONE)
  - Per‑authorization intrinsic gas charged before validation; behavior covered in tests.

P0 — ApplyAndCall core (DONE)
- Happy path (atomic apply+call) validates mapping/nonce behavior.
- Invalids rejected with `USR_ILLEGAL_ARGUMENT`: empty list, invalid chainId, invalid yParity, zero r/s, high‑s, nonce mismatch, duplicates.
- Tuple decoding shape (DAG‑CBOR): canonical atomic params; round‑trip tested.

P0 — EVM interpreter delegation (DONE)
- NV gating: pre‑activation ignores mapping; post‑activation executes delegate under authority context.
- Delegated execution: delegate writes storage; CALL→EOA executes delegate; event emitted; depth limited to 1.

P1 — Authorization semantics and state
- chainId handling: accept 0 (global) and local ChainID; reject others.
- Nonce accounting: absent authority treated as nonce=0; applying nonce=0 initializes; subsequent apply requires increment.
- Duplicate authorities in one message: rejected (DONE).
- Map/nonce HAMT integrity: flush/reload yields identical mappings and nonces.

P1 — Gas and refunds
- Defer asserting absolute numeric charges until constants are finalized; focus on behavior (paths invoked, no double‑charging across tuples).
- Refund behavior validated behaviorally; switch to numeric assertions once constants stabilize.
- Intrinsic gas OOG not asserted in unit tests.

P1 — Encoding and interop (DONE)
- Cross‑compat with Lotus: atomic CBOR params match; round‑trip vectors added.

P2 — Edge and fuzz
- Fuzz tuple decoding for arity/type issues.
- Large authorization lists (stress HAMT) within block gas limits.
- Malicious inputs: overlong leading zeros/odd sizes in r/s, etc.

Suggested test locations
- `actors/evm/tests/`
  - `apply_and_call_happy.rs` (P0)
  - `apply_and_call_invalid.rs` (P0)
  - `apply_and_call_tuple_roundtrip.rs` (P0)
  - `delegation_nonce_accounting.rs` (P1)
  - `eoa_call_pointer_semantics.rs` (P0) — DONE
  - `eip7702_delegated_log.rs` (P0)
  - `delegated_storage_persistence.rs` (P0)
  - `apply_and_call_atomicity_revert.rs` (P0)
  - `apply_and_call_intrinsic_gas.rs` (P1)
  - `apply_and_call_duplicates.rs` (P1)
  - `delegated_call_revert_data.rs` (P0) — DONE

Notes
- Keep `NV_EIP_7702` in `runtime/src/features.rs` as the single gate; mirror in Lotus.
- When gas constants change, update both repos and adjust tests together.

---

**Lotus Test Plan**

This list tracks Lotus‑side tests for EIP‑7702 and complements the builtin‑actors plan above.

Priority: P0 (now), P1 (soon), P2 (later)

P0 — Decisions & safety
- Mempool policy (DECIDED: document deviation)
  - No 7702‑specific ingress policies on this branch; standard mempool rules apply. Documented in this notebook/changelog.

P0 — Parsers and encoding
- RLP 0x04 parser/encoder
  - Round‑trip encode/decode; multi‑authorization list; empty `authorizationList` rejected; non‑empty `accessList` rejected; invalid outer `v` rejected; invalid auth `yParity` rejected; wrong tuple arity rejected; signature init from 65‑byte r||s||v.
- CBOR params (ApplyAndCall): `[ [tuple...], [to(20), value, input] ]`
  - Encoder produces canonical wrapper of 6‑tuples; compatible with actor decoder.

P0 — SignedMessage view + receipts
- Eth view reconstruction (DONE)
  - `EthTransactionFromSignedFilecoinMessage` reconstructs 0x04 (EVM.ApplyAndCall) and echoes `authorizationList`.
- Receipts attribution (DONE)
  - `adjustReceiptForDelegation` sets `delegatedTo` from `authorizationList` or synthetic `Delegated(address)` event.

P0 — Mempool (N/A on this branch)
- Cross‑account invalidation and per‑EOA cap not implemented; deviation documented.

P0 — Gas accounting (scaffold) (DONE)
- Counting + gating only; no absolute overhead assertions.
- `countAuthInDelegatorParams` handles canonical wrapper; tests cover gating and monotonicity.

P1 — JSON‑RPC plumbing (DONE)
- `eth_getTransactionReceipt` returns `authorizationList` and `delegatedTo`; covered in unit tests.
- Block/tx receipt flows call `adjustReceiptForDelegation`.
- EthTransaction reconstruction robustness: strict decoder for ApplyAndCall params with negative tests for malformed CBOR.

P1 — Estimation integration (DONE)
- `eth_estimateGas` adds intrinsic overhead for N tuples (behavioral placeholder); tuple counting/gating tested.

P1 — E2E tests (behind `eip7702_enabled`, run once wasm includes EVM ApplyAndCall)
- Send‑path routing constructs ApplyAndCall params; mined receipt echoes `authorizationList` and `delegatedTo`.
- Delegated execution: CALL→EOA executes delegate code via actor/runtime; storage/logs reflect delegation.
- Persistent storage across transactions under authority.
- Atomicity/revert semantics aligned with spec‑compliant persistence: mapping/nonces persist; receipt status reflects embedded status.

P2 — Edge/fuzz
- Fuzz RLP parsing for malformed tuples/fields; broaden coverage.
- Large `authorizationList` sizes for performance regressions.

P1 — RLP robustness (DONE)
- Negative tests for canonical integer encodings (leading zeros) in tuple fields; parser enforces canonical forms.

P1 — Additional negative tests (DONE)
- ApplyAndCall CBOR reconstruction: malformed tuple arity, empty list, invalid address length; strict decoder rejects shape/type mismatches.

Notes
- Keep gating aligned to a single NV constant shared with builtin‑actors.
- Update gas constants/refunds in lockstep once finalized.
