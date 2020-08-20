package sealtasks

type TaskType string

const (
	TTAddPiece   TaskType = "seal/v0/addpiece"
	TTPreCommit1 TaskType = "seal/v0/precommit/1"
	TTPreCommit2 TaskType = "seal/v0/precommit/2"
	TTCommit1    TaskType = "seal/v0/commit/1" // NOTE: We use this to transfer the sector into miner-local storage for now; Don't use on workers!
	TTCommit2    TaskType = "seal/v0/commit/2"

	TTFinalize TaskType = "seal/v0/finalize"

	TTFetch        TaskType = "seal/v0/fetch"
	TTUnseal       TaskType = "seal/v0/unseal"
	TTReadUnsealed TaskType = "seal/v0/unsealread"
)

var order = map[TaskType]int{
	TTAddPiece:     7,
	TTPreCommit1:   6,
	TTPreCommit2:   5,
	TTCommit2:      4,
	TTCommit1:      3,
	TTFetch:        2,
	TTFinalize:     1,
	TTUnseal:       0,
	TTReadUnsealed: 0,
}

var shortNames = map[TaskType]string{
	TTAddPiece: "AP ",

	TTPreCommit1: "PC1",
	TTPreCommit2: "PC2",
	TTCommit1:    "C1 ",
	TTCommit2:    "C2 ",

	TTFinalize: "FIN",

	TTFetch:        "GET",
	TTUnseal:       "UNS",
	TTReadUnsealed: "RD ",
}

func (a TaskType) Less(b TaskType) bool {
	return order[a] < order[b]
}

func (a TaskType) Short() string {
	n, ok := shortNames[a]
	if !ok {
		return "UNK"
	}

	return n
}
