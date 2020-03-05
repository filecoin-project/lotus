package sealmgr

type TaskType string

const (
	TTAddPiece   TaskType = "seal/v0/addpiece"
	TTPreCommit1 TaskType = "seal/v0/precommit/1"
	TTPreCommit2 TaskType = "seal/v0/precommit/2" // Commit1 is called here too
	TTCommit2    TaskType = "seal/v0/commit/2"
)
