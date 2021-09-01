package fsjournal

import (
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/journal"
	"github.com/stretchr/testify/require"
)

func TestRollingRemovesOldFiles(t *testing.T) {
	req := require.New(t)
	j := newtestfsjournal(t, 1, 3)
	for i := 0; i <= int(j.maxBackups)+1; i++ {
		files, _ := os.ReadDir(j.dir)
		req.Lenf(files, i, "add one file for every roll before max keep")
		j.rollJournalFile()
		// there is a lag between when the file is Create()'d and when it appears.
		// it's actually a pretty long delay.
		time.Sleep(time.Second)
	}
	// on the last iteration, one of the files should have been pruned,
	// so we should still have only the maximum kept files.
	files, _ := os.ReadDir(j.dir)
	req.Lenf(files, int(j.maxBackups)+1, "files are not being pruned from the journal directory")
}

func TestZeroSize(t *testing.T) {
	req := require.New(t)
	j := newtestfsjournal(t, 0, 3)
	for i := 0; i <= int(j.maxBackups); i++ {
		files, _ := os.ReadDir(j.dir)
		req.Lenf(files, 0, "expected files not to be created")
		j.RecordEvent(journal.EventType{
			System: "tst",
			Event:  "tst",
		}, func() interface{} { return "data" })
		time.Sleep(time.Second)
	}
}

func TestZeroKeep(t *testing.T) {
	req := require.New(t)
	j := newtestfsjournal(t, 1, 0)
	j.rollJournalFile()
	time.Sleep(time.Second)
	for i := 0; i <= 3; i++ {
		files, _ := os.ReadDir(j.dir)
		req.Lenf(files, 1, "expected no files to be kept")
		expectation := "lotus-journal.ndjson"
		req.Equalf(expectation, files[0].Name(), "expected %s found %s", expectation, files[0])
		j.rollJournalFile()
		time.Sleep(time.Second)
	}
}

func TestNegativeKeep(t *testing.T) {
	req := require.New(t)
	j := newtestfsjournal(t, 1, -1)
	for i := 0; i <= int(j.maxBackups)+2; i++ {
		files, _ := os.ReadDir(j.dir)
		req.Lenf(files, i, "old journals should not be deleted with negative keep")
		j.rollJournalFile()
		time.Sleep(time.Second)
	}
}

func newtestfsjournal(t *testing.T, maxSize int64, maxBackups int64) *fsJournal {
	var backuplen int64
	if maxBackups > 0 {
		backuplen = maxBackups
	}
	j := &fsJournal{
		dir:        t.TempDir(),
		maxSize:    maxSize,
		maxBackups: maxBackups,
		backups:    make([]string, backuplen),
	}
	return j
}
