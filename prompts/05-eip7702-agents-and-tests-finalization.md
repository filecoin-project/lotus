Goal: reconcile `AGENTS.md` with the actual code/tests for EIP‑7702 follow‑ups and run the focused test matrix to confirm everything is in a good state.

Scope
- Primary repo: `./lotus` (AGENTS + Lotus tests).
- Paired repos:
  - `../builtin-actors`
  - `../ref-fvm`

Tasks (idempotent)
1. Reconcile AGENTS follow‑ups:
   - Re‑read `AGENTS.md`, especially:
     - The EIP‑7702 sections.
     - The “What Remains” and “Follow‑ups from 2025‑11‑13 review (OPEN)” bullets.
   - For each follow‑up item:
     - EthAccount → VM outer‑call bridge + E2E.
     - Positive R/S padding tests.
     - Delegated(address) event coverage in ref‑fvm.
     - Naming/comment cleanup.
   - Determine, based on the current code and tests, whether it is:
     - DONE (fully implemented and tested).
     - PARTIAL (some pieces implemented/tests in progress).
     - OPEN (no meaningful implementation yet).
   - Update `AGENTS.md` bullets accordingly:
     - Mark items DONE only if code and tests truly match the intended behavior.
     - Leave items as OPEN or clarify as PARTIAL where appropriate.

2. Run targeted test matrix:
   - Lotus:
     - `go test ./chain/types/ethtypes -run 7702 -count=1`
     - `go test ./node/impl/eth -run 7702 -count=1`
     - If the E2E test `TestEth7702_DelegatedExecute` is enabled: `go test ./itests -run Eth7702 -tags eip7702_enabled -count=1`
   - builtin‑actors:
     - `cargo test -p fil_actor_evm`
     - `cargo test -p fil_actor_ethaccount`
   - ref‑fvm:
     - `cargo test -p fvm --tests -- --nocapture`
     - Optionally, `scripts/run_eip7702_tests.sh` if Docker and the bundle are available and not prohibitively slow.
   - If any of these commands are not runnable in the current environment (e.g., missing Docker, platform issues), clearly note which ones were skipped and why.

3. Idempotency expectations:
   - On re‑runs, if `AGENTS.md` already accurately reflects which tasks are DONE vs. OPEN/PARTIAL, limit changes to status updates where something has newly landed since the last run.

4. Final reporting:
   - In your final message for this prompt:
     - For each of the four main follow‑up tasks, state whether it is DONE, PARTIAL, or OPEN, and reference the key files/tests backing that assessment.
     - List which tests you ran and whether they passed; for any skipped tests, provide a short reason.
     - Summarize any `AGENTS.md` changes you made.
   - Explicitly say whether this “AGENTS and tests finalization” prompt finished its job properly or not.

