## Related Issues
Related FIP PR Link: https://github.com/filecoin-project/FIPs/pull/1209

## Proposed Changes
- Add EIP-7702 (type 0x04) transaction support in `chain/types/ethtypes`:
  - RLP decode/encode for 0x04 including `authorizationList` (6-field tuples).
  - Per-type RLP element limit (13 for 0x04); tuple arity and `yParity` validation (0/1 only).
  - Authorization domain helpers: `AuthorizationPreimage` and `AuthorizationKeccak` implementing `keccak256(0x05 || rlp([chain_id,address,nonce]))`.
  - Canonical CBOR params encoders: wrapper list for authorizations, and atomic ApplyAndCall params `[ [tuple...], [to(20), value, input] ]`.
  - Robust integer parsing for `chain_id`/`nonce` up to `uint64`; reject non‑canonical encodings.
- Route 0x04 to the EthAccount actor’s atomic ApplyAndCall entrypoint (current design; the earlier EVM.ApplyAndCall/Delegator path has been removed on this branch):
  - `Eth7702TxArgs.ToUnsignedFilecoinMessageAtomic` builds a Filecoin message targeting `EthAccount.ApplyAndCall` (FRC-42 method hash) with canonical CBOR params.
  - Feature‑gated by `-tags eip7702_enabled`; adds `Eip7702FeatureEnabled` flag and an `EthAccountApplyAndCallActorAddr` actor address stub used by tests (the older `EvmApplyAndCallActorAddr` remains as a deprecated alias for historical compatibility).
  - No Delegator path on this branch; routing is via EthAccount + VM intercept.
- Receipts attribution for delegated execution:
  - `adjustReceiptForDelegation` surfaces `delegatedTo` from `authorizationList` and, if absent, from a synthetic event topic emitted by the EVM runtime.
  - Topic keyed as `keccak("Delegated(address)")`; data is a 32‑byte ABI word whose last 20 bytes form the authority (EOA) address.
- Gas estimation alignment to actor behavior:
  - Behavioral intrinsic overhead applied when `ApplyAndCall` is targeted and `authorizationList` is non‑empty.
  - Overhead grows monotonically with tuple count; disabled otherwise. Tuple counting is derived by CBOR shape inspection only.
- OpenRPC updates: include 0x04 transaction shape and receipt fields so JSON‑RPC returns echo `authorizationList` and `delegatedTo` when present.
- Tests and fuzzing:
  - RLP decoding tests for tuple arity, `yParity`, per‑type list limit, canonical encoding, and overflow boundaries.
  - AuthorizationKeccak vectors for stability.
  - Receipts attribution tests covering both tuple echo and synthetic event extraction, precedence, multi‑event, and data parsing.
  - Gas estimation tests validating behavioral overhead application and monotonicity (no numeric pinning).
  - E2E scaffold `itests/Eth7702` to exercise atomic apply‑and‑call once the wasm bundle includes `ApplyAndCall`.
  - Mempool regression tests to ensure standard policies remain unchanged for 0x04 ingress.

## Additional Info
- Branch scope: internal development branch; EthAccount + VM intercept; no backward compatibility preserved. Canonical CBOR only (legacy shapes removed).
- Activation: route enabled via build tag `eip7702_enabled`; actor bundle controls consensus activation. No runtime NV gates in Lotus.
- Event topic: actor and Lotus both use `Delegated(address)` in final form for synthetic delegated attribution (see `adjustReceiptForDelegation` and ref‑fvm intercept tests).
- Gas model: FEVM runs under FVM gas; estimation is behavioral. Tests avoid pinning absolute gas constants and effective prices.
- Quick validation:
  - `go test ./chain/types/ethtypes -run 7702 -count=1`
  - `go test ./node/impl/eth -run 7702 -count=1`
  - E2E (post‑wasm): `go test ./itests -run Eth7702 -tags eip7702_enabled -count=1`

## Checklist

Before you mark the PR ready for review, please make sure that:

- [ ] Commits have a clear commit message.
- [ ] PR title conforms with [contribution conventions](https://github.com/filecoin-project/lotus/blob/master/CONTRIBUTING.md#pr-title-conventions)
- [ ] Update CHANGELOG.md or signal that this change does not need it per [contribution conventions](https://github.com/filecoin-project/lotus/blob/master/CONTRIBUTING.md#changelog-management)
- [ ] New features have usage guidelines and / or documentation updates in
  - [ ] [Lotus Documentation](https://lotus.filecoin.io)
  - [ ] [Discussion Tutorials](https://github.com/filecoin-project/lotus/discussions/categories/tutorials)
- [ ] Tests exist for new functionality or change in behavior
- [ ] CI is green
