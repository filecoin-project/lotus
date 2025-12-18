# EIP-7702 Implementation Notebook (Lotus + Builtin Actors)

IMPORTANT — Current Work Priority

- The authoritative plan for the ongoing migration (EthAccount state + ref-fvm delegation) is tracked in `documentation/eip7702_ethaccount_ref-fvm_migration.md`.
- This is the active focus for the branch. Use that document for scope, sequencing, and acceptance criteria. The content below remains as background and historical context.

This notebook tracks the end‑to‑end EIP‑7702 implementation across Lotus (Go) and builtin‑actors (Rust), including current status, remaining tasks, and validation steps.

**Purpose**
- Provide a concise, actionable plan to complete EIP‑7702.
- Document current status, remaining work, and how to validate.

**Progress Sync (current)**
- Event encoding: receipts now expect topic keccak("Delegated(address)") and a 32‑byte ABI word for the address. Lotus adjuster extracts the last 20 bytes and tolerates both 20/32‑byte encodings during the transition.
- Routing: type‑0x04 now targets EthAccount.ApplyAndCall (FRC‑42 method hash) with the canonical CBOR wrapper. EVM.ApplyAndCall routing is deprecated on this branch.
- Runtime helper: ref‑fvm exposes `get_eth_delegate_to(ActorID) -> Option<[u8;20]>`; EVM EXTCODE* now consults this helper to project the 23‑byte pointer image.
- VM intercept: ref‑fvm now intercepts CALL→EOA to EthAccount when `delegate_to` is set; it executes the delegate under authority context (depth=1), mounts/persists the authority `evm_storage_root`, and emits `Delegated(address)` (32‑byte ABI) best‑effort.
- Interpreter minimalization: EVM CALL/STATICCALL to EOAs no longer follows delegation internally; delegation is handled by the VM intercept. Legacy EVM ApplyAndCall/InvokeAsEoa have been removed on this branch; only the `InvokeAsEoaWithRoot` trampoline remains for VM intercept use.
- EthAccount: state struct added (`delegate_to`, `auth_nonce`, `evm_storage_root`) and ApplyAndCall implemented with full tuple validation and receiver‑only persistence; outer call forwards gas and returns embedded status/returndata. Initial unit tests added (invalid yParity/lengths, nonce init/increment, atomicity on revert, value transfer short‑circuit).
- Tests: Lotus 7702 receipts updated and green; ref‑fvm helper unit test added; EVM EXTCODE* tests continue to pass with the Runtime helper.

Docker bundle/test flow
- Use `../ref-fvm/scripts/run_eip7702_tests.sh` to build the builtin-actors bundle in Docker and run ref‑fvm tests end‑to‑end. This avoids macOS toolchain issues (e.g., env::__stack_chk_fail, blst section too large).
- Fallback (local, non‑Docker): in `../builtin-actors`, try `RUSTFLAGS="-C link-arg=-Wl,-dead_strip" make bundle-testing` or `SKIP_BUNDLE=1 cargo build -p fil_builtin_actors_bundle` for compile‑time paths only (note: tests that execute EVM still need a real bundle).

Next up
- Expand/enable ref‑fvm unit tests for intercept semantics (delegated mapping success/revert + revert bytes, depth limit, EXTCODECOPY windowing/zero‑fill, SELFDESTRUCT no‑op); finalize cleanup of any remaining legacy stubs.

---

Coverage Improvement Plan (ref‑fvm)

Context
- Codecov patch coverage flagged low coverage across recent changes in ref‑fvm, especially in:
  - `fvm/src/call_manager/default.rs`
  - `sdk/src/actor.rs`
  - `fvm/src/kernel/default.rs`
  - `fvm/src/syscalls/actor.rs`
- Some CI coverage steps ran with `--no-default-features`, skipping delegated CALL intercept paths and undercounting coverage.

Goals
- Patch coverage ≥ 80% on changed files.
- Project coverage back near pre‑change baseline; fix upload parity vs. base.

Actions (sequenced)
1) SDK helper + tests
   - Extract the 20‑byte address slicing into a pure helper in `sdk/src/actor.rs` and add unit tests covering lengths {0, <20, 20, >20} to avoid relying on syscalls.
2) Kernel/call‑manager paths
   - Ensure existing delegated tests cover success, revert, value‑transfer short‑circuit, depth limit, and EXTCODE* pointer image with windowing. These already exist under `fvm/tests/*`; confirm they run under the coverage profile.
3) CI coverage job
   - For the `test-fvm` coverage step, run with default features (remove `--no-default-features`) so delegated paths are exercised and reported. Keep other steps as‑is to avoid feature unification side‑effects.
4) Syscall wrapper spots
   - Opportunistic coverage via existing flows (resolve_address, get_actor_code_cid). Add minimal negative‑path assertions if needed.
5) Upload parity
   - Ensure the same number of coverage uploads as base (Linux primary). If base had macOS uploads, mirror them or disable redundant ones consistently.

Execution Notes
- Keep minimal‑build fallbacks in tests guarded so coverage runs (default features) take the asserting paths.
- Commit in small steps and keep this AGENTS.md in sync after each significant change.

Status (to hand off)
- Initial test adjustments for minimal builds landed; CI green.
- SDK: added pure extractor + unit tests for `eth20` slicing in `sdk/src/actor.rs`.
- CI: coverage step for `fvm` now runs with default features to exercise delegated paths.
- Update: to avoid OpenCL linkage on Ubuntu runners, restored `--no-default-features` on the `test-fvm` coverage step. Delegation tests remain compatible with minimal builds. If patch coverage stays low for kernel/call-manager, add targeted unit tests that don't require default features.
- Next: validate Codecov patch % (target ≥80% on changed files) and project coverage; add any missing edge tests if needed.

Updates (coverage work in progress)
- Added fvm crate tests for send paths that hit DefaultCallManager branches:
  - Create placeholder actor (f4) via METHOD_SEND + value=0, then transfer non-zero.
  - Create BLS account actor via METHOD_SEND + value=0, then transfer non-zero.
- Added fvm unit tests for keccak32/frc42 helpers inside call_manager/default.rs.
- Expect improved patch coverage in call_manager; continue adding focused tests if needed.

**Paired Repos**
- `./lotus` (this folder)
- `../builtin-actors` (paired repo in a neighboring folder)

**Branch Scope & Compatibility**
- This is an internal development branch with no external users or on‑chain compatibility requirements.
- We do not preserve backward compatibility in this branch. It is acceptable to simplify encodings and remove legacy paths.
- Routing and semantics have moved to EthAccount + VM intercept. Type‑0x04 transactions target `EthAccount.ApplyAndCall` (FRC‑42 method hash with canonical CBOR params). Delegated CALL/EXTCODE* behavior is implemented in ref‑fvm; the EVM interpreter no longer follows delegation internally.
- There is no Delegator actor and no single‑EVM‑actor delegation map. Delegation state lives per‑EOA in EthAccount state (`delegate_to`, `auth_nonce`, `evm_storage_root`). Global pointer semantics are provided by the VM intercept reading EthAccount state.
- Encodings are canonicalized: wrapper/atomic CBOR only; legacy shapes removed from tests/helpers.

**EthAccount Delegation State (current)**
- Ownership and persistence: EthAccount persists three fields in its state: `delegate_to: Option<EthAddress>`, `auth_nonce: u64`, and `evm_storage_root: Cid`. These survive across transactions until updated or cleared (zero delegate clears).
- Apply‑and‑Call flow: `EthAccount.ApplyAndCall` validates tuples (domain 0x05, chainId in {0, local}, yParity ∈ {0,1}, non‑zero/≤32‑byte r/s with left‑padding, low‑s), recovers the `authority`, enforces nonce equality, updates `delegate_to`/`auth_nonce` (receiver‑only), initializes `evm_storage_root` if needed, then invokes the outer call via VM with all gas forwarded. It returns embedded status/returndata. Mapping/nonces persist even if the outer call reverts.
- Pointer semantics: When a CALL/EXTCODE* targets an EOA with `delegate_to` set, ref‑fvm intercepts and executes the delegate under authority context (depth=1). EXTCODESIZE/HASH/COPY expose a 23‑byte virtual code image (`0xEF 0x01 0x00 || delegate(20)`).
- Authority context and safety: In authority context, delegation is not re‑followed (depth limit = 1), SELFDESTRUCT is a no‑op (no tombstone or balance move), and storage is mounted using the authority’s persistent storage root, then persisted on success.

**ApplyAndCall Outer Call Gas**
- Top‑level behavior: For the outer call executed by `EthAccount.ApplyAndCall`, ref‑fvm forwards all available gas (no 63/64 cap), mirroring Ethereum’s top‑level semantics.
- Subcalls: Inside the interpreter, subcalls (CALL/STATICCALL/DELEGATECALL) still enforce the EIP‑150 63/64 gas clamp.
- Consequence: Emitting the `Delegated(address)` event from the VM intercept is best‑effort. Under extreme gas tightness, the event may be dropped.
- Rationale: Prioritize correctness of the callee’s execution budget for the outer call. If telemetry shows attribution logs are frequently dropped, consider reserving a small fixed gas budget for event emission (not a 63/64 clamp).

**Testing TODO (Highest Priority)**
- Cross‑repo scope: tests span `./lotus` and `../builtin-actors`. Keep encoding and gas behavior aligned.
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
  - Negative validations: invalid `chainId`, invalid `yParity`, zero/over‑32 R/S, high‑S, nonce mismatch, duplicates, tuple cap >64. (DONE)
  - Pre‑existence policy: reject when authority resolves to EVM contract. (DONE)
  - R/S padding interop: accept 1..31‑byte values (left‑padded), reject >32‑byte values. (DONE)

**Audit Remediation Plan (Gemini)**

- Atomic 0x04 semantics
  - Implement atomic “apply authorizations + execute outer call” for type‑0x04 within a single transaction.
  - Builtin‑actors: `EthAccount.ApplyAndCall` validates tuples, enforces nonces, persists `delegate_to`/`auth_nonce` (receiver‑only), and invokes the outer call via VM atomically.
  - Lotus: route 0x04 to EthAccount `ApplyAndCall` exclusively; no env toggles; gas estimation simulates tuple overhead behaviorally.
  - Tests: revert path returns embedded status/revert data while delegation/nonce updates persist; mirror geth’s `TestEIP7702`.

- RLP per‑type decode limit
  - Replace global `maxListElements = 13` with a per‑call limit; apply 13 only for 0x04 parsing.
  - Add unit tests proving no regression for legacy/1559 decoders.

- Canonical CBOR (consensus safety)
  - Actor: accept wrapper `[ [ tuple, ... ] ]` only; remove dual‑shape detection in Go helper.
  - Negative tests: malformed top‑level shapes, wrong tuple arity, invalid yParity.

- Runtime hardening (EVM + VM intercept)
  - Delegated CALL pointer semantics and event emission are implemented in ref‑fvm; EXTCODESIZE/HASH/COPY expose a virtual 23‑byte pointer code for delegated EOAs.
  - Interpreter guards for authority context; depth limit on delegation; storage safety.
  - Authorization domain/signatures; tuple cap; refunds.

- Lotus follow‑ups
  - Receipts/RPC: ensure receipts/logs reflect atomic apply+call; maintain `delegatedTo` attribution (tuples or synthetic event). RPC reconstruction supports EthAccount `ApplyAndCall` route.
- Gas estimation: model atomic flow; behavioral tests only until constants finalize. Overhead applied for EthAccount route.

Gas Model (Lotus on FVM)
- EVM executes inside a Wasm actor; Filecoin consensus gas rules apply. We intentionally do not try to replicate Ethereum’s gas schedule here.
- Lotus `eth_estimateGas` behavior for 7702 is best‑effort and behavioral: we add an intrinsic overhead per authorization tuple for user experience. Exact constants are placeholders and may change.
- Tests must not assert exact gas usage or effective gas price; they should only assert the presence/absence of tuple overhead and its monotonicity with tuple count.
  - Parser/encoding: add `AuthorizationKeccak` test vectors for preimage/hash stability.

- Networking and limits
  - Rely on actor‑level tuple cap; keep general `MaxMessageSize` as is. Optionally validate tuple count client‑side for fast‑fail.

- Gating and constants
  - Activation: via bundle; no runtime NV gates to maintain.

**Work Breakdown (Sequenced)**
1. RLP per‑type limit in Lotus + tests. (DONE)
2. Canonical CBOR wrapper only (actor + Go helpers) + negative tests. (DONE)
3. Implement atomic 0x04 semantics (EthAccount.ApplyAndCall + Lotus route + receipts) + e2e test. (ApplyAndCall, optional Lotus route, receipts unit tests DONE; E2E PENDING)
4. FVM hardening (InvokeAsEoa guards; pointer semantics; domain/signature checks; tuple cap) + tests. (PARTIAL)
5. Gas estimation alignment to actor constants; maintain behavioral tests until finalization. (DONE)
6. Doc updates and test vector additions for `AuthorizationKeccak`. (IN PROGRESS)

Detailed test plans are included below: see Builtin‑Actors Test Plan and Lotus Test Plan. These lists are part of the highest‑priority testing work for the sprint.

References for parity:
- `geth_eip_7702.md` (diff: TestEIP7702, intrinsic gas, empty auth errors)
- `revm_eip_7702.md` (auth validity, gas constants, delegation code handling)

- Lotus/Go (done):
  - Typed 0x04 parsing/encoding with `authorizationList`; dispatch via `ParseEthTransaction`.
  - `EthTx` and receipts echo `authorizationList`; receipt adjuster surfaces `delegatedTo` from tuples or `Delegated(address)` event.
  - Send path behind `-tags eip7702_enabled`: builds a Filecoin message targeting EthAccount.ApplyAndCall with canonical CBOR params.
  - Mempool policies (cross‑account invalidation; per‑EOA cap). No runtime NV gate on this branch.
  - Gas estimation scaffold adds intrinsic overhead per tuple (behavioral only).
- Builtin‑actors/Rust (updated):
  - EVM interpreter consults Runtime helper for EXTCODE* pointer projection; no unwraps; windowing semantics enforced.
  - EthAccount state added; ApplyAndCall validates tuples (domain 0x05, yParity, non‑zero/≤32 R/S, low‑S), enforces nonce equality, persists `delegate_to` and `auth_nonce` (receiver‑only), initializes `evm_storage_root`, and executes the outer call; mapping persists on revert. Initial unit tests added.
  - DONE: Legacy EVM delegation paths removed. `EVM.ApplyAndCall` and `InvokeAsEoa` are removed/stubbed; only `InvokeAsEoaWithRoot` remains for VM intercept trampolines. Interpreter does not re‑follow delegation.
- ref‑fvm (updated):
  - Runtime/Kernel/SDK helper `get_eth_delegate_to(ActorID)` implemented with strict EthAccount code check; unit test added.
  - VM intercept implemented: delegated CALL → execute delegate under authority context (depth=1); mount/persist `evm_storage_root`; emit `Delegated(address)` event (ABI 32‑byte); map success/revert and propagate revert data; EXTCODE* projects 23‑byte pointer code with windowing/zero‑fill semantics. Tests added for windowing, depth limit, value‑transfer short‑circuit, and storage overlay persistence.

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
    - Receipts adjuster updated to topic keccak("Delegated(address)"); data read as 32‑byte ABI word (last 20 bytes extracted); attribution prefers tuples; tolerates missing events. Gas logic remains behavioral and guarded by non‑empty `authorizationList` and target=EthAccount.ApplyAndCall.

- Builtin‑actors (Rust)
  - EVM interpreter: EXTCODE* pointer projection via Runtime helper; `0xef0100` virtual code exposed; windowing and zero‑fill semantics enforced; no unwraps.
  - EthAccount.ApplyAndCall: tuple validation as above; receiver‑only persistence; outer call forwards gas; events will be emitted from VM intercept.
  - No runtime network‑version gating; activation via bundle.

Required changes (tests):
- Lotus (Go)
  - `chain/types/ethtypes`: AuthTupleHash vectors; tuple arity/yParity negatives. (DONE)
  - `node/impl/eth` receipts: attribution via tuples or `Delegated(address)` (32‑byte ABI data tolerant). (DONE)
  - `node/impl/eth` gas: behavioral overhead tests guarded by tuple count and target. (DONE)

- Builtin‑actors (Rust)
  - `actors/evm` tests stay green with Runtime helper in EXTCODE* paths.
  - `actors/ethaccount` tests added: invalid yParity/lengths, nonce init/increment, atomicity on revert, value transfer short‑circuit. More to be ported as VM intercept lands.

Validation notes
- Keep the following aligned across repos and tests:
  - `SetCodeAuthorizationMagic = 0x05` (authorization domain)
  - `Eip7702BytecodeMagic = 0xef01` and `Eip7702BytecodeVersion = 0x00` (delegation indicator)
  - Gas constants: treat as placeholders until finalized; do only behavioral assertions in Lotus.
  - Activation: via bundle; no runtime NV gates.

**What Remains**
- VM intercept (ref‑fvm): DONE — delegated CALL → execute delegate under authority context with depth=1; mount/persist `evm_storage_root`; best‑effort `Delegated(address)` event (ABI 32‑byte); success/revert mapping; EXTCODE* projection with windowing/zero‑fill.
- EVM minimalization: DONE — interpreter does not re‑follow delegation; legacy `InvokeAsEoa`/`EVM.ApplyAndCall` removed; only `InvokeAsEoaWithRoot` trampoline remains for VM intercept.
- Gas constants/refunds: finalize numbers (behavioral in Lotus until then).
- Lotus E2E: validate atomic apply+call, delegated execution, and receipts/logs attribution once the wasm bundle is integrated.

Follow‑ups from 2025‑11‑13 review (status)
- EthAccount → VM outer‑call bridge + E2E (DONE):
  - builtin‑actors/ref‑fvm: DONE — EthAccount.ApplyAndCall routes outer calls to EVM contracts via the `InvokeEVM` entrypoint (see `../builtin-actors/actors/ethaccount/src/lib.rs` and `../builtin-actors/actors/ethaccount/tests/apply_and_call_outer_call.rs`), and delegated CALL semantics are exercised via the VM intercept and associated tests in `../ref-fvm/fvm/src/call_manager/default.rs` and `../ref-fvm/fvm/tests`.
  - lotus: DONE — `itests/TestEth7702_DelegatedExecute` is enabled (under the `eip7702_enabled` build tag) and asserts the full 0x04 → EthAccount.ApplyAndCall → delegated CALL→EOA lifecycle, including authority storage overlay, `Delegated(address)` event emission, receipts attribution, and status mapping.
- Signature padding positives (DONE):
  - builtin‑actors: Explicit EthAccount tests now accept minimally‑encoded big‑endian `r/s` with lengths 1..31 and 32 bytes (left‑padded internally), in addition to the existing >32‑byte and zero‑value rejection tests, locking in interoperability with Lotus’ minimal encoding. See `../builtin-actors/actors/ethaccount/tests/apply_and_call_rs_padding.rs`.
- Delegated(address) event coverage (DONE):
  - ref‑fvm: An integration test exercises a delegated CALL→EOA path, then inspects the emitted events (via `EventsRoot`) to assert:
    - topic0 = `keccak256("Delegated(address)")`, and
    - data is a 32‑byte ABI word whose last 20 bytes equal the authority (EOA) address.
    See `../ref-fvm/fvm/tests/delegated_event_emission.rs`.
- Naming/documentation cleanup (PARTIAL):
  - lotus/builtin‑actors/ref‑fvm: Code and test comments describing live routing now consistently reference `EthAccount.ApplyAndCall` + VM intercept, and Delegator/EVM.ApplyAndCall paths are marked removed or historical. Some standalone design notes (e.g., early EVM‑only explanation docs) still describe the older EVM.ApplyAndCall‑centric design and will be updated or explicitly marked historical in a later documentation pass.

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
  - builtin‑actors: In ApplyAndCall, for each authority, resolve to ID and reject when builtin type == EVM (USR_ILLEGAL_ARGUMENT). Add a negative test. (DONE)
- Nested delegation depth limit
  - builtin‑actors: DONE — Enforced depth==1. When executing under authority context (via `InvokeAsEoaWithRoot`), delegation chains are not followed. ApplyAndCall-driven unit test present.
- Signature robustness (length + low‑s)
  - builtin‑actors: Accept ≤32‑byte R/S (left‑pad to 32); reject >32 with precise errors; keep low‑s and non‑zero checks. Negative tests added. (DONE)

HIGH PRIORITY (correctness/robustness)
- Refunds (staged)
  - builtin‑actors: STAGED — refund plumbing points present; constants/caps to be wired once finalized.
  - lotus: Keep estimation behavioral until constants finalize; wire estimates to refunds when available.
- Tuple cap (placeholder)
  - builtin‑actors: DONE — Enforces `len(params.list) <= 64`; boundary tests added. Placeholder noted in code comments.
- SELFDESTRUCT interaction
  - builtin‑actors: Add tests where delegated code executes SELFDESTRUCT; specify and assert expected behavior for authority mapping/storage and gas. (DONE)
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
- ref‑fvm (preferred):
  - `../ref-fvm/scripts/run_eip7702_tests.sh`
  - The script first builds the builtin‑actors bundle in Docker with `ref-fvm` mounted (to satisfy local patch paths), then runs ref‑fvm tests. If host tests fail, it falls back to running tests inside Docker.
  - If Docker is unavailable, bundle build will fail on macOS; proceed with builtin‑actors tests while enabling Docker.
- Builtin‑actors (local toolchain permitting):
  - `cargo test -p fil_actor_evm` (EVM runtime changes; EXTCODE* helper usage).
  - `cargo test -p fil_actor_ethaccount` (EthAccount state + ApplyAndCall tests: invalids, nonces, atomicity, value transfer).
- Lotus E2E (requires updated wasm bundle):
  - `go test ./itests -run Eth7702 -tags eip7702_enabled -count=1`

- Docker bundle + ref‑fvm tests (recommended on macOS):
  - Convenience script: `../ref-fvm/scripts/run_eip7702_tests.sh`
    - Builds the builtin‑actors bundle in Docker (`make bundle-testing-repro`).
    - Runs ref‑fvm tests, including EXTCODE* windowing, depth limit, value‑transfer short‑circuit, and revert payload propagation.
  - Fallback (no Docker): set
    `CC_wasm32_unknown_unknown=/opt/homebrew/opt/llvm/bin/clang AR_wasm32_unknown_unknown=/opt/homebrew/opt/llvm/bin/llvm-ar RANLIB_wasm32_unknown_unknown=/opt/homebrew/opt/llvm/bin/llvm-ranlib RUSTFLAGS="-C link-arg=-Wl,-dead_strip"`
    and run `cargo test -p fvm`. Note: some EVM Wasm invocations may fail on macOS due to `__stack_chk_fail`.

To route 0x04 transactions in development, build Lotus with `-tags eip7702_enabled`.

**Files & Areas**
- Lotus:
  - `chain/types/ethtypes/` (tx parsing/encoding; CBOR params; types)
  - `node/impl/eth/` (send path; gas estimation; receipts)
  - `chain/messagepool/` (generic mempool; no 7702-specific policies)
- Builtin‑actors:
  - `actors/evm/` (ApplyAndCall; CALL pointer semantics; EXTCODE* behavior; event emission)
  - `runtime/src/features.rs` (activation doc)

**Editing Strategy**
- Keep diffs small and scoped. Mirror existing style (e.g., 1559 code) where possible.
- When changing encodings, update encoder/decoder and tests; no backward‑compatibility is required on this branch. Drop legacy/dual shapes in favor of canonical forms.
  

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
- A signed type‑0x04 tx decodes, constructs a Filecoin message calling EthAccount.ApplyAndCall, applies valid delegations atomically with the outer call, and subsequent CALL→EOA executes delegate code.
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
  - Status: DONE — length checks occur first; precise messages present; tests assert length‑error messages.

- HIGH: Insufficient Actor Validation Tests
  - Add a dedicated `actors/evm/tests/apply_and_call_invalid.rs` suite covering:
    - Invalid `chainId` (not 0 or local).
    - Invalid `yParity` (>1).
    - Zero R or zero S.
    - High‑S rejection.
    - Nonce mismatch.
    - Pre‑existence policy violation (authority is an EVM contract).
    - Duplicate authority in a single message.
  - Status: DONE — covered by `apply_and_call_invalids.rs`, `apply_and_call_nonces.rs`, `apply_and_call_duplicates.rs`.

- MEDIUM: Missing Corner Case Tests (Actors)
  - SELFDESTRUCT no‑op in delegated context:
    - Build delegate bytecode that executes SELFDESTRUCT when executed under authority context (via VM intercept); assert no authority state/balance change; pointer mapping preserved; event emission intact.
  - Storage isolation/persistence on delegate changes:
    - A→B write storage; switch A→C and verify C cannot read B’s storage; clear A→0; re‑delegate A→B and verify B’s storage persists.
  - First‑time nonce handling:
    - Absent authority defaults to nonce=0; applying nonce=0 succeeds; next attempt with nonce=0 fails with nonce mismatch.
  - Status: DONE — `apply_and_call_selfdestruct.rs`, `delegated_storage_isolation.rs`, `delegated_storage_persistence.rs`, and `apply_and_call_nonces.rs`.

- LOW: Fuzzing and Vectors
  - lotus: add RLP fuzz harness for 0x04 decode focused on tuple arity/yParity/value sizes and malformed tails; ensure no panics and proper erroring.
  - builtin‑actors: add CBOR fuzz harness for `ApplyAndCallParams` that mutates wrapper shape, tuple arity, and byte sizes for `r/s`.
  - Status: DONE (builtin‑actors) — CBOR fuzz harness `apply_and_call_cbor_fuzz.rs` present.
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
    - File: ref‑fvm call manager intercept and EVM interpreter return mapping.
    - Action: when the VM intercept returns a revert, propagate revert bytes to the EVM return buffer and ensure `RETURNDATASIZE/RETURNDATACOPY` expose them.
    - Status: DONE — intercept path maps revert and copies data to memory; tests assert behavior.
  - Value transfer result handling in `ApplyAndCall`
    - File: `actors/evm/src/lib.rs`.
    - Action: check the result of `system.transfer` for delegated EOA targets; if it fails, set `status: 0` and return immediately (mirrors CALL behavior).
    - Status: DONE — short‑circuit implemented and covered by tests.

- HIGH — Tests to add (consensus‑critical and behavioral)
  - `actors/evm/tests/apply_and_call_invalids.rs`: invalid `chainId`, `yParity > 1`, zero R/S, high‑S, nonce mismatch, duplicate authorities, exceeding 64‑tuple cap. (DONE)
  - Pre‑existence policy: reject when authority resolves to an EVM contract actor (expect `USR_ILLEGAL_ARGUMENT`). (DONE)
  - R/S padding interop: accept 1..31‑byte R/S (left‑padded) and reject >32 bytes. (DONE)
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
- Routing: all 0x04 transactions route to EthAccount `ApplyAndCall`; no Delegator send/execute path remains.
- Atomic‑only: there are no non‑atomic paths or fallback code; tests assert atomic semantics for success and revert.
- Clean surface: no optional env toggles for routing or legacy decoder branches; documentation and tests reference a single, canonical flow.

**Migration / Compatibility**
- No migration required. The implementation is EVM‑only and atomic‑only.
- Delegator has been removed; EthAccount.ApplyAndCall is the sole entry point.
Domain: `0x05`. Pointer code magic/version: `0xef 0x01 0x00`.

---

**Builtin‑Actors Test Plan (EthAccount + EVM)**

Scope: `../builtin-actors` (paired repo), tracked here for sprint execution. Keep encodings and gas behavior in sync with Lotus.

Priority: P0 (blocking), P1 (recommended), P2 (nice‑to‑have)

P0 — Critical (spec + safety)
- Persistent delegated storage context (DONE)
  - VM intercept executes delegate under authority context against the authority’s persistent storage root, and persists the updated `evm_storage_root` back into EthAccount state on success.
- Atomicity semantics (spec‑compliant persistence)
  - Delegation mapping and nonce bumps persist even if the outer call reverts. Tests assert persistence on revert and embedded status/return in ApplyAndCall result.
- Intrinsic gas charging (per tuple) (DONE)
  - Per‑authorization intrinsic gas charged before validation; behavior covered in tests.

P0 — ApplyAndCall core (DONE)
- Happy path (atomic apply+call) validates mapping/nonce behavior.
- Invalids rejected with `USR_ILLEGAL_ARGUMENT`: empty list, invalid chainId, invalid yParity, zero r/s, high‑s, nonce mismatch, duplicates.
- Tuple decoding shape (DAG‑CBOR): canonical atomic params; round‑trip tested.

P0 — Delegated CALL semantics (DONE)
- Delegation handling: CALL→EOA executes delegate under authority context via VM intercept; depth limited to 1.
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
  - `apply_and_call_happy.rs` (P0) — covered by existing happy‑path tests
  - `apply_and_call_invalids.rs` (P0) — DONE
  - `apply_and_call_tuple_roundtrip.rs` (P0) — DONE
  - `apply_and_call_nonces.rs` (P1) — DONE
  - `eoa_call_pointer_semantics.rs` (P0) — DONE
  - `eip7702_delegated_log.rs` (P0) — covered in event assertions within existing tests
  - `delegated_storage_persistence.rs` (P0) — DONE
  - `apply_and_call_atomicity_revert.rs` (P0) — DONE
  - `apply_and_call_intrinsic_gas.rs` (P1) — DONE
  - `apply_and_call_duplicates.rs` (P1) — DONE
  - `delegated_call_revert_data.rs` (P0) — DONE

Notes
 
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
  - `EthTransactionFromSignedFilecoinMessage` reconstructs 0x04 (EthAccount.ApplyAndCall) and echoes `authorizationList`.
- Receipts attribution (DONE)
  - `adjustReceiptForDelegation` sets `delegatedTo` from `authorizationList` or synthetic `Delegated(address)` event.

P0 — Mempool (N/A on this branch)
- Cross‑account invalidation and per‑EOA cap not implemented; deviation documented.

P0 — Gas accounting (scaffold) (DONE)
- Counting + gating only; no absolute overhead assertions.
- `countAuthInApplyAndCallParams` handles canonical wrapper; tests cover monotonicity.

P1 — JSON‑RPC plumbing (DONE)
- `eth_getTransactionReceipt` returns `authorizationList` and `delegatedTo`; covered in unit tests.
- Block/tx receipt flows call `adjustReceiptForDelegation`.
- EthTransaction reconstruction robustness: strict decoder for ApplyAndCall params with negative tests for malformed CBOR.

P1 — Estimation integration (DONE)
- `eth_estimateGas` adds intrinsic overhead for N tuples (behavioral placeholder); tuple counting tested.

P1 — E2E tests (behind `eip7702_enabled`, run once wasm includes EthAccount.ApplyAndCall)
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
 
- Update gas constants/refunds in lockstep once finalized.
