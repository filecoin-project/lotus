This directory contains EIP‑7702 follow‑up prompts intended to be run via Codex CLI in sequence. Each prompt is:
- Focused on a specific aspect of the EthAccount + ref‑fvm migration (outer‑call bridge, tests, event coverage, naming/docs).
- Idempotent: it should be safe to run multiple times; if work is already complete, it should not re‑apply changes.
- Responsible for clear final reporting (per‑prompt).

Prompts (in order)
- `00-eip7702-read-validate-plan.md`  
  Read and reconcile the current migration plan and `AGENTS.md` follow‑ups with actual code/tests; do not implement changes here, only fix stale documentation.

- `01-eip7702-ethaccount-outer-call-bridge-and-e2e.md`  
  Implement or verify the EthAccount → VM outer‑call bridge and wire (or clarify) the Lotus 7702 E2E test.

- `02-eip7702-ethaccount-rs-padding-tests.md`  
  Add/verify positive tests for minimally‑encoded `r/s` padding acceptance in EthAccount.

- `03-eip7702-ref-fvm-delegated-event-coverage.md`  
  Add/verify ref‑fvm tests that assert `Delegated(address)` event topic and ABI‑encoded authority address.

- `04-eip7702-naming-and-comment-cleanup.md`  
  Clean up comments/tests that still use `EvmApplyAndCall` / Delegator terminology for current behavior.

- `05-eip7702-agents-and-tests-finalization.md`  
  Reconcile `AGENTS.md` status with reality and run the focused test matrix to confirm everything is green.

Standalone combined prompt
- `eip7702_followups.md`  
  The original combined prompt capturing all follow‑ups in one file. It is not used by `run_prompts.sh` but can be executed directly if you prefer a single, monolithic run.

