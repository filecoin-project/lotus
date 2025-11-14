Goal: perform a thorough, cross‑repo review of the EIP‑7702 EthAccount + ref‑fvm migration as it exists on this branch, treating the current state of the three repos as the PR under review.

Important: this prompt is **review‑only**. Do not modify code or tests unless explicitly requested by a higher‑level instruction; instead, focus on analysis, findings, and recommendations.

Scope
- Primary repo (CWD): `lotus` (this branch is the candidate PR branch).
- Paired repos (same parent directory):
  - `../builtin-actors`
  - `../ref-fvm`
- Ground‑truth design docs:
  - `AGENTS.md` (this repo).
  - `documentation/eip7702_ethaccount_ref-fvm_migration.md`.
  - `../eip-7702.md` (EIP‑7702 spec notes for this workstream).

High‑level objectives
1. Verify that the EthAccount + VM intercept migration is complete and coherent across the three repos, relative to the design docs.
2. Identify any correctness, safety, or spec‑alignment issues (including subtle edge cases).
3. Assess test coverage and note meaningful gaps.
4. Summarize findings clearly so a human reviewer can quickly decide whether to merge, and what follow‑ups to schedule.

Tasks (idempotent; analysis‑only)

1) Context and plan reconciliation
- Read the following carefully:
  - `AGENTS.md` (focus on the EIP‑7702 sections, “What Remains”, and the “Follow‑ups from 2025‑11‑13 review (status)” block).
  - `documentation/eip7702_ethaccount_ref-fvm_migration.md`.
  - `../eip-7702.md`.
- Confirm that the items marked DONE / PARTIAL / OPEN in `AGENTS.md` match what you see in the code/tests across the three repos.
  - If you find inconsistencies, **do not update code**; just record them as review findings.

2) Lotus review (Go)
- Focus files:
  - `chain/types/ethtypes/*7702*` (RLP 0x04 parsing, CBOR params, `AuthorizationKeccak`, transaction→message conversion).
  - `chain/types/ethtypes/eth_transactions.go` (special handling for ApplyAndCall).
  - `node/impl/eth/receipt_7702_scaffold.go` and `node/impl/eth/transaction_7702_receipts_test.go` (receipts, `delegatedTo`, synthetic `Delegated(address)` event handling).
  - `node/impl/eth/gas.go` and `node/impl/eth/gas_7702_scaffold*.go` (7702 gas overhead; behavioral only).
- Review items:
  - Encoding/decoding:
    - 0x04 RLP parser’s limits, canonical integer checks, yParity handling, and tuple arity.
    - CBOR `[ [tuple...], [to(20), value, input] ]` encoding/decoding and error paths.
  - Routing:
    - Type‑0x04 routing to `EthAccountApplyAndCallActorAddr` and `MethodHash("ApplyAndCall")`.
  - Receipts:
    - Status derivation from embedded ApplyAndCall return vs. exit code.
    - `AuthorizationList` and `delegatedTo` reconstruction from tuples and `Delegated(address)` event (32‑byte ABI word).
  - Gas:
    - Behavioral overhead only when targeting `ApplyAndCall` and when tuples are present.
    - No tests pin absolute gas numbers.
- Note any confusing error messages, brittle handling, or missing negative tests.

3) builtin‑actors review (Rust)
- Focus files:
  - `actors/ethaccount/src/state.rs` and `actors/ethaccount/src/lib.rs`.
  - `actors/evm/src/lib.rs`, `actors/evm/src/interpreter/instructions/ext.rs`, `actors/evm/src/interpreter/instructions/lifecycle.rs`.
  - Tests under `actors/ethaccount/tests` and `actors/evm/tests` that reference 7702.
- Review items:
  - EthAccount state/validation:
    - `delegate_to`, `auth_nonce`, `evm_storage_root` semantics and initialization.
    - Tuple validation (chainId domain, yParity, non‑zero R/S, ≤32‑byte R/S, low‑S).
    - Pre‑existence policy for authorities that resolve to EVM contracts.
    - Receiver‑only behavior and nonce handling (including absent‑nonce = 0).
  - ApplyAndCall behavior:
    - Order of operations (state persistence before outer call).
    - Outer call routing:
      - Invoke EVM contracts via `InvokeEVM` when target is an EVM actor.
      - Fallback to plain `METHOD_SEND` for non‑EVM targets.
      - Embedded `(status, output_data)` semantics and error mapping.
  - EVM actor + interpreter:
    - Removal/stubbing of legacy `InvokeAsEoa` / `EVM.ApplyAndCall`.
    - `InvokeAsEoaWithRoot` trampoline behavior (authority context, storage root mounting/persistence).
    - EXTCODE* pointer semantics via `get_eth_delegate_to` helper and 23‑byte `0xEF 0x01 0x00 || delegate(20)` image.
    - SELFDESTRUCT no‑op in authority context.
  - Test coverage:
    - Invalid/edge tests (tuples, nonces, duplicates, tuple cap).
    - Positive `r/s` padding tests.
    - Outer‑call routing tests (InvokeEVM vs METHOD_SEND).
- Assess whether EthAccount’s behavior matches the design doc, and note any surprising behavior or missing invariants.

4) ref‑fvm review (Rust)
- Focus files:
  - `fvm/src/call_manager/default.rs` (especially `try_intercept_evm_call_to_eoa`, keccak helpers, state update).
  - `fvm/src/syscalls/actor.rs` and `sdk/src/actor.rs` (get_eth_delegate_to syscall + helper).
  - `fvm/tests/*` covering:
    - `delegated_call_mapping.rs`
    - `delegated_value_transfer_short_circuit.rs`
    - `depth_limit.rs`
    - `evm_extcode_projection.rs`
    - `overlay_persist_success.rs`
    - `selfdestruct_noop_authority.rs`
    - `delegated_event_emission.rs`
    - `eth_delegate_to.rs`
    - `ethaccount_state_roundtrip.rs`
- Review items:
  - Intercept gating:
    - Only for `InvokeEVM` → EthAccount with `delegate_to` set.
    - Correct use of EthAccount state and resolution via EAM.
  - Value transfer semantics:
    - Transfer to authority before delegated execution; short‑circuit behavior and returned exit code/data on failure.
  - Delegated execution:
    - Correct construction and call of `InvokeAsEoaWithRoot`.
    - Mapping of success/revert to exit codes and return data.
    - Overlay persistence only on success.
  - Event emission:
    - `Delegated(address)` event topic and ABI encoding of the authority address.
  - EXTCODE* behavior:
    - 23‑byte pointer image and keccak hash consistency.
    - Windowing/zero‑fill semantics.
  - Test behavior for minimal builds (`--no-default-features`) and how tests tolerate feature flags.

5) Test and CI view
- Run (or at least conceptually verify) the focused test sets:
  - Lotus:
    - `go test ./chain/types/ethtypes -run 7702 -count=1`
    - `go test ./node/impl/eth -run 7702 -count=1`
    - Note status of `itests/TestEth7702_DelegatedExecute` (skip vs enabled).
  - builtin‑actors:
    - `cargo test -p fil_actor_ethaccount`
    - `cargo test -p fil_actor_evm`
  - ref‑fvm:
    - `cargo test -p fvm --tests -- --nocapture`
    - Optionally `scripts/run_eip7702_tests.sh` if the environment supports Docker and testing isn’t prohibitively slow.
- If you cannot actually run any of these in this environment, use the code and existing CI notes to infer their status, and call that out explicitly in your report.

6) Risk / edge‑case analysis
- Identify and discuss potential issues such as:
  - Cross‑repo constant mismatches (domain magic, bytecode magic/version).
  - Incomplete or surprising behavior around:
    - Non‑EVM outer calls.
    - Gas estimation and refunds.
    - Minimal builds where delegated paths may be disabled.
    - Error propagation (especially around value transfer and revert mapping).
  - Any consensus‑sensitive behavior that feels brittle or under‑tested.
- For each issue, categorize:
  - “Blocking” (should be fixed before merge).
  - “Follow‑up” (acceptable as a documented TODO).
  - “Non‑issue” (just worth noting).

7) Idempotency expectations
- This prompt should be safe to run multiple times:
  - You should not modify code, tests, or docs as part of this prompt.
  - If you do need to suggest changes, present them as recommendations with concrete file/line references, not as applied patches.
- On re‑runs, you may refine or extend the review conclusions but should not redo expensive work unnecessarily.

8) Final reporting (very important)
- Your final message for this prompt should be a structured review, not a patch summary. It must include:
  - **Overall assessment**: is the cross‑repo EIP‑7702 implementation ready to merge as‑is, or are there blocking issues?
  - **Per‑repo findings** (Lotus, builtin‑actors, ref‑fvm):
    - Strengths/what looks solid.
    - Issues or deviations from the design docs (with file references).
  - **Cross‑repo alignment**:
    - Constants/encodings.
    - Event semantics.
    - Routing and intercept behavior.
  - **Test/coverage summary**:
    - Which key tests you confirmed as present and (if possible) passing.
    - Any notable coverage gaps.
  - **Recommendation**:
    - Explicitly state whether you would:
      - Approve the PR as‑is,
      - Approve with noted follow‑ups, or
      - Block pending specific changes.
  - A clear sentence at the end indicating whether this “cross‑repo PR review” prompt has finished its job properly (e.g., “Cross‑repo EIP‑7702 review complete; no blocking issues found,” or “Cross‑repo review complete; blocking issues identified in X, see above.”).

