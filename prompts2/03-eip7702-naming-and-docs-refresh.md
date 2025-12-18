Goal: clean up remaining naming and documentation drift around EIP‑7702 (e.g., `EvmApplyAndCallActorAddr` references and outdated comments) so that the code and docs accurately reflect the EthAccount + VM intercept routing.

Scope
- CWD: `./lotus` (primary target for naming/docs).
- Paired repos (read‑only unless a clear gap is found):
  - `../builtin-actors` (actor comments).
  - `../ref-fvm` (design comments).

Tasks (idempotent)
1) Locate residual naming and comment drift
- In `./lotus`:
  - Search for `EvmApplyAndCallActorAddr`:
    - Identify where it is still used to represent the EthAccount actor (e.g., tests and scaffolding).
    - Distinguish between:
      - Places where the name is purely local to tests and harmless, and
      - Places where it can cause confusion in comments or public documentation.
  - Search for mentions of “Delegator” or `EVM.ApplyAndCall` in comments and docs:
    - In `AGENTS.md`, `documentation/eip7702_ethaccount_ref-fvm_migration.md`, and any design notes.
    - Ensure that they either:
      - Clearly mark old paths as historical, or
      - Are updated to reference `EthAccount.ApplyAndCall` + VM intercept instead.
- In `../builtin-actors` and `../ref-fvm`:
  - Skim for comments that still describe the older EVM‑centric design (e.g., `InvokeAsEoa` as the primary path) instead of the current EthAccount + intercept architecture.

2) Update names in Lotus where it improves clarity
- For identifiers that currently use `EvmApplyAndCallActorAddr` to refer to the EthAccount actor:
  - In tests and scaffolding (e.g., `node/impl/eth/transaction_7702_receipts_test.go`, `node/impl/eth/gas_7702_estimate_integration_test.go`, `itests/eth_7702_e2e_test.go`):
    - Prefer renaming local variables to something like `EthAccountApplyAndCallActorAddr` or a clearly neutral name (e.g., `applyAndCallActorAddr`) where it makes the intent obvious.
    - Ensure that any renaming is local and does not break existing behavior; keep method calls and constants intact.
- Avoid large mechanical renames; focus on the most confusing or externally visible instances.

3) Refresh comments and docs to match the current design
- In `AGENTS.md`:
  - Ensure that the sections describing routing and semantics:
    - Emphasize `EthAccount.ApplyAndCall` as the top-level 0x04 target.
    - Note that `EVM.ApplyAndCall` and `InvokeAsEoa` are removed/stubbed and that delegation is handled via the VM intercept and `InvokeAsEoaWithRoot`.
  - Verify that the “status” sections for:
    - EthAccount outer‑call bridge,
    - R/S padding tests,
    - Delegated(address) event coverage,
    - reflect the current state (DONE vs PARTIAL/OPEN).
- In `documentation/eip7702_ethaccount_ref-fvm_migration.md`:
  - Check that the descriptions of:
    - EthAccount state,
    - VM intercept semantics,
    - ApplyAndCall outer call,
    - pointer code and events,
    - match the code as of this branch.
  - If you find obvious mismatches (e.g., references to a Delegator actor that no longer exists), update them to:
    - Describe EthAccount as the owner of delegation state.
    - Mention ref‑fvm intercept as the implementation of delegated CALL/EXTCODE*.

4) Keep changes minimal and focused
- Do not change function signatures or behavior in this prompt.
- Limit edits to:
  - Local variable / constant names where they reduce confusion.
  - Comments and docs that are clearly out of date.
- Ensure everything remains idempotent: rerunning this prompt should not produce new diffs once comments/names are aligned.

5) Final reporting
- In your final answer for this prompt:
  - List the main places where names or comments were updated (files and brief descriptions).
  - Confirm that you did not change any behavior, only naming/comments/docs.
  - Call out any remaining intentional references to legacy names (e.g., compatibility stubs) and why they were left alone.
- Finish with a line like:
  - “Naming and documentation for EIP‑7702 are now consistent with the EthAccount + VM intercept design; only legacy stubs remain for historical reasons.”

