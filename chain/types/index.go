package types

type IndexValidation struct {
	TipSetKey TipSetKey
	Height    uint64

	IndexedMessagesCount uint64
	IndexedEventsCount   uint64
	IndexedTxHashCount   uint64
	Backfilled           bool
	IsNullRound          bool
}
