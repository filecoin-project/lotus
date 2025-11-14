package consensus

import (
	"context"
	"errors"

	"go.opencensus.io/stats"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/metrics"
)

var rlog = logging.Logger("reservations")

// ReservationsEnabled returns true when tipset reservations should be attempted.
// Before the reservations activation network version, this helper consults the
// MultiStageReservations feature flag. At or after the activation network
// version, reservations are always enabled and become consensus-critical.
func ReservationsEnabled(nv network.Version) bool {
	// After activation, reservations are required regardless of feature flags.
	if nv >= vm.ReservationsActivationNetworkVersion() {
		return true
	}

	// Pre-activation: best-effort mode controlled by the feature flag.
	return Feature.MultiStageReservations
}

// buildReservationPlan aggregates per-sender gas reservations across the full
// tipset. The amount reserved for each message is gas_limit * gas_fee_cap, and
// messages are deduplicated by CID across all blocks in canonical order,
// matching processedMsgs handling in ApplyBlocks.
func buildReservationPlan(bms []FilecoinBlockMessages) map[address.Address]abi.TokenAmount {
	plan := make(map[address.Address]abi.TokenAmount)
	seen := make(map[cid.Cid]struct{})

	for _, b := range bms {
		// canonical order is preserved in the combined slices append below
		for _, cm := range append(b.BlsMessages, b.SecpkMessages...) {
			m := cm.VMMessage()
			mcid := m.Cid()
			if _, ok := seen[mcid]; ok {
				continue
			}
			seen[mcid] = struct{}{}
			// Only explicit messages are included in blocks; implicit messages are applied separately.
			cost := types.BigMul(types.NewInt(uint64(m.GasLimit)), m.GasFeeCap)
			if prev, ok := plan[m.From]; ok {
				plan[m.From] = types.BigAdd(prev, cost)
			} else {
				plan[m.From] = cost
			}
		}
	}
	return plan
}

// startReservations is a helper that starts a reservation session on the VM if enabled.
// If the computed plan is empty (no explicit messages), Begin is skipped entirely.
func startReservations(ctx context.Context, vmi vm.Interface, bms []FilecoinBlockMessages, nv network.Version) error {
	if !ReservationsEnabled(nv) {
		return nil
	}

	plan := buildReservationPlan(bms)
	if len(plan) == 0 {
		rlog.Debugw("skipping tipset reservations for empty plan")
		return nil
	}

	total := abi.NewTokenAmount(0)
	for _, amt := range plan {
		total = big.Add(total, amt)
	}

	stats.Record(ctx,
		metrics.ReservationPlanSenders.M(int64(len(plan))),
		metrics.ReservationPlanTotal.M(total.Int64()),
	)

	rlog.Infow("starting tipset reservations", "senders", len(plan), "total", total)
	if err := vmi.StartTipsetReservations(ctx, plan); err != nil {
		return handleReservationError("begin", err, nv)
	}
	return nil
}

// endReservations ends the active reservation session if enabled.
func endReservations(ctx context.Context, vmi vm.Interface, nv network.Version) error {
	if !ReservationsEnabled(nv) {
		return nil
	}
	if err := vmi.EndTipsetReservations(ctx); err != nil {
		return handleReservationError("end", err, nv)
	}
	return nil
}

// handleReservationError interprets Begin/End reservation errors according to
// network version and feature flags, deciding whether to fall back to legacy
// mode (pre-activation, non-strict) or surface the error.
func handleReservationError(stage string, err error, nv network.Version) error {
	if err == nil {
		return nil
	}

	// Post-activation: reservations are consensus-critical; all Begin/End
	// errors surface to the caller. ErrReservationsNotImplemented becomes a
	// node error (engine too old) under active rules.
	if nv >= vm.ReservationsActivationNetworkVersion() {
		return err
	}

	// Pre-activation: ErrNotImplemented is always treated as a benign signal
	// that the engine does not support reservations yet; fall back to legacy
	// mode regardless of strictness.
	if errors.Is(err, vm.ErrReservationsNotImplemented) {
		rlog.Debugw("tipset reservations not implemented; continuing in legacy mode",
			"stage", stage, "error", err)
		return nil
	}

	// Node-error classes: always surface as errors, even pre-activation.
	if errors.Is(err, vm.ErrReservationsSessionOpen) ||
		errors.Is(err, vm.ErrReservationsSessionClosed) ||
		errors.Is(err, vm.ErrReservationsNonZeroRemainder) ||
		errors.Is(err, vm.ErrReservationsOverflow) ||
		errors.Is(err, vm.ErrReservationsInvariantViolation) {
		return err
	}

	// Reservation failures toggled by strict mode. When strict is disabled,
	// treat these as best-effort pre-activation and fall back to legacy mode.
	switch {
	case errors.Is(err, vm.ErrReservationsInsufficientFunds), errors.Is(err, vm.ErrReservationsPlanTooLarge):
		if Feature.MultiStageReservationsStrict {
			return err
		}
		rlog.Debugw("tipset reservations failed pre-activation; continuing in legacy mode (non-strict)",
			"stage", stage, "error", err)
		return nil
	default:
		// Unknown errors pre-activation are treated as node errors.
		return err
	}
}
