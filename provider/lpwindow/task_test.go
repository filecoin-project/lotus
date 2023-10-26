package lpwindow

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/node/config"
)

// test to create WDPostTask, invoke AddTask and check if the task is added to the DB
func TestAddTask(t *testing.T) {
	db, err := harmonydb.NewFromConfig(config.HarmonyDB{
		Hosts:    []string{"localhost"},
		Port:     "5433",
		Username: "yugabyte",
		Password: "yugabyte",
		Database: "yugabyte",
	})
	require.NoError(t, err)
	wdPostTask := NewWdPostTask(db, nil, 0)
	taskEngine, err := harmonytask.New(db, []harmonytask.TaskInterface{wdPostTask}, "localhost:12300")
	_ = taskEngine
	ts := types.TipSet{}
	deadline := dline.Info{}

	require.NoError(t, err)
}
