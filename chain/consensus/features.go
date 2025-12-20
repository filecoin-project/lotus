package consensus

import "os"

// ReservationFeatureFlags holds feature toggles for tipset gas reservations.
//
// These flags are evaluated by consensus and the message pool when deciding
// whether to attempt tipset‑scope reservations pre‑activation, and how to
// interpret Begin/End reservation errors.
type ReservationFeatureFlags struct {
	// MultiStageReservations enables tipset‑scope gas reservations
	// pre‑activation. When false, ReservationsEnabled returns false for
	// network versions before ReservationsActivationNetworkVersion and Lotus
	// operates in legacy mode (no Begin/End calls).
	//
	// At or after activation, reservations are always enabled regardless of
	// this flag.
	MultiStageReservations bool

	// MultiStageReservationsStrict controls how pre‑activation reservation
	// failures are handled when MultiStageReservations is true:
	//
	//   - When false (non‑strict), non‑NotImplemented Begin/End errors such
	//     as ErrReservationsInsufficientFunds and ErrReservationsPlanTooLarge
	//     are treated as best‑effort: Lotus logs and falls back to legacy
	//     mode for that tipset.
	//   - When true (strict), those reservation failures invalidate the
	//     tipset pre‑activation. Node‑error classes (e.g. overflow or
	//     invariant violations) always surface as errors regardless of this
	//     flag.
	MultiStageReservationsStrict bool
}

// Feature exposes the current reservation feature flags.
//
// Defaults:
//   - MultiStageReservations: enabled when LOTUS_ENABLE_TIPSET_RESERVATIONS=1.
//   - MultiStageReservationsStrict: enabled when
//     LOTUS_ENABLE_TIPSET_RESERVATIONS_STRICT=1.
//
// These defaults preserve the existing environment‑based gating while making
// the flags explicit and testable.
var Feature = ReservationFeatureFlags{
	MultiStageReservations:       os.Getenv("LOTUS_ENABLE_TIPSET_RESERVATIONS") == "1",
	MultiStageReservationsStrict: os.Getenv("LOTUS_ENABLE_TIPSET_RESERVATIONS_STRICT") == "1",
}

// SetFeatures overrides the global reservation feature flags. This is intended
// for wiring from higher‑level configuration and for tests; callers should
// treat it as process‑wide and set it once during initialization.
func SetFeatures(flags ReservationFeatureFlags) {
	Feature = flags
}
