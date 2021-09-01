package fsjournal

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("fsjournal")

const (
	RFC3339nocolon  = "2006-01-02T150405Z0700"
	currentFilename = "lotus-journal.ndjson"
)

// fsJournal is a basic journal backed by files on a filesystem.
type fsJournal struct {
	journal.EventTypeRegistry

	dir     string
	maxSize int64

	fi    *os.File
	fSize int64

	maxBackups int64
	backups    []string
	cur        int64

	incoming chan *journal.Event

	closing chan struct{}
	closed  chan struct{}
}

// OpenFSJournal constructs a rolling filesystem journal, with a default
// per-file size limit of 1GiB.
func OpenFSJournal(lr repo.LockedRepo, disabled journal.DisabledEvents, maxSize int64, maxBackups int64) (journal.Journal, error) {
	dir := filepath.Join(lr.Path(), "journal")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to mk directory %s for file journal: %w", dir, err)
	}
	var backuplen int64
	if maxBackups > 0 {
		backuplen = maxBackups
	}
	f := &fsJournal{
		EventTypeRegistry: journal.NewEventTypeRegistry(disabled),
		dir:               dir,
		maxSize:           maxSize,
		maxBackups:        maxBackups,
		backups:           make([]string, backuplen),
		cur:               0,
		incoming:          make(chan *journal.Event, 32),
		closing:           make(chan struct{}),
		closed:            make(chan struct{}),
	}

	// load existing journal file names, if there are any.
	if files, err := os.ReadDir(f.dir); err != nil {
		finfos := make([]fs.FileInfo, 0)
		for _, de := range files {
			if de.IsDir() || de.Name() == currentFilename {
				continue
			}
			if fi, err := de.Info(); err != nil {
				finfos = append(finfos, fi)
			}
		}
		sort.Slice(finfos, func(i, j int) bool {
			return finfos[i].ModTime().After(finfos[j].ModTime())
		})
		for i, fi := range finfos {
			fullName := filepath.Join(f.dir, fi.Name())
			if i < int(backuplen) {
				f.backups[i] = fullName
			} else {
				os.Remove(fullName)
			}
		}
	}

	if maxSize != 0 {
		if err := f.rollJournalFile(); err != nil {
			return nil, err
		}
	}

	go f.runLoop()

	return f, nil
}

func (f *fsJournal) RecordEvent(evtType journal.EventType, supplier func() interface{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("recovered from panic while recording journal event; type=%s, err=%v", evtType, r)
		}
	}()

	if !evtType.Enabled() {
		return
	}

	je := &journal.Event{
		EventType: evtType,
		Timestamp: build.Clock.Now(),
		Data:      supplier(),
	}
	select {
	case f.incoming <- je:
	case <-f.closing:
		log.Warnw("journal closed but tried to log event", "event", je)
	}
}

func (f *fsJournal) Close() error {
	close(f.closing)
	<-f.closed
	return nil
}

func (f *fsJournal) putEvent(evt *journal.Event) error {
	b, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	n, err := f.fi.Write(append(b, '\n'))
	if err != nil {
		return err
	}

	f.fSize += int64(n)

	if f.maxSize > 0 && f.fSize >= f.maxSize {
		fmt.Println("ok, so its here?")
		f.rollJournalFile()
	}

	return nil
}

func (f *fsJournal) rollJournalFile() error {
	if f.fi != nil {
		_ = f.fi.Close()
	}
	current := filepath.Join(f.dir, currentFilename)
	rolled := filepath.Join(f.dir, fmt.Sprintf(
		"lotus-journal-%s.ndjson",
		build.Clock.Now().Format(RFC3339nocolon),
	))

	// journal rotation
	if f.maxBackups != 0 {
		// check if journal file exists
		if fi, err := os.Stat(current); err == nil && !fi.IsDir() {
			err := os.Rename(current, rolled)
			if err != nil {
				return xerrors.Errorf("failed to roll journal file: %w", err)
			}
		}

		// when maxBackups is a positive number, delete old journals
		if f.maxBackups > 0 {
			if r := f.backups[f.cur]; r != "" {
				os.Remove(r)
			}
			f.backups[f.cur] = rolled
			f.cur = (f.cur + 1) % f.maxBackups
		}
	}

	nfi, err := os.Create(current)
	if err != nil {
		return xerrors.Errorf("failed to create journal file: %w", err)
	}
	f.fi = nfi
	f.fSize = 0
	return nil
}

func (f *fsJournal) runLoop() {
	defer func() {
		log.Info("closing journal")
		close(f.closed)
	}()

	for {
		select {
		case je := <-f.incoming:
			if f.maxSize != 0 {
				if err := f.putEvent(je); err != nil {
					log.Errorw("failed to write out journal event", "event", je, "err", err)
				}
			}
		case <-f.closing:
			_ = f.fi.Close()
			return
		}
	}
}
