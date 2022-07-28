package splitstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"
)

type debugLog struct {
	readLog, writeLog, deleteLog, stackLog *debugLogOp

	stackMx  sync.Mutex
	stackMap map[string]string
}

type debugLogOp struct {
	path  string
	mx    sync.Mutex
	log   *os.File
	count int
}

func openDebugLog(path string) (*debugLog, error) {
	basePath := filepath.Join(path, "debug")
	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		return nil, err
	}

	readLog, err := openDebugLogOp(basePath, "read.log")
	if err != nil {
		return nil, err
	}

	writeLog, err := openDebugLogOp(basePath, "write.log")
	if err != nil {
		_ = readLog.Close()
		return nil, err
	}

	deleteLog, err := openDebugLogOp(basePath, "delete.log")
	if err != nil {
		_ = readLog.Close()
		_ = writeLog.Close()
		return nil, err
	}

	stackLog, err := openDebugLogOp(basePath, "stack.log")
	if err != nil {
		_ = readLog.Close()
		_ = writeLog.Close()
		_ = deleteLog.Close()
		return nil, xerrors.Errorf("error opening stack log: %w", err)
	}

	return &debugLog{
		readLog:   readLog,
		writeLog:  writeLog,
		deleteLog: deleteLog,
		stackLog:  stackLog,
		stackMap:  make(map[string]string),
	}, nil
}

func (d *debugLog) LogReadMiss(cid cid.Cid) {
	if d == nil {
		return
	}

	stack := d.getStack()
	err := d.readLog.Log("%s %s %s\n", d.timestamp(), cid, stack)
	if err != nil {
		log.Warnf("error writing read log: %s", err)
	}
}

func (d *debugLog) LogWrite(blk blocks.Block) {
	if d == nil {
		return
	}

	var stack string
	if enableDebugLogWriteTraces {
		stack = " " + d.getStack()
	}

	err := d.writeLog.Log("%s %s%s\n", d.timestamp(), blk.Cid(), stack)
	if err != nil {
		log.Warnf("error writing write log: %s", err)
	}
}

func (d *debugLog) LogWriteMany(blks []blocks.Block) {
	if d == nil {
		return
	}

	var stack string
	if enableDebugLogWriteTraces {
		stack = " " + d.getStack()
	}

	now := d.timestamp()
	for _, blk := range blks {
		err := d.writeLog.Log("%s %s%s\n", now, blk.Cid(), stack)
		if err != nil {
			log.Warnf("error writing write log: %s", err)
			break
		}
	}
}

func (d *debugLog) LogDelete(cids []cid.Cid) {
	if d == nil {
		return
	}

	now := d.timestamp()
	for _, c := range cids {
		err := d.deleteLog.Log("%s %s\n", now, c)
		if err != nil {
			log.Warnf("error writing delete log: %s", err)
			break
		}
	}
}

func (d *debugLog) Flush() {
	if d == nil {
		return
	}

	// rotate non-empty logs
	d.readLog.Rotate()
	d.writeLog.Rotate()
	d.deleteLog.Rotate()
	d.stackLog.Rotate()
}

func (d *debugLog) Close() error {
	if d == nil {
		return nil
	}

	err1 := d.readLog.Close()
	err2 := d.writeLog.Close()
	err3 := d.deleteLog.Close()
	err4 := d.stackLog.Close()

	return multierr.Combine(err1, err2, err3, err4)
}

func (d *debugLog) getStack() string {
	sk := d.getNormalizedStackTrace()
	hash := sha256.Sum256([]byte(sk))
	key := string(hash[:])

	d.stackMx.Lock()
	repr, ok := d.stackMap[key]
	if !ok {
		repr = hex.EncodeToString(hash[:])
		d.stackMap[key] = repr

		err := d.stackLog.Log("%s\n%s\n", repr, sk)
		if err != nil {
			log.Warnf("error writing stack trace for %s: %s", repr, err)
		}
	}
	d.stackMx.Unlock()

	return repr
}

func (d *debugLog) getNormalizedStackTrace() string {
	sk := string(debug.Stack())

	// Normalization for deduplication
	// skip first line -- it's the goroutine
	// for each line that ends in a ), remove the call args -- these are the registers
	lines := strings.Split(sk, "\n")[1:]
	for i, line := range lines {
		if len(line) > 0 && line[len(line)-1] == ')' {
			idx := strings.LastIndex(line, "(")
			if idx < 0 {
				continue
			}
			lines[i] = line[:idx]
		}
	}

	return strings.Join(lines, "\n")
}

func (d *debugLog) timestamp() string {
	ts, _ := time.Now().MarshalText()
	return string(ts)
}

func openDebugLogOp(basePath, name string) (*debugLogOp, error) {
	path := filepath.Join(basePath, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, xerrors.Errorf("error opening %s: %w", name, err)
	}

	return &debugLogOp{path: path, log: file}, nil
}

func (d *debugLogOp) Close() error {
	d.mx.Lock()
	defer d.mx.Unlock()

	return d.log.Close()
}

func (d *debugLogOp) Log(template string, arg ...interface{}) error {
	d.mx.Lock()
	defer d.mx.Unlock()

	d.count++
	_, err := fmt.Fprintf(d.log, template, arg...)
	return err
}

func (d *debugLogOp) Rotate() {
	d.mx.Lock()
	defer d.mx.Unlock()

	if d.count == 0 {
		return
	}

	err := d.log.Close()
	if err != nil {
		log.Warnf("error closing log (file: %s): %s", d.path, err)
		return
	}

	arxivPath := fmt.Sprintf("%s-%d", d.path, time.Now().Unix())
	err = os.Rename(d.path, arxivPath)
	if err != nil {
		log.Warnf("error moving log (file: %s): %s", d.path, err)
		return
	}

	go func() {
		cmd := exec.Command("gzip", arxivPath)
		err := cmd.Run()
		if err != nil {
			log.Warnf("error compressing log (file: %s): %s", arxivPath, err)
		}
	}()

	d.count = 0
	d.log, err = os.OpenFile(d.path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Warnf("error opening log (file: %s): %s", d.path, err)
		return
	}
}
