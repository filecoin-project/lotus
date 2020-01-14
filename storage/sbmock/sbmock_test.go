package sbmock

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-sectorbuilder"
)

func TestOpFinish(t *testing.T) {
	sb := NewMockSectorBuilder(1, 1024)

	sid, pieces, err := sb.StageFakeData()
	if err != nil {
		t.Fatal(err)
	}

	ctx, done := AddOpFinish(context.TODO())

	finished := make(chan struct{})
	go func() {
		_, err := sb.SealPreCommit(ctx, sid, sectorbuilder.SealTicket{}, pieces)
		if err != nil {
			t.Error(err)
			return
		}

		close(finished)
	}()

	select {
	case <-finished:
		t.Fatal("should not finish until we tell it to")
	case <-time.After(time.Second / 2):
	}

	done()

	select {
	case <-finished:
	case <-time.After(time.Second / 2):
		t.Fatal("should finish after we tell it to")
	}
}
