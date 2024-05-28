package fsjournal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("fsjournal")

const RFC3339nocolon = "2006-01-02T150405Z0700"

// fsJournal is a basic journal backed by files on a filesystem.
type fsJournal struct {
	journal.EventTypeRegistry

	dir       string
	sizeLimit int64

	fi    *os.File
	fSize int64

	incoming chan *journal.Event

	closing chan struct{}
	closed  chan struct{}
}

// OpenFSJournal constructs a rolling filesystem journal, with a default
// per-file size limit of 1GiB.
func OpenFSJournal(lr repo.LockedRepo, disabled journal.DisabledEvents) (journal.Journal, error) {
	return OpenFSJournalPath(lr.Path(), disabled)
}

func OpenFSJournalPath(path string, disabled journal.DisabledEvents) (journal.Journal, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, xerrors.Errorf("failed to expand repo path: %w", err)
	}

	dir := filepath.Join(path, "journal")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to mk directory %s for file journal: %w", dir, err)
	}

	f := &fsJournal{
		EventTypeRegistry: journal.NewEventTypeRegistry(disabled),
		dir:               dir,
		sizeLimit:         1 << 30,
		incoming:          make(chan *journal.Event, 32),
		closing:           make(chan struct{}),
		closed:            make(chan struct{}),
	}

	if err := f.rollJournalFile(); err != nil {
		return nil, err
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

	if f.fSize >= f.sizeLimit {
		_ = f.rollJournalFile()
	}

	return nil
}

func (f *fsJournal) rollJournalFile() error {
	if f.fi != nil {
		_ = f.fi.Close()
	}
	current := filepath.Join(f.dir, "lotus-journal.ndjson")
	rolled := filepath.Join(f.dir, fmt.Sprintf(
		"lotus-journal-%s.ndjson",
		build.Clock.Now().Format(RFC3339nocolon),
	))

	// check if journal file exists
	if fi, err := os.Stat(current); err == nil && !fi.IsDir() {
		err := os.Rename(current, rolled)
		if err != nil {
			return xerrors.Errorf("failed to roll journal file: %w", err)
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
	defer close(f.closed)

	for {
		select {
		case je := <-f.incoming:
			if err := f.putEvent(je); err != nil {
				log.Errorw("failed to write out journal event", "event", je, "err", err)
			}
		case <-f.closing:
			_ = f.fi.Close()
			return
		}
	}
}
