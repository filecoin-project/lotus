# feat(consensus): tipset gas reservations and reservation‑aware mpool pre‑pack

This PR wires Lotus into the engine‑managed tipset gas reservation design described in `AGENTS.md` (Option A), and adds an optional reservation‑aware mempool pre‑pack pass. The goal is to eliminate miner‑charged underfunded messages in a tipset while preserving receipts and gas outputs.

## Summary

- Add NV‑gated Begin/End orchestration around explicit tipset messages using the new FVM reservation session API.
- Build a per‑sender reservation plan in consensus (`Σ(gas_limit * gas_fee_cap)`), deduped by CID across blocks.
- Enforce strict vs non‑strict behaviour pre‑activation via explicit feature flags, with ref‑fvm remaining network‑version agnostic.
- Add an optional reservation‑aware mpool pre‑pack heuristic that avoids constructing blocks which would fail reservations.
- Expose plan‑level metrics and operator documentation.

This branch assumes matching `multistage-execution` branches in `ref-fvm` (engine reservations) and `filecoin-ffi` (Begin/End FFI) are present.

## Changes

### Consensus and activation

- **`chain/consensus/features.go`**
  - Introduce `ReservationFeatureFlags` with:
    - `MultiStageReservations` (best‑effort pre‑activation enablement).
    - `MultiStageReservationsStrict` (pre‑activation behaviour: legacy fallback vs tipset‑invalidating).
  - Provide `Feature` globals with env‑based defaults:
    - `LOTUS_ENABLE_TIPSET_RESERVATIONS`
    - `LOTUS_ENABLE_TIPSET_RESERVATIONS_STRICT`

- **`chain/consensus/reservations.go`**
  - Implement `ReservationsEnabled(nv)`:
    - `nv >= ReservationsActivationNetworkVersion()` → always true (reservations required).
    - Pre‑activation → controlled by `Feature.MultiStageReservations`.
  - Implement `buildReservationPlan(bms []FilecoinBlockMessages) map[address.Address]abi.TokenAmount`:
    - Deduplicate messages by CID across all blocks in the tipset.
    - Aggregate `gas_limit * gas_fee_cap` per sender.
  - Implement `startReservations` / `endReservations`:
    - Skip empty plans (no Begin/End).
    - Call `vmi.StartTipsetReservations` / `EndTipsetReservations` when enabled.
    - Record `metrics.ReservationPlanSenders` and `metrics.ReservationPlanTotal`.
    - Use `handleReservationError` to decide, pre‑activation:
      - Always fallback for `ErrReservationsNotImplemented`.
      - Always surface node‑error classes (session misuse, overflow, invariant violations).
      - For `ErrReservationsInsufficientFunds` / `ErrReservationsPlanTooLarge`:
        - Non‑strict: log and fall back to legacy mode.
        - Strict: surface as tipset errors.
    - Post‑activation: all Begin/End errors surface; `ErrReservationsNotImplemented` becomes a node error.

- **`chain/consensus/compute_state.go`**
  - After constructing the VM, wrap explicit message application with:
    - `startReservations(ctx, vmi, bms, nv)` (before applying any explicit messages).
    - A `defer`ed `endReservations(ctx, vmi, nv)` plus an explicit `endReservations` call before cron to scope reservations to explicit messages only.
  - Existing message, reward, and cron flows are otherwise unchanged.

### VM interface and FVM wiring

- **`chain/vm/vmi.go` & `chain/vm/execution.go`**
  - Extend `vm.Interface` with:
    - `StartTipsetReservations(ctx context.Context, plan map[address.Address]abi.TokenAmount) error`
    - `EndTipsetReservations(ctx context.Context) error`
  - Implement `vmExecutor.StartTipsetReservations` / `EndTipsetReservations` by:
    - Acquiring an execution lane token (consistent with `ApplyMessage` / `ApplyImplicitMessage`).
    - Forwarding the call to the underlying VM implementation.

- **`chain/vm/fvm.go`**
  - Re‑export typed reservation errors from `filecoin-ffi` and define:
    - `ReservationsActivationNetworkVersion()` (currently `network.Version28`).
  - Implement:
    - `func (vm *FVM) BeginReservations(plan []byte) error`
    - `func (vm *FVM) EndReservations() error`
    - `func (vm *FVM) StartTipsetReservations(ctx context.Context, plan map[address.Address]abi.TokenAmount) error`
    - `func (vm *FVM) EndTipsetReservations(ctx context.Context) error`
  - Behaviour:
    - CBOR encode the reservation plan as `[[address_bytes, amount_bytes], ...]`.
    - Call `ffi.FVM.BeginReservations` / `ffi.FVM.EndReservations`, which return typed errors potentially wrapped with short engine‑provided messages from ref‑fvm.
    - Track `reservationsActive` to avoid calling End when no Begin occurred (empty plan or legacy fallback).
    - Pre‑activation:
      - Treat `ErrReservationsNotImplemented` as a benign signal and fall back to legacy mode.
      - Surface other errors; consensus decides fallback vs invalidity via `handleReservationError`.
    - Post‑activation:
      - Treat `ErrReservationsNotImplemented` as a node error (engine too old).

- **`chain/vm/vm.go`**
  - Implement no‑op `StartTipsetReservations` / `EndTipsetReservations` on `LegacyVM` to satisfy the interface; reservations are only supported in FVM mode.

- **`chain/vm/fvm_reservations_test.go`**
  - Tests for:
    - `reservationStatusToError` mapping from raw status codes to typed errors.
    - CBOR plan encode/decode round‑trip.
    - Validation (duplicate sender and invalid entry length).

### Mempool reservation‑aware pre‑pack (optional)

- **`chain/types/mpool.go` & `chain/messagepool/config.go`**
  - Add `EnableReservationPrePack` to `MpoolConfig`, default `false`.
  - Document it as advisory-only pre‑pack simulation that shares the same activation gating as tipset reservations.

- **`chain/messagepool/selection.go`**
  - Extend `selectedMessages` with:
    - `reservationEnabled`, `reservationCtx`, `reservationTipset`.
    - `reservedBySender` (`Σ(cap×limit)` per sender for selected messages).
    - `balanceBySender` (sender balances at the selection base tipset).
  - Add:
    - `reservationsPrePackEnabled() bool`:
      - Uses current tipset height + 1 to derive the NV at which the selected messages will execute.
      - Requires both `EnableReservationPrePack` and `consensus.ReservationsEnabled(nextNv)`.
    - `initReservations(ctx, ts, mp)` to initialize tracking maps when enabled.
    - `reserveForMessages(sender, msgs)` to:
      - Lazily load the sender’s balance at the selection base state via `mp.api.StateGetActor`/`GetActorAfter` and cache it.
      - Accumulate Σ(cap×limit) for candidate chains and compare against the sender’s balance.
      - Skip/disable heuristics on lookup failure to avoid partial behaviour.
    - Use `reserveForMessages` from `tryToAdd`/`tryToAddWithDeps` to:
      - Prune or trim chains that would over‑commit the sender at the selection base state.
      - Keep behaviour advisory only; consensus still enforces reservations at Begin/End.

- **`chain/messagepool/selection_test.go`**
  - `TestReservationPrePackPrunesOverCommittedChain`:
    - Ensures chains whose Σ(cap×limit) exceeds the sender balance are invalidated and not added.
  - `TestReservationPrePackAccumulatesCapTimesLimit`:
    - Ensures Σ(cap×limit) is correctly accumulated per sender across multiple chains.

### Metrics and docs

- **`metrics/metrics.go`**
  - Add:
    - `ReservationPlanSenders` (`vm/reservations_plan_senders`) – number of unique senders in the plan.
    - `ReservationPlanTotal` (`vm/reservations_plan_total_atto`) – total reserved Σ(cap×limit) in attoFIL.
  - Register views and attach them to the default node view set.

- **`documentation/en/tipset-reservations.md`**
  - Document:
    - Activation at `network.Version28` / `UpgradeXxHeight`.
    - Pre‑activation env/feature gating, including Strict vs non‑Strict behaviours.
    - Advisory nature of mempool pre‑pack.
    - Plan‑level metrics and logging for observability.

- **`CHANGELOG.md`**
  - Add UNRELEASED entry summarizing:
    - Tipset gas reservations.
    - Reservation‑aware mpool pre‑pack.
    - NV28 activation and receipts/gas‑output invariants.

## Activation & gating

- Ref‑fvm remains network‑version agnostic; all activation logic lives in Lotus.
- **Pre‑activation (NV < 28)**:
  - `Feature.MultiStageReservations` = false → legacy mode (no Begin/End, no pre‑pack).
  - `Feature.MultiStageReservations` = true, `MultiStageReservationsStrict` = false:
    - Begin/End attempted; reservation failures (insufficient funds, plan too large) are best‑effort and cause a fallback to legacy for that tipset.
  - `Feature.MultiStageReservations` = true, `MultiStageReservationsStrict` = true:
    - Reservation failures invalidate the tipset pre‑activation (mirroring post‑activation), except `ErrReservationsNotImplemented`, which still triggers legacy fallback.
- **Post‑activation (NV ≥ 28)**:
  - `ReservationsEnabled` always true.
  - Begin/End errors surface to consensus; `ErrReservationsNotImplemented` is treated as a node error (engine too old).

## Testing

- Unit tests:
  - `chain/consensus/reservations_test.go` – plan build, env/feature gating, error handling paths.
  - `chain/vm/fvm_reservations_test.go` – status‑to‑error mapping, CBOR plan encode/decode.
  - `chain/messagepool/selection_test.go` – reservation pre‑pack heuristics.
- Targeted `go test` runs (already exercised locally on this branch):
  - `go test ./chain/consensus -run TestReservation`
  - `go test ./chain/vm -run TestReservation`
  - `go test ./chain/messagepool -run TestReservation`

## Notes

- This PR depends on:
  - `ref-fvm` `multistage-execution` (engine reservation session + enforcement).
  - `filecoin-ffi` `multistage-execution` (Begin/End FFI + error messages).
- Receipts (`ExitCode`, `GasUsed`, events) and `GasOutputs` remain unchanged; the reservation ledger is internal to the engine.

