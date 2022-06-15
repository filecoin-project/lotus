package sealer

import (
	"fmt"
	"testing"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
)

func TestRequestQueue(t *testing.T) {
	rq := &RequestQueue{}

	rq.Push(&WorkerRequest{TaskType: sealtasks.TTAddPiece})
	rq.Push(&WorkerRequest{TaskType: sealtasks.TTPreCommit1})
	rq.Push(&WorkerRequest{TaskType: sealtasks.TTPreCommit2})
	rq.Push(&WorkerRequest{TaskType: sealtasks.TTPreCommit1})
	rq.Push(&WorkerRequest{TaskType: sealtasks.TTAddPiece})

	dump := func(s string) {
		fmt.Println("---")
		fmt.Println(s)

		for sqi := 0; sqi < rq.Len(); sqi++ {
			task := (*rq)[sqi]
			fmt.Println(sqi, task.TaskType)
		}
	}

	dump("start")

	pt := rq.Remove(0)

	dump("pop 1")

	if pt.TaskType != sealtasks.TTPreCommit2 {
		t.Error("expected precommit2, got", pt.TaskType)
	}

	pt = rq.Remove(0)

	dump("pop 2")

	if pt.TaskType != sealtasks.TTPreCommit1 {
		t.Error("expected precommit1, got", pt.TaskType)
	}

	pt = rq.Remove(1)

	dump("pop 3")

	if pt.TaskType != sealtasks.TTAddPiece {
		t.Error("expected addpiece, got", pt.TaskType)
	}

	pt = rq.Remove(0)

	dump("pop 4")

	if pt.TaskType != sealtasks.TTPreCommit1 {
		t.Error("expected precommit1, got", pt.TaskType)
	}
}
