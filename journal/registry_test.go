package journal

import (
	"container/list"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/node/repo"
	"github.com/stretchr/testify/require"
)

func TestDisabledEvents(t *testing.T) {
	req := require.New(t)

	test := func(dis DisabledEvents) func(*testing.T) {
		return func(t *testing.T) {
			registry := NewEventTypeRegistry(dis)

			reg1 := registry.RegisterEventType("system1", "disabled1")
			reg2 := registry.RegisterEventType("system1", "disabled2")

			req.False(reg1.Enabled())
			req.False(reg2.Enabled())
			req.True(reg1.safe)
			req.True(reg2.safe)

			reg3 := registry.RegisterEventType("system3", "enabled3")
			req.True(reg3.Enabled())
			req.True(reg3.safe)
		}
	}

	t.Run("direct", test(DisabledEvents{
		EventType{System: "system1", Event: "disabled1"},
		EventType{System: "system1", Event: "disabled2"},
	}))

	dis, err := ParseDisabledEvents("system1:disabled1,system1:disabled2")
	req.NoError(err)

	t.Run("parsed", test(dis))

	dis, err = ParseDisabledEvents("  system1:disabled1 , system1:disabled2  ")
	req.NoError(err)

	t.Run("parsed_spaces", test(dis))
}

func TestParseDisableEvents(t *testing.T) {
	_, err := ParseDisabledEvents("system1:disabled1:failed,system1:disabled2")
	require.Error(t, err)
}

func newtestfsjournal(t *testing.T, lr repo.LockedRepo, sizeLimit int64, keep int) *fsJournal {
	req := require.New(t)
	dir := filepath.Join(lr.Path(), "journal")
	req.NoErrorf(os.MkdirAll(dir, 0755), "could not make journal directory")

	j := &fsJournal{
		dir:       dir,
		sizeLimit: sizeLimit,
		keep:      keep,
		old:       list.New(),
	}
	j.rollJournalFile()
	return j
}

func TestRollingRemovesOldFiles(t *testing.T) {
	r := repo.NewMemory(nil)
	req := require.New(t)
	lr, err := r.Lock(repo.FullNode)
	req.NoError(err)
	j := newtestfsjournal(t, lr, 0, 3)
	dir := filepath.Join(lr.Path(), "journal")
	for i := 0; i <= j.keep; i++ {
		time.Sleep(time.Second)
		files, _ := os.ReadDir(dir)
		req.Lenf(files, i+1, "add one file for every roll before max keep")
		j.rollJournalFile()
	}
	// on the last iteration, one of the files should have been pruned,
	// so we should still have only the maximum kept files.
	time.Sleep(time.Second)
	files, _ := os.ReadDir(dir)
	req.Lenf(files, j.keep+1, "files are not being pruned from the journal directory")
}
