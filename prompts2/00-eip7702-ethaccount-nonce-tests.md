Goal: finalize and enable EthAccount nonce handling tests for EIP‑7702 so that nonce initialization and increment semantics are fully covered and no longer rely on ignored tests.

Scope
- CWD: `./lotus` (this repo).
- Paired repos:
  - `../builtin-actors` (EthAccount actor implementation and tests).
  - `../ref-fvm` (VM intercept and EthAccount state round‑trip).

Tasks (idempotent)
1) Inspect current nonce behavior and tests
- In `../builtin-actors/actors/ethaccount/src/lib.rs`, review `EthAccountActor::apply_and_call`:
  - Confirm that `auth_nonce` starts at 0, is compared for equality against each tuple’s `nonce`, and is incremented once per accepted tuple/message.
  - Verify that nonce persistence happens inside the `rt.transaction::<State, _, _>` block, before the outer call is executed.
- In `../builtin-actors/actors/ethaccount/tests/apply_and_call_nonces.rs`:
  - Check the `#[ignore]`-annotated test `nonce_init_and_increment` and understand why it was skipped (e.g., flakiness, harness limitations, or behavior changes).
  - Confirm whether the test still matches the current actor semantics (absent‑nonce = 0, first nonce=0 succeeds, second nonce=0 fails).

2) Either re‑enable or replace the nonce test
- If `nonce_init_and_increment` is still correct and stable under the current semantics:
  - Remove the `#[ignore]` annotation and adjust expectations only as needed to match the actual behavior (e.g., error messages or exact send expectations) while preserving:
    - success for the first `nonce=0` tuple on a fresh EthAccount, and
    - failure for a second `nonce=0` tuple.
- If the existing test is stale or structurally mismatched to the current ApplyAndCall implementation:
  - Replace it with one or more focused tests that:
    - Explicitly assert:
      - Initial nonce=0 for a fresh EthAccount (no prior delegations).
      - Nonce equality enforcement (old nonce accepted; repeated nonce rejected).
    - Use the existing MockRuntime helpers and expectations (e.g., `expect_send_any_params`) in an idempotent way.
  - Keep tests self‑contained and deterministic so they can run reliably in CI.

3) Cross‑check nonce semantics in ref‑fvm
- In `../ref-fvm/fvm/tests/ethaccount_state_roundtrip.rs` and `../ref-fvm/fvm/tests/common.rs`:
  - Confirm that `auth_nonce` is encoded/decoded in the same position and type as the EthAccount state used by the kernel (`delegate_to`, `auth_nonce`, `evm_storage_root`).
  - If necessary, add a small test to verify that EthAccount state written by builtin‑actors (via the bundle) round‑trips through the VM without losing nonce information.

4) Run focused tests
- In `../builtin-actors`:
  - `cargo test -p fil_actor_ethaccount --tests -- --nocapture`
- In `../ref-fvm` (sanity only; do not change behavior):
  - `cargo test -p fvm --tests -- --nocapture`

5) Final reporting
- In your final answer for this prompt:
  - State whether the EthAccount nonce behavior was already correct and fully covered, or if you had to enable/adjust/add tests.
  - Point to the specific test file(s) and test name(s) that now cover:
    - initial nonce behavior, and
    - nonce increment/rejection semantics.
  - Confirm which of the above test commands you ran and whether they passed.
- Explicitly close with a sentence such as:
  - “EthAccount nonce tests are now enabled and passing; nonce semantics are fully covered,”
  - or, if something must remain skipped, explain why and what remains open.

