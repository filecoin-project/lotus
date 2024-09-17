package types

type IndexValidation struct {
	TipsetKey string
	Height    uint64

	NonRevertedMessageCount uint64
	NonRevertedEventsCount  uint64
	Backfilled              bool
}
