package sectorstorage

import (
	"container/heap"
	"testing"

	"github.com/filecoin-project/sector-storage/sealtasks"
)

func TestRequestQueue(t *testing.T) {
	rq := &requestQueue{}

	heap.Push(rq, &workerRequest{taskType: sealtasks.TTAddPiece})
	heap.Push(rq, &workerRequest{taskType: sealtasks.TTPreCommit1})
	heap.Push(rq, &workerRequest{taskType: sealtasks.TTPreCommit2})
	heap.Push(rq, &workerRequest{taskType: sealtasks.TTPreCommit1})
	heap.Push(rq, &workerRequest{taskType: sealtasks.TTAddPiece})

	pt := heap.Pop(rq).(*workerRequest)

	if pt.taskType != sealtasks.TTPreCommit2 {
		t.Error("expected precommit2, got", pt.taskType)
	}

	pt = heap.Pop(rq).(*workerRequest)

	if pt.taskType != sealtasks.TTPreCommit1 {
		t.Error("expected precommit1, got", pt.taskType)
	}
}
