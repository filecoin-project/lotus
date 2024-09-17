package types

type IndexValidation struct {
	TipSetKey TipSetKey
	Height    uint64

	NonRevertedMessageCount uint64
	NonRevertedEventsCount  uint64
	Backfilled              bool
}
