package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"
)

func InitializeSystemJournal(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	j, err := OpenFSJournal(dir)
	if err != nil {
		return err
	}
	currentJournal = j
	return nil
}

func Add(sys string, val interface{}) {
	if currentJournal == nil {
		log.Warn("no journal configured")
		return
	}
	currentJournal.AddEntry(sys, val)
}

var log = logging.Logger("journal")

var currentJournal Journal

type Journal interface {
	AddEntry(system string, obj interface{})
	Close() error
}

// fsJournal is a basic journal backed by files on a filesystem
type fsJournal struct {
	fi    *os.File
	fSize int64

	lk sync.Mutex

	journalDir string

	incoming         chan *JournalEntry
	journalSizeLimit int64

	closing chan struct{}
}

func OpenFSJournal(dir string) (*fsJournal, error) {
	fsj := &fsJournal{
		journalDir:       dir,
		incoming:         make(chan *JournalEntry, 32),
		journalSizeLimit: 1 << 30,
		closing:          make(chan struct{}),
	}

	if err := fsj.rollJournalFile(); err != nil {
		return nil, err
	}

	go fsj.runLoop()

	return fsj, nil
}

type JournalEntry struct {
	System    string
	Timestamp time.Time
	Val       interface{}
}

func (fsj *fsJournal) putEntry(je *JournalEntry) error {
	b, err := json.Marshal(je)
	if err != nil {
		return err
	}
	n, err := fsj.fi.Write(append(b, '\n'))
	if err != nil {
		return err
	}

	fsj.fSize += int64(n)

	if fsj.fSize >= fsj.journalSizeLimit {
		fsj.rollJournalFile()
	}

	return nil
}

func (fsj *fsJournal) rollJournalFile() error {
	if fsj.fi != nil {
		fsj.fi.Close()
	}

	nfi, err := os.Create(filepath.Join(fsj.journalDir, fmt.Sprintf("lotus-journal-%s.ndjson", time.Now().Format(time.RFC3339))))
	if err != nil {
		return xerrors.Errorf("failed to open journal file: %w", err)
	}

	fsj.fi = nfi
	fsj.fSize = 0
	return nil
}

func (fsj *fsJournal) runLoop() {
	for {
		select {
		case je := <-fsj.incoming:
			if err := fsj.putEntry(je); err != nil {
				log.Errorw("failed to write out journal entry", "entry", je, "err", err)
			}
		case <-fsj.closing:
			fsj.fi.Close()
			return
		}
	}
}

func (fsj *fsJournal) AddEntry(system string, obj interface{}) {
	je := &JournalEntry{
		System:    system,
		Timestamp: time.Now(),
		Val:       obj,
	}
	select {
	case fsj.incoming <- je:
	case <-fsj.closing:
		log.Warnw("journal closed but tried to log event", "entry", je)
	}
}

func (fsj *fsJournal) Close() error {
	close(fsj.closing)
	return nil
}
