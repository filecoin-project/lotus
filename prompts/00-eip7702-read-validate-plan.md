Goal: read and validate the current EIP‑7702 migration plan and follow‑ups for this repo set (Lotus, builtin‑actors, ref‑fvm), without making speculative changes.

Scope
- CWD: Lotus repo root (this directory).
- Paired repos (same parent directory):
  - `../builtin-actors`
  - `../ref-fvm`
- Ground truth docs:
  - `AGENTS.md` (this repo).
  - `documentation/eip7702_ethaccount_ref-fvm_migration.md`.
  - `../eip-7702.md` (spec notes).

Tasks (idempotent)
1. Read and reconcile plan:
   - Open and skim:
     - `AGENTS.md`, focusing on the EIP‑7702 sections and “What Remains / Follow‑ups from 2025‑11‑13 review (OPEN)”.
     - `documentation/eip7702_ethaccount_ref-fvm_migration.md`.
     - `../eip-7702.md`.
   - Verify that the open items listed in `AGENTS.md` accurately reflect the true remaining work in code:
     - EthAccount → VM outer‑call bridge + Lotus E2E.
     - Positive R/S padding tests for EthAccount.
     - Delegated(address) event coverage in ref‑fvm.
     - Naming/comment cleanup for EvmApplyAndCall / Delegator.
   - If you discover that any of these items are already fully implemented and tested, update the corresponding bullet in `AGENTS.md` from “OPEN” to “DONE”, with a short note and file references.
   - Do NOT implement any of the missing features/tests in this prompt; only reconcile docs vs. reality and adjust `AGENTS.md` if it is stale.

2. Idempotency expectations:
   - On a re‑run, if `AGENTS.md` already correctly describes which items are DONE vs. OPEN, make no changes.

3. Final reporting (for this prompt only):
   - In your final message:
     - State whether each of the four follow‑ups is currently “already satisfied”, “clearly missing”, or “partially present” based on code/tests as of this run.
     - State whether you updated `AGENTS.md`, and if so, which bullets were changed.
   - Explicitly say whether this “plan validation” prompt finished its job properly (i.e., docs and code are now in sync for the follow‑up list).

