Goal: add or verify explicit ref‑fvm coverage for the `Delegated(address)` event emitted by the delegated CALL intercept, asserting topic and ABI encoding of the authority address.

Scope
- Repo: `../ref-fvm`.
- Files of interest:
  - `fvm/src/call_manager/default.rs` (intercept, `keccak32`, event emission).
  - `fvm/tests/*` (especially delegated mapping/value, EXTCODE* tests).

Tasks (idempotent)
1. Inspect current event emission:
   - In `fvm/src/call_manager/default.rs`, verify that:
     - `try_intercept_evm_call_to_eoa` emits an event with:
       - Topic0 = `keccak32(b"Delegated(address)")`.
       - A 32‑byte word whose last 20 bytes contain the authority’s 20‑byte address.

2. Add an integration test ONLY if missing:
   - Search `fvm/tests` for any test that explicitly checks:
     - The topic hash for `Delegated(address)`, and
     - That the event data’s last 20 bytes equal the authority’s address.
   - If such a test does not exist, add a new test file, e.g., `delegated_event_emission.rs`, that:
     - Uses `fvm/tests/common.rs::new_harness` to build a machine from the builtin‑actors bundle.
     - Sets up:
       - An EthAccount authority A with `delegate_to` set to a delegate EVM contract B (as in `set_ethaccount_with_delegate`).
       - A caller EVM contract C that CALLs A, triggering the delegated CALL intercept.
     - Executes C, retrieves the receipt (including `EventsRoot`), decodes events, and asserts:
       - There is at least one event whose topic0 equals `keccak32("Delegated(address)")`.
       - The corresponding data is 32 bytes, and its last 20 bytes equal A’s Ethereum address.
   - Ensure the test tolerates minimal builds (`--no-default-features`) in a similar way to other delegated tests (early exit if features are unavailable).

3. Idempotency expectations:
   - If such a test already exists and passes, do not add another; simply confirm it still passes.

4. Final reporting:
   - In your final message for this prompt:
     - State whether a new event‑coverage test was added (and where), or whether existing tests already cover the topic + ABI encoding.
     - Confirm that `cargo test -p fvm --tests -- --nocapture` passes.
   - Explicitly say whether this delegated‑event coverage prompt finished its job properly or not.

