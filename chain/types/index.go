package types

// IndexValidation contains detailed information about the validation status of a specific chain epoch.
type IndexValidation struct {
	// TipSetKey is the key of the canonical tipset for this epoch.
	TipSetKey TipSetKey
	// Height is the epoch height at which the validation is performed.
	Height uint64
	// IndexedMessagesCount indicates the number of indexed messages for the canonical tipset at this epoch.
	IndexedMessagesCount uint64
	// IndexedEventsCount signifies the number of indexed events for the canonical tipset at this epoch.
	IndexedEventsCount uint64
	// Backfilled denotes whether missing data was successfully backfilled into the index during validation.
	Backfilled bool
	// IsNullRound indicates if the epoch corresponds to a null round and therefore does not have any indexed messages or events.
	IsNullRound bool
}
