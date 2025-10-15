# EIP-7702 Implementation Notebook (Lotus)

This notebook is for agents continuing the EIP-7702 work in Lotus. It captures what’s already implemented (Phase‑1 front‑half), and what remains to wire the actor/FVM and policy (Phase‑2 back‑half). It also lists quick validation steps and suggested tests.

**Purpose**
- Provide a concise, actionable plan to complete EIP‑7702.
- Document current status, remaining work, and validation steps.

**Summary (Lotus vs FVM)**
- Lotus/Go (completed): type-0x04 parsing/encoding; `authorizationList` in RPC and receipts; CBOR params; send-path targets Delegator method (dev/test flag); mempool policies (cross-account invalidation and per‑EOA cap) gated by network version; intrinsic gas overhead scaffold; delegator tuple decode/validation + state apply helpers; tests and observability.
- Lotus/Go (remaining): align gas estimation with actor constants/refunds; end‑to‑end tests once actor/runtime land; receipt/log polish only if FVM attribution needs it; remove guardrails after network upgrade.
- FVM/Rust (remaining): Delegator actor (HAMT state, ApplyDelegations with ecrecover, nonce checks, gas/refunds); EVM runtime delegation for EOAs with empty code; receipts/logs attribution; protocol gating at network upgrade; conformance/integration tests.

**Status (This Commit)**
- Phase‑1 support on the JSON‑RPC/types path is implemented and tested.
- Adds EIP‑7702 typed transaction support on the Ethereum JSON‑RPC/types layer:
  - `EIP7702TxType = 0x04` constant.
  - Parser/encoder for type `0x04` with `authorizationList` per EIP‑7702.
  - `EthTx` extended to include `authorizationList` in RPC views.
  - Dispatch wired in `ParseEthTransaction`.
  - Guard in `ToUnsignedFilecoinMessage` returning a helpful error until actor/FVM wiring exists.
  - RLP decoder limit updated to support 13‑field payloads (7702) in `rlp.go`.
  - Unit tests for 7702: round‑trip encode/decode and guard rails.
- Feature flag scaffold for send‑path:
  - `ethtypes.Eip7702FeatureEnabled` (default false; set true via build tag `eip7702_enabled`).
  - `ethtypes.DelegatorActorAddr` placeholder for the deployed actor address.
  - `CborEncodeEIP7702Authorizations` helper to build CBOR params for delegations.
- Additional tests:
  - CBOR params shape test validating `[chain_id, address(20), nonce, y_parity, r, s]` tuples.
  - Validation: inner `authorizationList[*].y_parity` must be 0 or 1 (parser + test).
  - Cross-package: delegator decodes CBOR produced by ethtypes encoder (compat check).
- Delegator actor stub:
  - `Actor.ApplyDelegations(params []DelegationParam) error` placeholder (no‑op for now).
- Delegator param handling & validation (scaffold):
  - `DecodeAuthorizationTuples([]byte) ([]DelegationParam, error)` decodes CBOR array of 6‑tuples.
  - `ValidateDelegations([]DelegationParam, localChainID)` checks chainId∈{0,local}, y_parity∈{0,1}, non‑zero r/s, and low‑s.
  - `State.ApplyDelegationsWithAuthorities(nonces, authorities, list)` applies mappings with nonce checks (scaffold helper; authority recovery TBD).
- Node scaffolding:
  - Gas: `compute7702IntrinsicOverhead` stub and wiring in `EthEstimateGas` when targeting Delegator actor.
  - Receipts: `adjustReceiptForDelegation` stub and `authorizationList` echoed in receipts (omitempty).

**Files To Inspect**
- `chain/types/ethtypes/` (RLP, types, parsers; 7702 support lives here)
- `node/impl/eth/` (send path, gas estimation, receipts)
- `chain/actors/builtin/delegator/` (new; actor scaffold and validation)
- `chain/actors/builtin/evm/` (runtime; for delegated execution path)
- Rust runtime tests and EVM integration tests live in the sibling repo `../builtin-actors`.
  Additions like test harness helpers (e.g., “any params” matcher) and EVM CALL-path tests should be
  implemented there (see `runtime/src/test_utils.rs` and `actors/evm/tests/`).
  When changing builtin-actors code, remember to `cd ../builtin-actors`, make changes there, and commit in that repo.

**Quick Validation**
- Build: `go build ./chain/types/ethtypes`
- Focused tests: `go test ./chain/types/ethtypes -run 7702 -count=1`
- Full package tests: `go test ./chain/types/ethtypes -count=1`
- Delegator validation tests: `go test ./chain/actors/builtin/delegator -count=1`
- Expected error when converting to Filecoin message (until actor wiring):
  - `EIP-7702 not yet wired to actors/FVM; parsed OK but cannot construct Filecoin message (enable actor integration to proceed)`

Tip: to exercise the send‑path guard with the feature flag on, run tests with build tag `-tags eip7702_enabled`.

**What Remains (Phase‑2 Back‑Half)**
- Actor + FVM plumbing to apply delegations and route execution via delegate code.
  - Preferred: new Delegator system actor with state `Map<EOA, DelegateAddr>`.
  - Alternative: integrate into EVM actor with a compact mapping and code‑lookup path.
- EVM runtime: when calling an EOA with empty code, if a delegation exists, execute delegate code as callee.
- Flip `Eth7702TxArgs.ToUnsignedFilecoinMessage` to target Delegator actor with CBOR‑encoded tuples.
- Mempool policy: per‑EOA pending‑delegation caps and cross‑account nonce invalidation rules.
- Gas estimation: simulate delegation writes and intrinsic costs/refunds (`PER_EMPTY_ACCOUNT_COST`, etc.).
- RPC receipts/logs: ensure `authorizationList` echoes and logs reflect delegate execution context.
  - Implemented: Lotus receipts include optional `delegatedTo` for 7702 txs (from `authorizationList`)
    and for CALL→EOA delegated execution detected via a synthetic EVM log topic `keccak("EIP7702Delegated(address)")`.
    EVM runtime now emits this ETH-style log when delegation occurs; Lotus scans logs to populate `delegatedTo`.

Note: Lotus mempool policies are already implemented and are gated by network version; no user config is needed at/after activation. Gas/estimation currently uses conservative placeholders pending actor constants.

- Tracking checklist (to drive Phase‑2 to completion):
- [x] Delegator state storage implemented with HAMT (Rust FVM) and codegen types (actors/delegator/state.rs).
- [x] Authority recovery from `(r,s,y_parity)` implemented, including nonce lookup and increment.
- [x] `ApplyDelegations` decode/validate/nonce‑bump/write path implemented (actors/delegator).
- [x] EVM runtime consults Delegator mapping (CALL → EOA) and logs mapping; execution-in-EOA-context wiring pending.
- [x] `Eth7702TxArgs.ToUnsignedFilecoinMessage` targets Delegator using CBOR tuples and correct method number (behind `eip7702_enabled`).
- [x] Env-based Delegator actor address configuration (`LOTUS_ETH_7702_DELEGATOR_ADDR`).
- [x] Gas estimation accounts for intrinsic overhead (placeholder constants) and counts tuples.
- [x] Mempool policy added for pending delegation caps (per‑EOA, conservative default).
- [x] RPC receipts echo `authorizationList` (already carried via tx view).

Notes:
- Delegator actor added in builtin-actors at `actors/delegator` with HAMT-backed state and methods:
  - `ApplyDelegations(Vec<DelegationParam>)` (codec: DAG-CBOR) applies validated mappings and bumps nonces.
  - `LookupDelegate(authority EthAddress) -> Option<EthAddress>` (codec: CBOR) for runtime queries.
- Authority recovery (secp256k1 ecrecover over keccak(rlp(chain_id,address,nonce))) implemented in Delegator actor using runtime `recover_secp_public_key`.
- EVM runtime delegation hook implemented: CALL to EOA consults Delegator; executes delegate via internal `InvokeAsEoa` trampoline. Tests added in `../builtin-actors/actors/evm/tests/call_delegated_eoa.rs`.
- Delegation cap is configurable via env `LOTUS_ETH_7702_DELEGATION_CAP`.

**Concrete Next Steps**
- Complete Delegator actor implementation:
  - State: `HAMT<EOA, DelegateAddr>` (or AMT) and authority nonces. Start by replacing the in‑memory map in `state.go` with HAMT bindings and cbor‑gen types.
  - Params (CBOR): array of tuples `(chainId uint64, authority EthAddress, nonce uint64, yParity byte, r EthBigInt, s EthBigInt)`; the on‑chain struct is `delegator.DelegationParam` in `types.go`.
  - Method `ApplyDelegations`: validate per EIP‑7702 (correct chain, low‑s, `v∈{0,1}`, authority nonce match, no nested delegation), write mapping, bump nonce, charge gas, process refunds. Current placeholder is `Actor.ApplyDelegations` in `actor_stub.go`.
- Wire EVM runtime:
  - On message to EOA with empty code, check Delegator mapping; if present, load delegate code and execute as callee code. Integration lives in `chain/actors/builtin/evm/` call dispatch.
- Update send path:
  - With build tag `eip7702_enabled` and `DelegatorActorAddr` configured at process init, have `Eth7702TxArgs.ToUnsignedFilecoinMessage` set `To: DelegatorActorAddr`, `Method: delegator.MethodApplyDelegations`, and `Params: CborEncodeEIP7702Authorizations(authz)`.
- Extend gas estimation:
  - Implement `compute7702IntrinsicOverhead(len(authz))` in `node/impl/eth/` with concrete constants, matching Delegator actor charges.
  - Simulate Delegator mapping writes; apply any empty‑account refunds.
- Policy:
  - Define mempool limits for pending delegations per EOA and cross‑account nonce rules; enforce in message pool.
  - Policy is enabled at/after the network version activation (no user config).

**Suggested Tests (after actor wiring)**
- Unit: `ApplyDelegations` validates tuples (chainId, low‑s, `v∈{0,1}`), handles nonce increments, writes mappings, charges gas, and applies refunds.
  - Already added: unit tests for `State.ApplyDelegationsWithAuthorities` covering writes and nonce mismatches (`chain/actors/builtin/delegator/validation_test.go`).
- Integration: sending a 0x04 tx routes to Delegator; subsequent call to EOA executes delegate code.
- RPC: receipts include `authorizationList`, logs attributed to delegate, correct bloom updates.
- Policy: mempool caps enforce deterministic behavior under delegation load.

Build and test with feature flag enabled:
- `go test ./... -tags eip7702_enabled -count=1`

**Editing Strategy**
- Use small, focused diffs with anchored context (mirror `eth_1559_transactions.go` style where possible).
- For new files, include full contents in diffs.
- Keep edits minimal and scoped; avoid unrelated refactors.

**Acceptance Criteria (Phase‑2)**
- A signed type‑`0x04` tx decodes, constructs a Filecoin message calling Delegator actor, applies valid delegations, and executes delegate code when calling EOAs.
- JSON‑RPC returns `authorizationList` in tx views and receipts.
- Mempool and gas estimation behave deterministically under delegation load.

**Handy Commands**
- Build fast path: `go build ./chain/types/ethtypes`
- Focused tests: `go test ./chain/types/ethtypes -run 7702 -count=1`
- Full package tests: `go test ./chain/types/ethtypes -count=1`

**Notes to Reviewers**
- Phase‑1 is additive and safe to merge independently; it does not require actor/FVM changes and keeps legacy/EIP‑1559 behavior intact.
- `chain/types/ethtypes/eth_7702_params_test.go` validates CBOR encoding structure using a `cbor-gen` reader.
- `chain/actors/builtin/delegator/actor_stub.go` outlines the intended API.
