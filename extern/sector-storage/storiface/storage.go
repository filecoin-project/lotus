package storiface

// PathType is the type of a path in the sector store.
type PathType string

const (
	// PathStorage means the path type that is used for long term storage of sealed sector files.
	PathStorage PathType = "storage"
	// PathSealing means the sealing sratch space that is used a staging area for sealing sector files.
	PathSealing PathType = "sealing"
)

// AcquireMode is the move/copy semantics that is used by workers when fetching sector files from another worker.
type AcquireMode string

const (
	// AcquireMove means move semantics that acquire a sector file from another location and them remove the original copy.
	AcquireMove AcquireMode = "move"
	// AcquireCopy means copy semantics that copy a sector file from another location without removing the original copy.
	AcquireCopy AcquireMode = "copy"
)
