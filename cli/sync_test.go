package cli

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/stretchr/testify/assert"
)

func TestSyncStatus(t *testing.T) {
	app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("sync", SyncStatusCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts1 := mock.TipSet(mock.MkBlock(nil, 0, 0))
	ts2 := mock.TipSet(mock.MkBlock(ts1, 0, 0))

	start := time.Now()
	end := start.Add(time.Minute)

	state := &api.SyncState{
		ActiveSyncs: []api.ActiveSync{{
			WorkerID: 1,
			Base:     ts1,
			Target:   ts2,
			Stage:    api.StageMessages,
			Height:   abi.ChainEpoch(0),
			Start:    start,
			End:      end,
			Message:  "whatever",
		}},
		VMApplied: 0,
	}

	mockApi.EXPECT().SyncState(ctx).Return(state, nil)

	err := app.Run([]string{"sync", "status"})
	assert.NoError(t, err)

	out := buf.String()

	// output is plaintext, had to do string matching
	assert.Contains(t, out, fmt.Sprintf("Base:\t[%s]", ts1.Blocks()[0].Cid().String()))
	assert.Contains(t, out, fmt.Sprintf("Target:\t[%s]", ts2.Blocks()[0].Cid().String()))
	assert.Contains(t, out, "Height diff:\t1")
	assert.Contains(t, out, "Stage: message sync")
	assert.Contains(t, out, "Height: 0")
	assert.Contains(t, out, "Elapsed: 1m0s")
}

func TestSyncMarkBad(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("sync", SyncMarkBadCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blk := mock.MkBlock(nil, 0, 0)

	mockApi.EXPECT().SyncMarkBad(ctx, blk.Cid()).Return(nil)

	err := app.Run([]string{"sync", "mark-bad", blk.Cid().String()})
	assert.NoError(t, err)
}
