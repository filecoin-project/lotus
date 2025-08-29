**Purpose**
- Provide a concise, actionable notebook for agents to continue EIP‑7702 implementation in Lotus.
- Capture what’s already implemented (Phase‑1 front‑half) and what remains (actor/FVM + policy).

**Status (This Commit)**
- Adds EIP‑7702 typed transaction support on the Ethereum JSON‑RPC/types layer:
  - `EIP7702TxType = 0x04` constant.
  - Parser/encoder for type `0x04` with `authorizationList` per EIP‑7702.
  - `EthTx` extended to include `authorizationList` in RPC views.
  - Dispatch wired in `ParseEthTransaction`.
  - Clear guard in `ToUnsignedFilecoinMessage` returning a helpful error until actor/FVM wiring exists.
  - RLP decoder limit updated to support 13‑field payloads (7702) in `rlp.go`.
  - Unit tests added for 7702: round‑trip encode/decode and guard rails.
  - Feature flag scaffold for send‑path:
    - `ethtypes.Eip7702FeatureEnabled` (default false; set true via build tag `eip7702_enabled`).
    - `ethtypes.DelegatorActorAddr` placeholder for the deployed actor address.
    - `CborEncodeEIP7702Authorizations` helper to build CBOR params for delegations.
  - Additional tests:
    - CBOR params shape test validating `[chain_id, address(20), nonce, y_parity, r, s]` tuples.
  - Delegator actor stub:
    - `Actor.ApplyDelegations(params []DelegationParam) error` placeholder (no‑op for now).

**Files Touched**
- `chain/types/ethtypes/eth_transactions.go`
  - New constant: `EIP7702TxType = 0x04`.
  - `EthTx` gains `AuthorizationList []EthAuthorization` (JSON `authorizationList`).
  - Dispatch extended: routes `0x04` to `parseEip7702Tx`.
- `chain/types/ethtypes/eth_7702_transactions.go` (new)
  - Defines `EthAuthorization` and `Eth7702TxArgs` implementing `EthTransaction`.
  - Implements RLP encode/decode (unsigned/signed), `TxHash`, `Signature`, `Sender`, `ToEthTx`.
  - `ToUnsignedFilecoinMessage` returns explicit error pending actor integration.
  - Now gated by `Eip7702FeatureEnabled`: when enabled and `DelegatorActorAddr` is set, builds a `types.Message` targeting the Delegator actor with CBOR‑encoded authorization tuples.
- `chain/types/ethtypes/eth_7702_params.go` (new)
  - `CborEncodeEIP7702Authorizations` encodes `[chain_id, address, nonce, y_parity, r, s]` tuples for Delegator params.
- `chain/types/ethtypes/eth_7702_featureflag.go` (new)
  - Declares `Eip7702FeatureEnabled` (false) and `DelegatorActorAddr`.
- `chain/types/ethtypes/eth_7702_featureflag_enabled.go` (new, build‑tagged)
  - `//go:build eip7702_enabled` sets `Eip7702FeatureEnabled = true`.
- `chain/actors/builtin/delegator/` (new, scaffold)
  - `README.md`: describes intended actor behavior and next steps.
  - `types.go`: defines `DelegationParam` (tuple shape) and placeholder method num `MethodApplyDelegations`.
  - `state.go`: placeholder state `Delegations map[address.Address][20]byte`.
  - `actor_stub.go`: no-op `ApplyDelegations(params []DelegationParam) error`.

**Quick Validation**
- Build: `go build ./chain/types/ethtypes`
- Tests (package-level): `go test ./chain/types/ethtypes -count=1`
- Expected failure message when attempting to convert to Filecoin message:
  - `EIP-7702 not yet wired to actors/FVM; parsed OK but cannot construct Filecoin message (enable actor integration to proceed)`

**What Remains (Phase‑2 Back‑Half)**
- Actor + FVM plumbing to apply delegations and route execution via delegate code:
  - Option A (preferred for isolation): new Delegator system actor `chain/actors/builtin/delegator/` with state `Map<EOA, DelegateAddr>`.
    - Method `ApplyDelegations(params []DelegationParam)` validates authorization tuples and writes delegation indicator; increments authority nonces; accounts gas/refunds.
  - Option B: integrate into EVM actor with a compact mapping and code‑lookup path.
  - EVM runtime change: on call to an EOA with empty code, if a delegation exists, execute the delegate’s code as the target.
- Flip `Eth7702TxArgs.ToUnsignedFilecoinMessage` to construct a `types.Message` targeting the new actor/method with CBOR‑encoded tuples.
- Mempool policy: per‑EOA pending‑delegation caps and cross‑account nonce invalidation.
- RPC receipts/logs: ensure `authorizationList` echoes and logs reflect delegate execution context.
- Gas estimation: simulate delegation writes and intrinsic costs/refunds (`PER_EMPTY_ACCOUNT_COST`, etc.).

**Concrete Next Steps for an Agent**
- Complete Delegator actor implementation:
  - Path: `chain/actors/builtin/delegator/`.
  - State: `HAMT<EOA, DelegateAddr>` or AMT mapping.
  - Params type (CBOR): array of tuples `(chainId uint64, authority EthAddress, nonce uint64, yParity byte, r EthBigInt, s EthBigInt)`.
  - Method: `ApplyDelegations` — validates per EIP‑7702 (correct chain, low‑s, V ∈ {0,1}, authority nonce match, no nested delegation), writes mapping, bumps nonce, charges gas, processes refunds.
- Wire EVM runtime:
  - On message to EOA with empty code, check delegator mapping; if present, load delegate code and execute as callee code.
- Update `Eth7702TxArgs.ToUnsignedFilecoinMessage`:
  - With `eip7702_enabled` build tag set and `DelegatorActorAddr` configured, send `types.Message{ To: DelegatorActorAddr, Method: ApplyDelegations, Params: CborEncodeEIP7702Authorizations(authz) }`.
- Extend gas estimation:
  - In `node/impl/eth` flow, simulate delegation writes and intrinsic costs per tuple.
  - Use a helper like `compute7702IntrinsicOverhead(len(authz))` to add intrinsic cost; simulate Delegator mapping writes for estimation.
  - If refunds apply for empty accounts, model them per EIP-7702.

**Suggested Next Tests (after actor wiring)**
- `ApplyDelegations` unit tests: tuple validation (chainId, low-s, V ∈ {0,1}), nonce increments, mapping writes, and gas/refund accounting.
- Integration: sending a 0x04 tx results in Delegator call; subsequent call to EOA executes delegate code.
- RPC: receipts include `authorizationList`, logs attributed to delegate, correct bloom updates.

**Tests Added (7702)**
- `TestEIP7702_RLPRoundTrip`: ensures encode/decode is lossless and fields match.
- `TestEIP7702_EmptyAuthorizationListRejected`: parser rejects empty `authorizationList`.
- `TestEIP7702_NonEmptyAccessListRejected`: parser rejects non‑empty access list per current Lotus policy.
- `TestEIP7702_VParityRejected`: parser rejects outer `v` not in `{0,1}`.
- `TestEIP7702_ToUnsignedFilecoinMessage_Guard`: verify front‑half guard error prior to actor wiring.

Run with: `go test ./chain/types/ethtypes -count=1`

**Pointers (Open These Paths/Files)**
- Repo paths:
  - `chain/types/ethtypes/` (this patch; RLP, types, parsers)
  - `node/impl/eth/` (send path, gas estimation, receipts) — see `gas_7702_scaffold.go`, `receipt_7702_scaffold.go`.
  - `chain/actors/builtin/evm/` (runtime) and new `chain/actors/builtin/delegator/` (to add)
- Spec: https://eips.ethereum.org/EIPS/eip-7702 (type 0x04, 13-field list, authorization tuple)

**Editing Strategy for Agents**
- Use unified diffs or “diff‑sandwich” with anchored context.
- For new files, provide full contents.
- Keep edits minimal and scoped; mirror the style of `eth_1559_transactions.go` where possible.

**Acceptance Criteria for Phase‑2**
- A signed type‑`0x04` tx decodes, constructs a Filecoin message calling Delegator actor, applies valid delegations, and executes delegate code when calling EOAs.
- JSON‑RPC returns `authorizationList` in tx views and receipts.
- Mempool and gas estimation behave deterministically under delegation load.

**Handy Commands**
- Build fast path: `go build ./chain/types/ethtypes`
- Focused tests: `go test ./chain/types/ethtypes -run 7702 -count=1`
- Full package tests: `go test ./chain/types/ethtypes -count=1`

**Note to Reviewers**
- Phase‑1 is additive and safe to merge independently; it does not require actor/FVM changes and keeps legacy/EIP‑1559 behavior intact.
- `chain/types/ethtypes/eth_7702_params_test.go` (new)
  - Validates CBOR encoding structure using `cbor-gen` reader.
- `chain/actors/builtin/delegator/actor_stub.go` (new)
  - Minimal stub to outline intended API.
