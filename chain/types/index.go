package types

type IndexValidation struct {
	TipsetKey string
	Height    uint64

	TotalMessages  uint64
	TotalEvents    uint64
	EventsReverted bool
	Backfilled     bool
}
