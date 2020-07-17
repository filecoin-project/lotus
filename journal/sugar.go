package journal

// MaybeAddEntry is a convenience function that evaluates if the EventType is
// enabled, and if so, it calls the supplier to create the entry and
// subsequently journal.AddEntry on the provided journal to record it.
//
// This is safe to call with a nil Journal, either because the value is nil,
// or because a journal obtained through NilJournal() is in use.
func MaybeAddEntry(journal Journal, evtType EventType, supplier func() interface{}) {
	if journal == nil || journal == nilj {
		return
	}
	if !evtType.Enabled() {
		return
	}
	journal.AddEntry(evtType, supplier())
}
