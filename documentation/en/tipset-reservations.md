# Tipset Gas Reservations (Multi‑Stage Execution)

This document summarizes activation, operational behaviour, and observability for tipset‑scope gas reservations as implemented in this branch.

## Activation and Gating

- **Network version:** reservations become consensus‑critical at **network.Version28** (the “Xx” network upgrade). The activation epoch is the upgrade height for Version28 on each network (see `build/buildconstants/params_*.go` for the concrete `UpgradeXxHeight` per network).
- **Pre‑activation behaviour (nv < 28):**
  - Tipset reservations are controlled by two Lotus feature flags (`Feature.MultiStageReservations` and `Feature.MultiStageReservationsStrict`), with environment variables providing defaults.
    - `Feature.MultiStageReservations` (default: false; enabled when `LOTUS_ENABLE_TIPSET_RESERVATIONS=1`) toggles whether Lotus attempts tipset‑scope reservations and enables reservation‑aware pre‑pack in the mempool when configured.
    - `Feature.MultiStageReservationsStrict` (default: false; can be enabled via `LOTUS_ENABLE_TIPSET_RESERVATIONS_STRICT=1` or config) controls whether non‑NotImplemented Begin/End reservation failures invalidate tipsets pre‑activation.
  - When `Feature.MultiStageReservations` is false, Lotus operates in legacy mode (no `BeginReservations` / `EndReservations` calls).
  - When `Feature.MultiStageReservations` is true and `Feature.MultiStageReservationsStrict` is **false** (non‑strict):
    - Lotus builds a per‑sender reservation plan for each applied tipset (`Σ(gas_limit * gas_fee_cap)` per sender) and calls `BeginReservations` / `EndReservations` on the FVM if the engine implements them.
    - Any Begin/End reservation failure (including `ErrReservationsInsufficientFunds` and `ErrReservationsPlanTooLarge`) is treated as best‑effort: Lotus logs and falls back to legacy mode for that tipset; consensus validity is unchanged.
  - When `Feature.MultiStageReservations` is true and `Feature.MultiStageReservationsStrict` is **true** (strict):
    - Begin/End reservation failures such as `ErrReservationsInsufficientFunds` and `ErrReservationsPlanTooLarge` cause the tipset to be considered invalid even before nv28, matching post‑activation behaviour.
    - If the engine returns `ErrReservationsNotImplemented` from `BeginReservations` or `EndReservations`, Lotus logs a debug message and continues in legacy mode (no reservations); consensus validity is unchanged.
  - In all pre‑activation modes, node‑error classes (`ErrReservationsSessionOpen`, `ErrReservationsSessionClosed`, `ErrReservationsNonZeroRemainder`, `ErrReservationsOverflow`, `ErrReservationsInvariantViolation`) surface as node errors and are **not** downgraded to best‑effort.
- **Post‑activation behaviour (nv ≥ 28):**
  - `ReservationsEnabled` is always true; Lotus will always build a reservation plan and call `BeginReservations` / `EndReservations` when applying tipsets with explicit messages.
  - If the engine returns `ErrReservationsNotImplemented` after activation, Lotus treats this as a **node error** (the node cannot validate under active rules); operators must upgrade the engine.
  - Reservation failures from the engine at Begin/End (e.g., insufficient funds at `BeginReservations`, plan too large) cause the tipset to be considered invalid under consensus; node‑error classes remain node errors.

## Feature Flags and Defaults

- **Consensus and reservations (package `chain/consensus`):**
  - `Feature.MultiStageReservations`:
    - Default: `false`, unless `LOTUS_ENABLE_TIPSET_RESERVATIONS=1` is set in the Lotus process environment at startup.
    - Pre‑activation: when `true`, Lotus attempts tipset‑scope reservations and enables reservation‑aware pre‑pack in the mempool when `EnableReservationPrePack` is set.
    - Post‑activation: ignored by consensus; reservations are always enabled for validation (`ReservationsEnabled` is always true for nv ≥ 28).
  - `Feature.MultiStageReservationsStrict`:
    - Default: `false` (non‑strict).
    - Pre‑activation: when `true`, non‑NotImplemented reservation failures at Begin/End (`ErrReservationsInsufficientFunds`, `ErrReservationsPlanTooLarge`) invalidate tipsets instead of falling back to legacy mode.
    - Post‑activation: redundant; reservations are already strict by consensus rules.
- **Mempool pre‑pack simulation:**
  - Controlled by `EnableReservationPrePack` in the mempool config (`types.MpoolConfig`, surfaced via `MpoolConfig.EnableReservationPrePack`):
    - Default: `false` (opt‑in and advisory).
    - When `true`, and when tipset reservations are enabled for the current network version (`ReservationsEnabled` is true at the selection height), the packer performs a reservation‑aware simulation using `Σ(gas_limit * gas_fee_cap)` per sender at the selection base state.
    - Chains whose additional reservation would exceed the sender’s on‑chain balance at the selection base state are skipped.
  - This pre‑pack simulation is advisory only and does not affect consensus; it simply avoids constructing blocks that would later fail reservation checks at `BeginReservations`.

## Detecting Reservation Failures

### Logs

- The consensus reservations helper logs under the `reservations` subsystem:
  - `starting tipset reservations` with fields:
    - `senders`: number of unique senders in the plan.
    - `total`: sum of all per‑sender reservations in attoFIL.
  - `skipping tipset reservations for empty plan` when no explicit messages are present.
- FVM‑level reservation errors are surfaced via the `vm` logs and the corresponding Go errors in `chain/vm/fvm.go`:
  - `ErrReservationsInsufficientFunds`
  - `ErrReservationsSessionOpen`
  - `ErrReservationsSessionClosed`
  - `ErrReservationsNonZeroRemainder`
  - `ErrReservationsPlanTooLarge`
  - `ErrReservationsOverflow`
  - `ErrReservationsInvariantViolation`
- Pre‑activation, `ErrReservationsNotImplemented` is logged at debug level and causes Lotus to continue in legacy mode for that tipset. Post‑activation, it is treated as a node error.

### Metrics

- Two OpenCensus metrics capture plan‑level reservation behaviour:
  - `vm/reservations_plan_senders` (`metrics.ReservationPlanSenders`):
    - Number of unique senders in the reservation plan for each applied tipset.
  - `vm/reservations_plan_total_atto` (`metrics.ReservationPlanTotal`):
    - Total reserved maximum gas cost across all senders in attoFIL for each tipset.
- Recommended operator checks:
  - Alert on sudden drops to zero of both metrics after nv28 on a healthy chain (may indicate reservations are not being attempted when they should be).
  - Track long‑term distribution of `reservations_plan_total_atto` to identify tipsets with unusually large reserved gas cost.

## Upgrade Expectations

- **Before nv28 (activation upgrade):**
  - Nodes may:
    - Run without reservations (default behaviour).
    - Opt‑in to reservations and reservation‑aware pre‑pack on dev/test networks by:
      - Enabling `Feature.MultiStageReservations` (e.g., via `LOTUS_ENABLE_TIPSET_RESERVATIONS=1`).
      - Optionally enabling `Feature.MultiStageReservationsStrict` to exercise strict invalidation behaviour before activation.
      - Enabling `EnableReservationPrePack` in the mempool config for advisory pre‑pack simulation.
  - The engine may or may not implement reservations; pre‑activation, `ErrReservationsNotImplemented` is treated as a benign indication that the node is running an older engine and triggers a legacy fallback for the affected tipset.
- **At and after nv28:**
  - Reservations are part of consensus:
    - All mainnet nodes must run an engine version that implements the reservation FFI (`BeginReservations` / `EndReservations`).
    - Tipsets that fail reservation checks are invalid under the new rules.
  - Operators should:
    - Upgrade Lotus and the underlying engine (filecoin‑ffi / ref‑fvm) ahead of the nv28 upgrade epoch (`UpgradeXxHeight` per network).
    - Monitor `reservations` logs and reservation metrics during and after the upgrade window.
  - Receipts, `GasOutputs`, and on‑chain accounting remain identical to legacy behaviour; the reservation ledger is internal to the engine and does not change actor code or on‑chain state.
