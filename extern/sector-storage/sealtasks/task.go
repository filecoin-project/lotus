package sealtasks

type TaskType string

const (
	TTAddPiece   TaskType = "seal/v0/addpiece"
	TTPreCommit1 TaskType = "seal/v0/precommit/1"
	TTPreCommit2 TaskType = "seal/v0/precommit/2"
	TTCommit1    TaskType = "seal/v0/commit/1"
	TTCommit2    TaskType = "seal/v0/commit/2"

	TTFinalize TaskType = "seal/v0/finalize"

	TTFetch  TaskType = "seal/v0/fetch"
	TTUnseal TaskType = "seal/v0/unseal"

	TTReplicaUpdate         TaskType = "seal/v0/replicaupdate"
	TTProveReplicaUpdate1   TaskType = "seal/v0/provereplicaupdate/1"
	TTProveReplicaUpdate2   TaskType = "seal/v0/provereplicaupdate/2"
	TTRegenSectorKey        TaskType = "seal/v0/regensectorkey"
	TTFinalizeReplicaUpdate TaskType = "seal/v0/finalize/replicaupdate"

	TTGenerateWindowPoSt  TaskType = "post/v0/windowproof"
	TTGenerateWinningPoSt TaskType = "post/v0/winningproof"
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

	TTFetch:    -1,
	TTFinalize: -2,

	TTGenerateWindowPoSt:  -3,
	TTGenerateWinningPoSt: -4, // most priority
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

	TTGenerateWindowPoSt:  "WDP",
	TTGenerateWinningPoSt: "WNP",
}

const (
	WorkerSealing     = "Sealing"
	WorkerWinningPoSt = "WinPost"
	WorkerWindowPoSt  = "WdPoSt"
)

func (a TaskType) WorkerType() string {
	switch a {
	case TTGenerateWinningPoSt:
		return WorkerWinningPoSt
	case TTGenerateWindowPoSt:
		return WorkerWindowPoSt
	default:
		return WorkerSealing
	}
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
