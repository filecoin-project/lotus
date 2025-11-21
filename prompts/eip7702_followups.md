Goal: resolve the open EIP‑7702 follow‑ups recorded in `AGENTS.md` (EthAccount → VM outer‑call bridge, positive R/S padding tests, Delegated(address) event coverage, and naming cleanup) across `./lotus`, `../builtin-actors`, and `../ref-fvm`, in a way that is safe, idempotent, and clearly reported.

Context and references
- Work in the following repos/branches:
  - `lotus` (CWD): branch `eip7702`.
  - `../builtin-actors`: branch `eip7702`.
  - `../ref-fvm`: branch `eip7702`.
- Ground truth docs:
  - `AGENTS.md` in `./lotus`.
  - `documentation/eip7702_ethaccount_ref-fvm_migration.md` in `./lotus`.
  - `../eip-7702.md` (spec notes).

High‑level tasks (all MUST be idempotent)
1. EthAccount → VM outer‑call bridge + Lotus E2E enablement.
2. Positive R/S padding tests for EthAccount.
3. Delegated(address) event coverage in ref‑fvm.
4. Naming and comment cleanup around `EvmApplyAndCall` / Delegator.
5. Update `AGENTS.md` to reflect the new status once work is done.

Idempotency requirements
- Before changing code for any task, first detect whether the work is already complete:
  - If code, tests, and docs already match the intended end state for that task, do NOT change them; simply record in your final message that this task was already satisfied.
  - Only apply changes when a clear gap remains.
- All added tests should be stable when re‑run and should not duplicate existing coverage; exercise unique behaviors or assert previously untested invariants.
- When updating `AGENTS.md`, only flip task status from OPEN to DONE (or adjust wording) if you actually completed the work in this run; never pre‑declare future work as DONE.

Task 1 — EthAccount → VM outer‑call bridge + Lotus E2E (0x04)

Intent
- Align the implementation with the migration plan: type‑0x04 transactions must atomically:
  1. Apply delegation mappings + nonce bumps in EthAccount (persisting even if the outer call reverts), and
  2. Execute the outer call under the VM’s delegated CALL semantics, using the authority’s `evm_storage_root`, depth‑limit, pointer semantics, and `Delegated(address)` event emission.

What to implement
1. Check current behavior:
   - In `../builtin-actors/actors/ethaccount/src/lib.rs`, inspect `EthAccountActor::apply_and_call`.
     - If it already invokes a dedicated VM/EVM helper (e.g., a syscall or runtime hook) instead of a bare `rt.send` for the outer call, and that helper is wired to delegated CALL semantics as per `documentation/eip7702_ethaccount_ref-fvm_migration.md`, then this task may already be complete; verify via tests in step 3 and skip changes if everything passes.
   - In `../ref-fvm/fvm/src/call_manager/default.rs`, confirm that delegated CALL interception (`try_intercept_evm_call_to_eoa`) is used for CALL/STATICCALL→EthAccount and that storage/event semantics match the migration doc.
2. If EthAccount still uses a plain `rt.send` for the outer call, introduce a dedicated bridge:
   - In ref‑fvm:
     - Add a VM entrypoint or helper (e.g., an `evm_apply_and_call`‑style syscall or an internal function) that:
       - Accepts `authority: EthAddress`, `to: EthAddress`, `value: TokenAmount`, and `input: Bytes`.
       - Forwards gas similar to the current delegated CALL path (no 63/64 clamp at the outer boundary).
       - Leverages the existing EthAccount state (`delegate_to`, `evm_storage_root`) and EVM actor `InvokeAsEoaWithRoot` trampoline to execute the delegate code under authority context when appropriate.
       - Returns `(status: u8, returndata: Bytes)` such that:
         - `status=1` for `ExitCode::OK`, `status=0` for revert or value‑transfer failures.
         - `returndata` carries the callee’s return or revert payload.
       - Preserves the existing intercept guarantees: depth limit, SELFDESTRUCT no‑op in authority context, overlay persistence on success only, and `Delegated(address)` event emission.
   - In builtin‑actors:
     - Update `EthAccountActor::apply_and_call` to:
       - Continue to validate and persist delegation mapping + nonce increments exactly as today (in a transaction, before the outer call).
       - Replace the direct `rt.send` with a call to the new VM/EVM helper so that the outer call’s execution and delegated semantics match the intercept path.
       - Keep the actor exit code `OK` and return an `ApplyAndCallReturn` where `status` (0/1) and `output_data` are derived from the helper’s result, maintaining the “embedded status” contract relied on by Lotus receipts.
3. Lotus E2E:
   - In `./lotus/itests/eth_7702_e2e_test.go`, locate `TestEth7702_DelegatedExecute`:
     - If it is still `t.Skip(...)` and the new outer‑call bridge is in place and bundled into the wasm, unskip the test and update the logic as needed to:
       - Apply delegations via a type‑0x04 transaction (0x04 → EthAccount.ApplyAndCall).
       - CALL → EOA and assert execution of the delegate, persistent storage under the authority, and correct `authorizationList` / `delegatedTo` / `status` in the receipt.
   - If the environment still lacks a wasm bundle with the new entrypoint, keep the skip but update the skip message and `AGENTS.md` to reflect what is missing (bundle wiring vs. code).

Idempotency for Task 1
- If the outer call already uses a dedicated VM/EVM helper with delegated semantics AND `TestEth7702_DelegatedExecute` is implemented and passing, do not re‑implement the bridge; just confirm via tests and mark the corresponding follow‑up as DONE in `AGENTS.md`.
- If only part of the work is done (e.g., helper exists but E2E is still skipped), complete the missing parts only.

Task 2 — Positive R/S padding tests for EthAccount

Intent
- Confirm that EthAccount accepts minimally‑encoded big‑endian `r/s` values with lengths from 1 to 32 bytes, left‑padding to 32 internally, while rejecting >32‑byte values and zero values (the latter already covered).

What to do
1. In `../builtin-actors/actors/ethaccount/src/lib.rs`, keep the current validation logic:
   - `len(r), len(s) ≤ 32`, rejection of >32 bytes, non‑zero check, `y_parity ∈ {0,1}`, and low‑S check on padded `s`.
2. In `../builtin-actors/actors/ethaccount/tests`:
   - If there is NOT already a test that explicitly asserts acceptance of shorter `r/s` lengths (e.g., 1‑byte, 31‑byte) for valid tuples, add one, e.g. in a new file `apply_and_call_rs_padding.rs` or by extending `apply_and_call_invalids.rs`:
     - Construct `DelegationParam` values with:
       - `r` lengths `{1, 31, 32}`, `s` lengths `{1, 31, 32}` (non‑zero, low‑S).
       - `y_parity` = 0 or 1, chain_id 0 or local.
     - Call `EthAccountActor::ApplyAndCall` via the mock runtime and assert success (proper exit, status=1) for these cases.
   - Ensure tests remain fast and deterministic.

Idempotency for Task 2
- Before adding new tests, search for existing EthAccount tests that already cover positive short‑length `r/s` cases; if they exist and clearly assert acceptance, do not add duplicates.

Task 3 — Delegated(address) event coverage in ref‑fvm

Intent
- Verify that the VM intercept emits a `Delegated(address)` event with:
  - Topic0 = `keccak256("Delegated(address)")`.
  - Data = a 32‑byte ABI word whose last 20 bytes equal the authority (EOA) address.

What to do
1. In `../ref-fvm/fvm/tests`, add a new integration test (e.g., `delegated_event_emission.rs`) if one does not already exist that explicitly inspects the event:
   - Use `fvm/tests/common.rs::new_harness` to instantiate a machine with the bundled EthAccount and EVM actors.
   - Set up:
     - An EthAccount authority A with `delegate_to` set to a delegate EVM contract B, as done in `set_ethaccount_with_delegate`.
     - A caller EVM contract C that CALLs A so the VM intercept path is exercised.
   - Execute C and obtain:
     - The receipt (including `EventsRoot`).
     - The decoded events, using the same machinery as other event tests.
   - Assert:
     - There is at least one event whose topic0 equals `keccak32(b"Delegated(address)")` (you can reuse `keccak32` from `call_manager/default.rs` or compute Keccak directly).
     - The associated data is 32 bytes long and its last 20 bytes equal A’s 20‑byte Ethereum address (authority).
2. Ensure the test tolerates minimal builds (`--no-default-features`) the same way existing delegation tests do (early‑exit or conditional assertions if needed).

Idempotency for Task 3
- If such a test already exists and asserts both topic and ABI encoding of the authority address, do not add another; just confirm it passes.

Task 4 — Naming / comment cleanup (EvmApplyAndCall / Delegator)

Intent
- Remove residual confusion from comments/tests referring to `EvmApplyAndCall` or “Delegator” where the live path is now EthAccount + VM intercept, while preserving historical documentation.

What to do
1. In `./lotus`:
   - Search for `EvmApplyAndCallActorAddr` and “Delegator” across code and tests:
     - For references in code/tests that describe *current behavior* but still name `EvmApplyAndCall` or “Delegator”, update the wording to refer to EthAccount.ApplyAndCall and the EthAccount + VM intercept design.
     - For historical changelogs or explicit “deprecated/removed” docs, keep the text but, if helpful, add a short clarifying note that the Delegator/EVM.ApplyAndCall paths are deprecated on this branch and have been replaced by EthAccount.ApplyAndCall.
2. Mirror any critical terminology updates in `../builtin-actors` and `../ref-fvm` where comments still mention Delegator as if it were live, but avoid over‑editing historical FIP documents.

Idempotency for Task 4
- Only change comments/tests where the terminology mismatch is actually confusing the current design; do not repeatedly rewrite already‑updated comments.

Task 5 — Update AGENTS.md and final verification

After implementing the above tasks (or confirming they are already satisfied):
1. Update `./lotus/AGENTS.md`:
   - In the “What Remains” / follow‑ups section, mark any completed items from the 2025‑11‑13 follow‑up list as DONE, with a short note (e.g., “DONE — wired EthAccount.ApplyAndCall to VM helper and unskipped TestEth7702_DelegatedExecute”).
   - If new tests were added, briefly note their locations under the appropriate test‑plan sections (Lotus, builtin‑actors, ref‑fvm).
2. Run the relevant tests:
   - Lotus:
     - `go test ./chain/types/ethtypes -run 7702 -count=1`
     - `go test ./node/impl/eth -run 7702 -count=1`
     - If the E2E is enabled: `go test ./itests -run Eth7702 -tags eip7702_enabled -count=1`
   - builtin‑actors:
     - `cargo test -p fil_actor_evm`
     - `cargo test -p fil_actor_ethaccount`
   - ref‑fvm:
     - `cargo test -p fvm --tests -- --nocapture`
     - Optionally `scripts/run_eip7702_tests.sh` if Docker and the bundle path are available.

Final reporting (VERY IMPORTANT)
- In your final message to the user, you MUST:
  - List each of the four main tasks (outer‑call bridge + E2E, R/S padding tests, Delegated(address) event coverage, naming cleanup).
  - For each task, state clearly whether:
    - It was already complete when you started.
    - You completed it in this run (and point to the key files/tests).
    - Or it remains incomplete, with a short explanation why.
  - Confirm whether all of the above tests were run and passed; if you had to skip any (e.g., missing wasm bundle or Docker), call that out explicitly.
- This explicit summary is required so that this prompt can be re‑run safely in the future and reviewers can quickly see whether it “finished its job properly” or if further work remains.

If, after following these steps, all tasks are either verified complete or implemented and all covered tests pass, state in your final answer:  
> “EIP‑7702 follow‑ups are complete for this run; AGENTS.md is updated and all targeted tests are passing.”  
Otherwise, clearly enumerate which items remain open and why.

