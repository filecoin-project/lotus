Goal: clean up confusing or outdated references to `EvmApplyAndCall` / Delegator in code and tests now that the live routing is EthAccount.ApplyAndCall + VM intercept, while preserving historical documentation.

Scope
- Primary repo: `./lotus`.
- Paired repos for comment alignment (minimal edits):
  - `../builtin-actors`
  - `../ref-fvm`

Tasks (idempotent)
1. Lotus comment/test audit:
   - Search for the following terms in `./lotus`:
     - `EvmApplyAndCallActorAddr`
     - `EVM.ApplyAndCall`
     - `Delegator`
   - For each hit:
     - If it describes the current routing/behavior (e.g., gas scaffolds, receipt adjusters, tests) but still uses EVM/Delegator terminology, update the wording to reflect EthAccount.ApplyAndCall and the EthAccount + VM intercept design.
     - If it is clearly historical or deprecated (e.g., changelogs documenting the earlier Delegator design), leave it intact or add a short note clarifying that Delegator/EVM.ApplyAndCall are removed on this branch.
   - Do not change any functional behavior in this prompt—only comments, test descriptions, and naming that is misleading.

2. Optional alignment in builtin‑actors and ref‑fvm:
   - Search `../builtin-actors` and `../ref-fvm` for comments that still describe Delegator/EVM.ApplyAndCall as active components.
   - Where such comments describe current state, adjust them to match the EthAccount + VM intercept architecture.

3. Idempotency expectations:
   - On subsequent runs, if comments and test names already reflect the EthAccount + VM intercept design accurately, make no changes.

4. Final reporting:
   - In your final message for this prompt:
     - Summarize which files were updated for naming/comment cleanup (if any).
     - Confirm that no functional logic was changed in this prompt.
   - Explicitly say whether this naming/comment cleanup prompt finished its job properly or not.

