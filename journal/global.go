package journal

var (
	// J is a globally accessible Journal. It starts being NilJournal, and early
	// during the Lotus initialization routine, it is reset to whichever Journal
	// is configured (by default, the filesystem journal). Components can safely
	// record in the journal by calling: journal.J.RecordEvent(...).
	J Journal = NilJournal() // nolint
)
