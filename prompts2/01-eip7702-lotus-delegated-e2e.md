Goal: enable and stabilize a full Lotus E2E test for delegated execution under EIP‑7702 (type‑0x04 → EthAccount.ApplyAndCall → delegated CALL→EOA) once the wasm bundle supports the EthAccount + ref‑fvm changes.

Scope
- CWD: `./lotus` (this repo).
- Paired repos (read‑only in this prompt):
  - `../builtin-actors` (EthAccount and EVM actors).
  - `../ref-fvm` (VM intercept, `get_eth_delegate_to`, and bundle).

Tasks (idempotent)
1) Confirm prerequisites
- Read `AGENTS.md` and `documentation/eip7702_ethaccount_ref-fvm_migration.md` to refresh the intended full lifecycle:
  - Type‑0x04 decode and CBOR params.
  - EthAccount state + ApplyAndCall semantics.
  - ref‑fvm delegated CALL intercept and EXTCODE* pointer semantics.
- Check that the current branch has a wasm bundle with EthAccount.ApplyAndCall and the ref‑fvm intercept wired:
  - Look at `../ref-fvm/fvm/tests` (delegated* and extcode* tests) and `../builtin-actors/actors/ethaccount/tests`:
    - If these are green and the bundle for this branch is known to include the same code, you can proceed to E2E wiring.
    - Otherwise, treat this prompt as design scaffolding only and do not attempt to force E2E in this run.

2) Inspect and update the E2E test scaffold
- Open `itests/eth_7702_e2e_test.go` and inspect:
  - `TestEth7702_SendRoutesToEthAccount`
  - `TestEth7702_ReceiptFields`
  - `TestEth7702_DelegatedExecute`
- For `TestEth7702_DelegatedExecute`:
  - If it is still `t.Skip(...)`:
    - Confirm that the skip reason matches the current state (e.g., missing wasm bundle or tuple‑signing helpers) and is not outdated.
    - If the bundle now supports full ApplyAndCall + delegated CALL, plan to:
      - Remove the skip.
      - Implement the test flow described in the design doc: send a type‑0x04 tx to apply delegation, then CALL the EOA and assert delegated execution and storage persistence.
  - If the test is already enabled:
    - Verify that it:
      - Uses type‑0x04 → EthAccount.ApplyAndCall to set `delegate_to`/`auth_nonce`.
      - Exercises a CALL→EOA that triggers the VM intercept (delegated execution).
      - Asserts:
        - storage changes under the authority (via a simple delegate contract),
        - presence of `authorizationList` and `delegatedTo` in the receipt,
        - receipt `Status` reflecting the embedded ApplyAndCall status.

3) Implement or refine the E2E flow (ONLY if prerequisites are met)
- Ensure the test:
  - Uses the same constants/magic as the unit tests and ref‑fvm:
    - `SetCodeAuthorizationMagic = 0x05`.
    - Delegation indicator bytecode `0xEF 0x01 0x00 || delegate(20)`.
  - Relies on the existing 0x04 parsing and CBOR helpers (`eth_7702_transactions.go`, `eth_7702_params.go`) instead of hand‑rolling encodings.
  - Avoids pinning numeric gas values; assert only:
    - inclusion of tuple overhead when appropriate, and
    - functional success (storage updates, events, and receipt status).
- Keep the test idempotent:
  - Use fresh accounts and contracts per run.
  - Avoid relying on global mutable state beyond the test harness’s standard lifecycle.

4) Run the E2E and focused suites
- In `./lotus`:
  - `go test ./chain/types/ethtypes -run 7702 -count=1`
  - `go test ./node/impl/eth -run 7702 -count=1`
  - `go test ./itests -run Eth7702_DelegatedExecute -tags eip7702_enabled -count=1`
- If `TestEth7702_DelegatedExecute` must remain skipped (e.g., bundle still missing), you may run only the first two commands to confirm nothing regressed, and explicitly document why E2E is still disabled.

5) Final reporting
- In your final answer for this prompt:
  - State whether `TestEth7702_DelegatedExecute` is:
    - enabled and passing,
    - enabled but failing (with a short diagnosis), or
    - still skipped due to clearly documented prerequisites.
  - Summarize the key behaviors that the E2E test now covers (storage, events, receipts, gas behavior).
  - List which test commands you ran and their outcomes.
- End with a clear statement such as:
  - “Lotus E2E delegated execution test is now enabled and passing,” or
  - “E2E remains skipped due to missing bundle; unit/integration tests remain green.”

