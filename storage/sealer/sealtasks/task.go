package sealtasks

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
)

type TaskType string

const (
	TTDataCid    TaskType = "seal/v0/datacid"
	TTAddPiece   TaskType = "seal/v0/addpiece"
	TTPreCommit1 TaskType = "seal/v0/precommit/1"
	TTPreCommit2 TaskType = "seal/v0/precommit/2"
	TTCommit1    TaskType = "seal/v0/commit/1"
	TTCommit2    TaskType = "seal/v0/commit/2"

	TTFinalize         TaskType = "seal/v0/finalize"
	TTFinalizeUnsealed TaskType = "seal/v0/finalizeunsealed"

	TTFetch  TaskType = "seal/v0/fetch"
	TTUnseal TaskType = "seal/v0/unseal"

	TTReplicaUpdate         TaskType = "seal/v0/replicaupdate"
	TTProveReplicaUpdate1   TaskType = "seal/v0/provereplicaupdate/1"
	TTProveReplicaUpdate2   TaskType = "seal/v0/provereplicaupdate/2"
	TTRegenSectorKey        TaskType = "seal/v0/regensectorkey"
	TTFinalizeReplicaUpdate TaskType = "seal/v0/finalize/replicaupdate"

	TTDownloadSector TaskType = "seal/v0/download/sector"

	TTGenerateWindowPoSt  TaskType = "post/v0/windowproof"
	TTGenerateWinningPoSt TaskType = "post/v0/winningproof"

	TTNoop TaskType = ""
)

var order = map[TaskType]int{
	TTRegenSectorKey:      11, // least priority
	TTDataCid:             10,
	TTAddPiece:            9,
	TTReplicaUpdate:       8,
	TTProveReplicaUpdate2: 7,
	TTProveReplicaUpdate1: 6,
	TTPreCommit1:          5,
	TTPreCommit2:          4,
	TTCommit2:             3,
	TTCommit1:             2,
	TTUnseal:              1,

	TTFetch:            -1,
	TTDownloadSector:   -2,
	TTFinalize:         -3,
	TTFinalizeUnsealed: -4,

	TTGenerateWindowPoSt:  -5,
	TTGenerateWinningPoSt: -6, // most priority
}

var shortNames = map[TaskType]string{
	TTDataCid:  "DC",
	TTAddPiece: "AP",

	TTPreCommit1: "PC1",
	TTPreCommit2: "PC2",
	TTCommit1:    "C1",
	TTCommit2:    "C2",

	TTFinalize:         "FIN",
	TTFinalizeUnsealed: "FUS",

	TTFetch:  "GET",
	TTUnseal: "UNS",

	TTReplicaUpdate:         "RU",
	TTProveReplicaUpdate1:   "PR1",
	TTProveReplicaUpdate2:   "PR2",
	TTRegenSectorKey:        "GSK",
	TTFinalizeReplicaUpdate: "FRU",

	TTDownloadSector: "DL",

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

// SectorSized returns true if the task operates on a specific sector size
func (a TaskType) SectorSized() bool {
	return a != TTDataCid
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

type SealTaskType struct {
	TaskType
	abi.RegisteredSealProof
}

func (a TaskType) SealTask(spt abi.RegisteredSealProof) SealTaskType {
	return SealTaskType{
		TaskType:            a,
		RegisteredSealProof: spt,
	}
}

func SttFromString(s string) (SealTaskType, error) {
	var res SealTaskType

	sub := strings.SplitN(s, ":", 2)
	if len(sub) != 2 {
		return res, xerrors.Errorf("seal task type string invalid")
	}

	res.TaskType = TaskType(sub[1])
	spt, err := strconv.ParseInt(sub[0], 10, 64)
	if err != nil {
		return SealTaskType{}, err
	}
	res.RegisteredSealProof = abi.RegisteredSealProof(spt)

	return res, nil
}

func (a SealTaskType) String() string {
	return fmt.Sprintf("%d:%s", a.RegisteredSealProof, a.TaskType)
}
