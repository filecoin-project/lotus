Goal: make depth‑limit enforcement for delegated CALLs in ref‑fvm explicit and verifiable, ensuring that delegation chains cannot recurse beyond depth=1 even under adversarial EthAccount configurations.

Scope
- CWD: `./lotus` (this repo; used for docs and context).
- Primary implementation repo: `../ref-fvm`.
- Paired repo for semantics: `../builtin-actors` (EVM System and authority‑context flag).

Tasks (idempotent)
1) Re‑establish current behavior
- In `../ref-fvm/fvm/src/call_manager/default.rs`, revisit `DefaultCallManager::call_actor_resolved` and `try_intercept_evm_call_to_eoa`:
  - Confirm that interception currently occurs when:
    - The target code CID belongs to EthAccount.
    - The entrypoint is `InvokeEVM` (FRC‑42 method hash for `"InvokeEVM"`).
    - The EthAccount state has `delegate_to = Some([u8;20])`.
  - Note that depth limiting is currently achieved implicitly: intercept is only aware of target code, not authority context.
- In `../builtin-actors/actors/evm/src/interpreter/system.rs`:
  - Confirm that `System.in_authority_context` exists and is set only by `InvokeAsEoaWithRoot`.
  - Verify that `selfdestruct` and any other authority‑sensitive opcodes check `system.in_authority_context`.

2) Decide on an explicit depth‑limit signal
- Design a simple, self‑contained mechanism to prevent re‑interception when already executing under authority context. Examples (pick one that fits the existing architecture):
  - A boolean “delegation active” flag attached to the call stack in `DefaultCallManager`, propagated into `try_intercept_evm_call_to_eoa`.
  - A convention that authority‑context calls set a special flag in the `InvocationResult` or via an additional parameter.
- The key property: when an EVM actor, executing under authority context as the delegate, issues CALL/STATICCALL to another EthAccount, the VM must not attempt to follow delegation again.

3) Implement the explicit guard
- In `../ref-fvm/fvm/src/call_manager/default.rs`:
  - Introduce the chosen depth‑limit marker (e.g., a `delegation_active: bool` on the call stack or within `CallManager`’s state).
  - Set the marker when a delegated CALL is initiated via `try_intercept_evm_call_to_eoa`.
  - Ensure the marker is cleared/restored appropriately when unwinding the stack.
  - Modify `try_intercept_evm_call_to_eoa` to early‑return `Ok(None)` when the marker indicates that a delegated CALL is already in progress (authority context).
- Keep the behavior otherwise identical:
  - Still require EthAccount code and `delegate_to` set.
  - Still require entrypoint `"InvokeEVM"`.
  - Keep event emission, value‑transfer semantics, overlay persistence, and revert mapping unchanged.

4) Strengthen tests for nested delegation attempts
- In `../ref-fvm/fvm/tests/depth_limit.rs`:
  - Review the existing test `delegated_call_depth_limit_enforced`:
    - It currently configures A→B and B→C and asserts that the observed behavior stops at B.
  - Extend or complement this test to:
    - Explicitly configure two EthAccount actors with delegates in a way that would cause re‑interception if depth limiting were not enforced.
    - Assert that, even with such a configuration, delegated execution stops after one level (i.e., delegate B’s code runs, but C’s delegate code never executes via the same intercept).
- If needed, add a small helper in `fvm/tests/common.rs` to construct multiple EthAccount actors with different `delegate_to` settings for easier matrix tests.

5) Run ref‑fvm tests
- In `../ref-fvm`:
  - `cargo test -p fvm --tests -- --nocapture`
- Ensure that:
  - All existing 7702 tests (`delegated_*`, `evm_extcode_projection`, `overlay_persist_success`, `selfdestruct_noop_authority`) remain green.
  - Any new depth‑limit tests pass consistently.

6) Final reporting
- In your final answer for this prompt:
  - Describe the chosen explicit depth‑limit mechanism and where it is implemented.
  - Summarize the new/updated tests that demonstrate the behavior when a second-level delegated EthAccount is present.
  - Confirm that `cargo test -p fvm --tests` passed and that no behavior regressed.
- End with a sentence like:
  - “Delegation depth limit is now explicitly enforced in ref‑fvm and covered by tests,”
  - or, if you determined that an explicit guard is not yet feasible, clearly explain why and what would be needed.

