package fsjournal

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRollingRemovesOldFiles(t *testing.T) {
	req := require.New(t)
	var i int64
	j := newtestfsjournal(t, 1, 3)
	for i = 0; i <= j.keep; i++ {
		files, _ := os.ReadDir(j.dir)
		req.Lenf(files, int(i), "add one file for every roll before max keep")
		j.rollJournalFile()
		// there is a lag between when the file is Create()'d and when it appears.
		// it's actually a pretty long delay.
		time.Sleep(time.Second)
	}
	// on the last iteration, one of the files should have been pruned,
	// so we should still have only the maximum kept files.
	files, _ := os.ReadDir(j.dir)
	req.Lenf(files, int(j.keep), "files are not being pruned from the journal directory")
}

func newtestfsjournal(t *testing.T, sizeLimit int64, keep int64) *fsJournal {
	req := require.New(t)
	dir := t.TempDir()
	req.NoErrorf(os.MkdirAll(dir, 0755), "could not make journal directory")

	j := &fsJournal{
		dir:       dir,
		sizeLimit: sizeLimit,
		keep:      keep,
		old:       make([]string, keep),
	}
	return j
}
