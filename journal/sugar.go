package journal

// MaybeAddEntry is a convenience function that evaluates if the EventType is
// enabled, and if so, it calls the supplier to create the entry and
// subsequently journal.AddEntry on the provided journal to record it.
func MaybeAddEntry(journal Journal, evtType EventType, supplier func() interface{}) {
	if !evtType.Enabled() {
		return
	}
	journal.AddEntry(evtType, supplier())
}
