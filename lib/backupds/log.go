package backupds

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
)

func (d *Datastore) startLog(logdir string) error {
	if err := os.MkdirAll(logdir, 0755); err != nil && !os.IsExist(err) {
		return xerrors.Errorf("mkdir logdir ('%s'): %w", logdir, err)
	}

	files, err := ioutil.ReadDir(logdir)
	if err != nil {
		return xerrors.Errorf("read logdir ('%s'): %w", logdir, err)
	}

	var latest string
	var latestTs int64

	for _, file := range files {
		fn := file.Name()
		if !strings.HasSuffix(fn, ".log.cbor") {
			log.Warn("logfile with wrong file extension", fn)
			continue
		}
		sec, err := strconv.ParseInt(fn[:len(".log.cbor")], 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing logfile as a number: %w", err)
		}

		if sec > latestTs {
			latestTs = sec
			latest = file.Name()
		}
	}

	var l *logfile
	if latest == "" {
		l, err = d.createLog(logdir)
		if err != nil {
			return xerrors.Errorf("creating log: %w", err)
		}
	} else {
		l, err = openLog(filepath.Join(logdir, latest))
		if err != nil {
			return xerrors.Errorf("opening log: %w", err)
		}
	}

	go func() {
		defer close(d.closed)
		for {
			select {
			case ent := <-d.log:
				if err := l.writeEntry(&ent); err != nil {
					log.Errorw("failed to write log entry", "error", err)
					// todo try to do something, maybe start a new log file (but not when we're out of disk space)
				}

				// todo: batch writes when multiple are pending; flush on a timer
				if err := l.file.Sync(); err != nil {
					log.Errorw("failed to sync log", "error", err)
				}
			case <-d.closing:
				if err := l.Close(); err != nil {
					log.Errorw("failed to close log", "error", err)
				}
			}
		}
	}()

	return nil
}

type logfile struct {
	file *os.File
}

func (d *Datastore) createLog(logdir string) (*logfile, error) {
	p := filepath.Join(logdir, strconv.FormatInt(time.Now().Unix(), 10)+".log.cbor")
	log.Infow("creating log", "file", p)

	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}

	if err := d.Backup(f); err != nil {
		return nil, xerrors.Errorf("writing log base: %w", err)
	}
	if err := f.Sync(); err != nil {
		return nil, xerrors.Errorf("sync log base: %w", err)
	}
	log.Infow("log opened", "file", p)

	// todo: maybe write a magic 'opened at' entry; pad the log to filesystem page to prevent more exotic types of corruption

	return &logfile{
		file: f,
	}, nil
}

func openLog(p string) (*logfile, error) {
	log.Infow("opening log", "file", p)
	f, err := os.OpenFile(p, os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	// check file integrity
	err = ReadBackup(f, func(_ datastore.Key, _ []byte) error {
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("reading backup part of the logfile: %w", err)
	}
	log.Infow("log opened", "file", p)

	// make sure we're at the end of the file
	at, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, xerrors.Errorf("get current logfile offset: %w", err)
	}
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, xerrors.Errorf("get current logfile offset: %w", err)
	}
	if at != end {
		return nil, xerrors.Errorf("logfile %s validated %d bytes, but the file has %d bytes (%d more)", p, at, end, end-at)
	}

	// todo: maybe write a magic 'opened at' entry; pad the log to filesystem page to prevent more exotic types of corruption

	return &logfile{
		file: f,
	}, nil
}

func (l *logfile) writeEntry(e *Entry) error {
	// todo: maybe marshal to some temp buffer, then put into the file?
	if err := e.MarshalCBOR(l.file); err != nil {
		return xerrors.Errorf("writing log entry: %w", err)
	}

	return nil
}

func (l *logfile) Close() error {
	// todo: maybe write a magic 'close at' entry; pad the log to filesystem page to prevent more exotic types of corruption

	if err := l.file.Close(); err != nil {
		return err
	}

	l.file = nil

	return nil
}
