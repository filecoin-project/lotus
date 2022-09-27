package backupds

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"
)

var loghead = datastore.NewKey("/backupds/log/head") // string([logfile base name];[uuid];[unix ts])

func (d *Datastore) startLog(logdir string) error {
	if err := os.MkdirAll(logdir, 0755); err != nil && !os.IsExist(err) {
		return xerrors.Errorf("mkdir logdir ('%s'): %w", logdir, err)
	}

	files, err := os.ReadDir(logdir)
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
		l, latest, err = d.createLog(logdir)
		if err != nil {
			return xerrors.Errorf("creating log: %w", err)
		}
	} else {
		l, latest, err = d.openLog(filepath.Join(logdir, latest))
		if err != nil {
			return xerrors.Errorf("opening log: %w", err)
		}
	}

	if err := l.writeLogHead(latest, d.child); err != nil {
		return xerrors.Errorf("writing new log head: %w", err)
	}

	go d.runLog(l)

	return nil
}

func (d *Datastore) runLog(l *logfile) {
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
			return
		}
	}
}

type logfile struct {
	file *os.File
}

var compactThresh = 2

func (d *Datastore) createLog(logdir string) (*logfile, string, error) {
	ctx := context.TODO()
	p := filepath.Join(logdir, strconv.FormatInt(time.Now().Unix(), 10)+".log.cbor")
	log.Infow("creating log", "file", p)

	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, "", err
	}

	if err := d.Backup(ctx, f); err != nil {
		return nil, "", xerrors.Errorf("writing log base: %w", err)
	}
	if err := f.Sync(); err != nil {
		return nil, "", xerrors.Errorf("sync log base: %w", err)
	}
	log.Infow("log opened", "file", p)

	return &logfile{
		file: f,
	}, filepath.Base(p), nil
}

func (d *Datastore) openLog(p string) (*logfile, string, error) {
	ctx := context.TODO()
	log.Infow("opening log", "file", p)
	lh, err := d.child.Get(ctx, loghead)
	if err != nil {
		return nil, "", xerrors.Errorf("checking log head (logfile '%s'): %w", p, err)
	}

	lhp := strings.Split(string(lh), ";")
	if len(lhp) != 3 {
		return nil, "", xerrors.Errorf("expected loghead to have 3 parts")
	}

	if lhp[0] != filepath.Base(p) {
		return nil, "", xerrors.Errorf("loghead log file doesn't match, opening %s, expected %s", p, lhp[0])
	}

	f, err := os.OpenFile(p, os.O_RDWR, 0644)
	if err != nil {
		return nil, "", err
	}

	var lastLogHead string
	var openCount, vals, logvals int64
	// check file integrity
	clean, err := ReadBackup(f, func(k datastore.Key, v []byte, log bool) error {
		if log {
			logvals++
		} else {
			vals++
		}
		if k == loghead {
			lastLogHead = string(v)
			openCount++
		}
		return nil
	})
	if err != nil {
		return nil, "", xerrors.Errorf("reading backup part of the logfile: %w", err)
	}
	if string(lh) != lastLogHead && clean { // if not clean, user has opted in to ignore truncated logs, this will almost certainly happen
		return nil, "", xerrors.Errorf("loghead didn't match, expected '%s', last in logfile '%s'", string(lh), lastLogHead)
	}

	// make sure we're at the end of the file
	at, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, "", xerrors.Errorf("get current logfile offset: %w", err)
	}
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, "", xerrors.Errorf("get current logfile offset: %w", err)
	}
	if at != end {
		return nil, "", xerrors.Errorf("logfile %s validated %d bytes, but the file has %d bytes (%d more)", p, at, end, end-at)
	}

	compact := logvals > vals*int64(compactThresh)
	if compact || !clean {
		log.Infow("compacting log", "current", p, "openCount", openCount, "baseValues", vals, "logValues", logvals, "truncated", !clean)
		if err := f.Close(); err != nil {
			return nil, "", xerrors.Errorf("closing current log: %w", err)
		}

		l, latest, err := d.createLog(filepath.Dir(p))
		if err != nil {
			return nil, "", xerrors.Errorf("creating compacted log: %w", err)
		}

		if clean {
			log.Infow("compacted log created, cleaning up old", "old", p, "new", latest)
			if err := os.Remove(p); err != nil {
				l.Close() // nolint
				return nil, "", xerrors.Errorf("cleaning up old logfile: %w", err)
			}
		} else {
			log.Errorw("LOG FILE WAS TRUNCATED, KEEPING THE FILE", "old", p, "new", latest)
		}

		return l, latest, nil
	}

	log.Infow("log opened", "file", p, "openCount", openCount, "baseValues", vals, "logValues", logvals)

	// todo: maybe write a magic 'opened at' entry; pad the log to filesystem page to prevent more exotic types of corruption

	return &logfile{
		file: f,
	}, filepath.Base(p), nil
}

func (l *logfile) writeLogHead(logname string, ds datastore.Batching) error {
	ctx := context.TODO()
	lval := []byte(fmt.Sprintf("%s;%s;%d", logname, uuid.New(), time.Now().Unix()))

	err := l.writeEntry(&Entry{
		Key:       loghead.Bytes(),
		Value:     lval,
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		return xerrors.Errorf("writing loghead to the log: %w", err)
	}

	if err := ds.Put(ctx, loghead, lval); err != nil {
		return xerrors.Errorf("writing loghead to the datastore: %w", err)
	}

	log.Infow("new log head", "loghead", string(lval))

	return nil
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
