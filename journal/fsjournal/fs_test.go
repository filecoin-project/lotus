package fsjournal

import (
	"container/list"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/node/repo"
	"github.com/stretchr/testify/require"
)

func TestRollingRemovesOldFiles(t *testing.T) {
	r := repo.NewMemory(nil)
	req := require.New(t)
	lr, err := r.Lock(repo.FullNode)
	req.NoError(err)
	j := newtestfsjournal(t, lr, 0, 3)
	dir := filepath.Join(lr.Path(), "journal")
	for i := 0; i <= j.keep; i++ {
		files, _ := os.ReadDir(dir)
		req.Lenf(files, i, "add one file for every roll before max keep")
		j.rollJournalFile()
		// there is a lag between when the file is Create()'d and when it appears.
		// it's actually a pretty long delay.
		time.Sleep(time.Second)
	}
	// on the last iteration, one of the files should have been pruned,
	// so we should still have only the maximum kept files.
	files, _ := os.ReadDir(dir)
	req.Lenf(files, j.keep, "files are not being pruned from the journal directory")
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
	return j
}
