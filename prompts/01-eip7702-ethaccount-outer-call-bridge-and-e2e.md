Goal: implement (or verify) the EthAccount → VM outer‑call bridge for EIP‑7702 and enable the Lotus E2E flow when the bundle supports it, matching the migration plan.

Scope
- CWD: Lotus repo root.
- Paired repos:
  - `../builtin-actors` (EthAccount + EVM actors).
  - `../ref-fvm` (VM intercept and syscalls).

Tasks (idempotent)
1. Detect current behavior:
   - In `../builtin-actors/actors/ethaccount/src/lib.rs`, inspect `EthAccountActor::apply_and_call`:
     - If it already calls a dedicated VM/EVM helper (e.g., a syscall or runtime hook) for the “outer call” instead of a plain `rt.send`, and that helper uses the delegated CALL semantics (authority context, depth=1, storage overlay, event), then the bridge may already be in place.
   - In `../ref-fvm/fvm/src/call_manager/default.rs`, confirm that:
     - `try_intercept_evm_call_to_eoa` intercepts CALL/STATICCALL→EthAccount with `delegate_to` set.
     - It uses `InvokeAsEoaWithRoot` and EthAccount state (`delegate_to`, `evm_storage_root`) to execute delegate code under authority context, enforces depth=1, and emits `Delegated(address)`.

2. Implement or wire the outer‑call bridge (ONLY if missing):
   - In ref‑fvm:
     - Introduce a dedicated VM entrypoint or helper (e.g., `evm_apply_and_call`‑style syscall or internal helper) that:
       - Accepts `(authority_eth: [u8;20], to_eth: [u8;20], value: TokenAmount, input: Bytes)`.
       - Reuses the existing delegation machinery (EthAccount state + `InvokeAsEoaWithRoot`) to execute delegate code under authority context when appropriate.
       - Returns `(status: u8, returndata: Bytes)` where `status=1` for `ExitCode::OK` and `status=0` for revert/value‑transfer failures, preserving revert payloads.
   - In builtin‑actors:
     - Update `EthAccountActor::apply_and_call` so that, after persisting delegation mapping + nonce increments:
       - It calls the new VM helper instead of a raw `rt.send` for the outer call.
       - It keeps the actor’s exit code `OK`, returning `ApplyAndCallReturn { status, output_data }` derived from the helper’s result (embedded status contract).
   - Maintain existing semantics:
     - Mapping and nonce updates MUST persist regardless of outer call outcome.
     - Storage overlay (`evm_storage_root`) persists only on successful delegated execution.

3. Lotus E2E (if wasm bundle supports it):
   - In `./itests/eth_7702_e2e_test.go`, locate `TestEth7702_DelegatedExecute`:
     - If the bundle includes the necessary EthAccount/EVM changes, unskip the test and ensure it:
       - Applies delegations via a 0x04 transaction (0x04 → EthAccount.ApplyAndCall).
       - CALLs the delegated EOA and asserts:
         - Delegated code executes.
         - Storage under the authority is updated and persists.
         - Receipt’s `authorizationList`, `delegatedTo`, and `status` reflect the embedded status.
     - If the bundle is still missing, leave the test skipped but ensure the skip message clearly indicates that only bundling is missing, not code.

4. Idempotency expectations:
   - If, on inspection, the EthAccount outer‑call bridge and E2E test are already implemented and passing, make no code changes; only adjust comments/docs if they are stale.
   - Ensure any new helper/syscall is additive and backward‑compatible within this branch.

5. Final reporting:
   - In your final answer for this prompt:
     - State whether the EthAccount outer‑call bridge was already present or was implemented in this run (with key file references).
     - State whether `TestEth7702_DelegatedExecute` is enabled and passing, or still skipped due to missing bundle wiring.
     - Confirm which tests you ran (at least the targeted EthAccount/EVM/ref‑fvm tests and `go test` for the focused 7702 suites), and whether they all passed.
   - Explicitly say whether this outer‑call bridge + E2E prompt finished its job properly or not.

