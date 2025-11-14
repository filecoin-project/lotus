Goal: ensure EthAccount explicitly accepts minimally‑encoded big‑endian `r/s` values with lengths 1..32 bytes (left‑padded internally), and rejects >32‑byte or zero values, aligning with Lotus’ RLP encoding.

Scope
- Repo: `../builtin-actors`.
- Files of interest:
  - `actors/ethaccount/src/lib.rs` (validation + recovery).
  - `actors/ethaccount/tests/*` (EthAccount test suites).

Tasks (idempotent)
1. Inspect current validation:
   - Confirm that `EthAccountActor::validate_tuple`:
     - Rejects `len(r) > 32` or `len(s) > 32`.
     - Rejects zero `r/s`.
     - Enforces `y_parity ∈ {0,1}`.
     - Enforces low‑S on a 32‑byte left‑padded `s`.
   - Confirm that `recover_authority` left‑pads `r/s` to 32 bytes before constructing the 65‑byte signature.

2. Add positive tests ONLY if missing:
   - Search the EthAccount tests for positive coverage of short‑length `r/s` (e.g., lengths 1, 31).
   - If such tests do not exist, add a small test file (or extend an existing one, e.g., `apply_and_call_invalids.rs`) that:
     - Constructs `ApplyAndCallParams` with single `DelegationParam` entries where:
       - `r` and `s` lengths vary over `{1, 31, 32}` (all non‑zero, low‑S).
       - `y_parity` is 0 or 1, `chain_id` is 0 or the local ChainID.
     - Calls `EthAccountActor::ApplyAndCall` via the mock runtime and asserts the call succeeds (no error) and that the actor accepts these tuples.
   - Keep tests fast and deterministic.

3. Idempotency expectations:
   - If the repository already has clear, explicit positive tests for sub‑32‑byte `r/s` values, do not add duplicate tests; instead, confirm they pass.

4. Final reporting:
   - In your final message for this prompt:
     - State whether new tests were added and where, or confirm that existing tests already cover positive `r/s` padding acceptance.
     - Confirm that `cargo test -p fil_actor_ethaccount` passes.
   - Explicitly say whether this R/S padding prompt finished its job properly or not.

