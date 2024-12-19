package cli

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
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

func TestSyncUnmarkBad(t *testing.T) {
	t.Run("one-block", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("sync", SyncUnmarkBadCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		blk := mock.MkBlock(nil, 0, 0)

		mockApi.EXPECT().SyncUnmarkBad(ctx, blk.Cid()).Return(nil)

		err := app.Run([]string{"sync", "unmark-bad", blk.Cid().String()})
		assert.NoError(t, err)
	})

	t.Run("all", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("sync", SyncUnmarkBadCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockApi.EXPECT().SyncUnmarkAllBad(ctx).Return(nil)

		err := app.Run([]string{"sync", "unmark-bad", "-all"})
		assert.NoError(t, err)
	})
}

func TestSyncCheckBad(t *testing.T) {
	t.Run("not-bad", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("sync", SyncCheckBadCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		blk := mock.MkBlock(nil, 0, 0)

		mockApi.EXPECT().SyncCheckBad(ctx, blk.Cid()).Return("", nil)

		err := app.Run([]string{"sync", "check-bad", blk.Cid().String()})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), "block was not marked as bad")
	})

	t.Run("bad", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("sync", SyncCheckBadCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		blk := mock.MkBlock(nil, 0, 0)
		reason := "whatever"

		mockApi.EXPECT().SyncCheckBad(ctx, blk.Cid()).Return(reason, nil)

		err := app.Run([]string{"sync", "check-bad", blk.Cid().String()})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), reason)
	})
}

func TestSyncCheckpoint(t *testing.T) {
	t.Run("tipset", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("sync", SyncCheckpointCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		blk := mock.MkBlock(nil, 0, 0)
		ts := mock.TipSet(blk)

		gomock.InOrder(
			mockApi.EXPECT().ChainGetBlock(ctx, blk.Cid()).Return(blk, nil),
			mockApi.EXPECT().SyncCheckpoint(ctx, ts.Key()).Return(nil),
		)

		err := app.Run([]string{"sync", "checkpoint", blk.Cid().String()})
		assert.NoError(t, err)
	})

	t.Run("epoch", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("sync", SyncCheckpointCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		epoch := abi.ChainEpoch(0)
		blk := mock.MkBlock(nil, 0, 0)
		ts := mock.TipSet(blk)

		gomock.InOrder(
			mockApi.EXPECT().ChainGetTipSetByHeight(ctx, epoch, types.EmptyTSK).Return(ts, nil),
			mockApi.EXPECT().SyncCheckpoint(ctx, ts.Key()).Return(nil),
		)

		err := app.Run([]string{"sync", "checkpoint", fmt.Sprintf("-epoch=%d", epoch)})
		assert.NoError(t, err)
	})
}
