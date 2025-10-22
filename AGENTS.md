# EIP-7702 Implementation Notebook (Lotus + Builtin Actors)

This notebook tracks the end‑to‑end EIP‑7702 implementation across Lotus (Go) and builtin‑actors (Rust), including current status, remaining tasks, and validation steps.

**Purpose**
- Provide a concise, actionable plan to complete EIP‑7702.
- Document current status, remaining work, and how to validate.

**Status Overview**
- Lotus/Go (done):
  - Typed 0x04 parsing/encoding with `authorizationList`; dispatch via `ParseEthTransaction`.
  - `EthTx` and receipts echo `authorizationList`; receipt adjuster surfaces `delegatedTo` from tuples or synthetic event.
  - Send path wired behind `-tags eip7702_enabled`: builds a Filecoin message targeting Delegator.ApplyDelegations with CBOR‑encoded tuples.
  - Mempool policies (cross‑account invalidation; per‑EOA cap), gated by network version.
  - Gas estimation scaffold adds intrinsic overhead per tuple.
  - Go‑side Delegator helpers: tuple decode/validation (chainId, yParity, low‑s), state apply with nonce checks (for tests).
- Builtin‑actors/Rust (done):
  - Delegator actor with HAMT‑backed mapping and authority nonces; methods: Constructor, ApplyDelegations (decode, validate, ecrecover, nonce check, write), LookupDelegate, Get/PutStorageRoot.
  - EVM runtime CALL→EOA hook: consults Delegator; executes delegate via `InvokeAsEoa`; emits `EIP7702Delegated(address)` for attribution; gated by `NV_EIP_7702`.
- In‑progress alignment:
  - CBOR params shape aligned to actor: wrapper tuple `ApplyDelegationsParams{ list: Vec<DelegationParam> }`.
  - Lotus decoder and gas counter accept both top‑level array and wrapper forms for robustness.
  - Default Delegator actor address when feature is enabled is `ID:18` (env override supported).
  - Gating unified to a single NV constant placeholder to avoid drift.

**What Remains**
- Gas constants/refunds: finalize authoritative costs in actor/runtime and mirror in Lotus estimation (placeholders currently used in Lotus).
- E2E tests in Lotus once wasm bundle is buildable in this environment; validate: 0x04 tx applies delegations; CALL→EOA executes delegate; receipts/logs attribution; policies behave as expected post‑activation.
- Optional receipt polish depending on explorer requirements.

**Quick Validation**
- Lotus fast path:
  - `go build ./chain/types/ethtypes`
  - `go test ./chain/types/ethtypes -run 7702 -count=1`
  - `go test ./chain/actors/builtin/delegator -count=1`
- Builtin‑actors (local toolchain permitting): `cargo test`.
- Lotus E2E (requires FFI + Delegator in bundle):
  - `go test ./itests -run Eth7702 -tags eip7702_enabled -count=1`
  - Test `TestEth7702_SendRoutesToDelegator` is implemented and currently skipped by default until the Delegator actor is included in the network bundle in this environment.

To route 0x04 transactions, build Lotus with `-tags eip7702_enabled` and set `LOTUS_ETH_7702_DELEGATOR_ADDR` (defaults to `ID:18` when enabled).

**Files & Areas**
- Lotus:
  - `chain/types/ethtypes/` (tx parsing/encoding; CBOR params; types)
  - `node/impl/eth/` (send path; gas estimation; receipts)
  - `chain/messagepool/` (mempool policies for 7702)
  - `chain/actors/builtin/delegator/` (Go helpers for validation/testing)
- Builtin‑actors:
  - `actors/delegator/` (actor implementation; tests)
  - `actors/evm/` (runtime CALL delegation; `InvokeAsEoa`)
  - `runtime/src/features.rs` (activation NV)

**Editing Strategy**
- Keep diffs small and scoped. Mirror existing style (e.g., 1559 code) where possible.
- When changing encodings, update both encoder/decoder and tests; prefer backward‑compatible decoders.
- Unify activation gating across repos to a single NV constant and avoid hard‑coding disparate values.

**Acceptance Criteria**
- A signed type‑0x04 tx decodes, constructs a Filecoin message calling Delegator.ApplyDelegations, applies valid delegations, and subsequent CALL→EOA executes delegate code.
- JSON‑RPC returns `authorizationList` and `delegatedTo` where applicable.
- Mempool policy behaves deterministically; gas estimation accounts for tuple overhead.

**Env/Flags**
- Build tag: `eip7702_enabled` enables send‑path in Lotus.
- Env: `LOTUS_ETH_7702_DELEGATOR_ADDR` for Delegator actor address (defaults to `ID:18` when enabled). `LOTUS_ETH_7702_DELEGATION_CAP` to adjust per‑EOA cap.
