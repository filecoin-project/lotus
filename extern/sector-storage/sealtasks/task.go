package sealtasks

type TaskType string

const (
	TTAddPiece   TaskType = "seal/v0/addpiece"
	TTPreCommit1 TaskType = "seal/v0/precommit/1"
	TTPreCommit2 TaskType = "seal/v0/precommit/2"
	TTCommit1    TaskType = "seal/v0/commit/1" // NOTE: We use this to transfer the sector into miner-local storage for now; Don't use on workers!
	TTCommit2    TaskType = "seal/v0/commit/2"

	TTFinalize TaskType = "seal/v0/finalize"

	TTFetch  TaskType = "seal/v0/fetch"
	TTUnseal TaskType = "seal/v0/unseal"

	TTReplicaUpdate         TaskType = "seal/v0/replicaupdate"
	TTProveReplicaUpdate1   TaskType = "seal/v0/provereplicaupdate/1"
	TTProveReplicaUpdate2   TaskType = "seal/v0/provereplicaupdate/2"
	TTRegenSectorKey        TaskType = "seal/v0/regensectorkey"
	TTFinalizeReplicaUpdate TaskType = "seal/v0/finalize/replicaupdate"
)

var order = map[TaskType]int{
	TTRegenSectorKey:      10, // least priority
	TTAddPiece:            9,
	TTReplicaUpdate:       8,
	TTProveReplicaUpdate2: 7,
	TTProveReplicaUpdate1: 6,
	TTPreCommit1:          5,
	TTPreCommit2:          4,
	TTCommit2:             3,
	TTCommit1:             2,
	TTUnseal:              1,
	TTFetch:               -1,
	TTFinalize:            -2, // most priority
}

var shortNames = map[TaskType]string{
	TTAddPiece: "AP",

	TTPreCommit1: "PC1",
	TTPreCommit2: "PC2",
	TTCommit1:    "C1",
	TTCommit2:    "C2",

	TTFinalize: "FIN",

	TTFetch:  "GET",
	TTUnseal: "UNS",

	TTReplicaUpdate:         "RU",
	TTProveReplicaUpdate1:   "PR1",
	TTProveReplicaUpdate2:   "PR2",
	TTRegenSectorKey:        "GSK",
	TTFinalizeReplicaUpdate: "FRU",
}

func (a TaskType) MuchLess(b TaskType) (bool, bool) {
	oa, ob := order[a], order[b]
	oneNegative := oa^ob < 0
	return oneNegative, oa < ob
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
