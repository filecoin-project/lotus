package wdpost

import (
	"context"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/stretchr/testify/require"
	"testing"
)

// test to create WDPostTask, invoke AddTask and check if the task is added to the DB
func TestAddTask(t *testing.T) {
	db, err := harmonydb.New(nil, "yugabyte", "yugabyte", "yugabyte", "5433", "localhost", nil)
	require.NoError(t, err)
	wdPostTask := NewWdPostTask(db)
	taskEngine, err := harmonytask.New(db, []harmonytask.TaskInterface{wdPostTask}, "localhost:12300")
	ts := types.TipSet{}
	deadline := dline.Info{}
	err := wdPostTask.AddTask(context.Background(), &ts, &deadline)

	require.NoError(t, err)
}
