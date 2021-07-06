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

	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
)

type debugLog struct {
	readPath, writePath, deletePath, stackPath string
	readMx, writeMx, deleteMx, stackMx         sync.Mutex
	readLog, writeLog, deleteLog, stackLog     *os.File
	readCnt, writeCnt, deleteCnt, stackCnt     int
	stackMap                                   map[string]string
}

func openDebugLog(path string) (*debugLog, error) {
	basePath := filepath.Join(path, "debug")
	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		return nil, xerrors.Errorf("error creating debug log directory: %w", err)
	}

	readPath := filepath.Join(basePath, "read.log")
	readFile, err := os.OpenFile(readPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, xerrors.Errorf("error opening read log: %w", err)
	}

	writePath := filepath.Join(basePath, "write.log")
	writeFile, err := os.OpenFile(writePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		_ = readFile.Close()
		return nil, xerrors.Errorf("error opening write log: %w", err)
	}

	deletePath := filepath.Join(basePath, "delete.log")
	deleteFile, err := os.OpenFile(deletePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		_ = readFile.Close()
		_ = writeFile.Close()
		return nil, xerrors.Errorf("error opening delete log: %w", err)
	}

	stackPath := filepath.Join(basePath, "stack.log")
	stackFile, err := os.OpenFile(stackPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		_ = readFile.Close()
		_ = writeFile.Close()
		_ = deleteFile.Close()
		return nil, xerrors.Errorf("error opening stack log: %w", err)
	}

	return &debugLog{
		readPath:   readPath,
		writePath:  writePath,
		deletePath: deletePath,
		stackPath:  stackPath,
		readLog:    readFile,
		writeLog:   writeFile,
		deleteLog:  deleteFile,
		stackLog:   stackFile,
		stackMap:   make(map[string]string),
	}, nil
}

func (d *debugLog) LogReadMiss(cid cid.Cid) {
	if d == nil {
		return
	}

	stack := d.getStack()

	d.readMx.Lock()
	defer d.readMx.Unlock()

	d.readCnt++

	_, err := fmt.Fprintf(d.readLog, "%s %s %s\n", d.timestamp(), cid, stack)
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

	d.writeMx.Lock()
	defer d.writeMx.Unlock()

	d.writeCnt++

	_, err := fmt.Fprintf(d.writeLog, "%s %s%s\n", d.timestamp(), blk.Cid(), stack)
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

	d.writeMx.Lock()
	defer d.writeMx.Unlock()

	d.writeCnt += len(blks)

	now := d.timestamp()
	for _, blk := range blks {
		_, err := fmt.Fprintf(d.writeLog, "%s %s%s\n", now, blk.Cid(), stack)
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

	d.deleteMx.Lock()
	defer d.deleteMx.Unlock()

	d.deleteCnt += len(cids)

	now := d.timestamp()
	for _, c := range cids {
		_, err := fmt.Fprintf(d.deleteLog, "%s %s\n", now, c)
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
	d.rotateReadLog()
	d.rotateWriteLog()
	d.rotateDeleteLog()
	d.rotateStackLog()
}

func (d *debugLog) rotateReadLog() {
	d.readMx.Lock()
	defer d.readMx.Unlock()

	if d.readCnt == 0 {
		return
	}

	err := d.rotate(d.readLog, d.readPath)
	if err != nil {
		log.Warnf("error rotating read log: %s", err)
		return
	}

	d.readLog, err = os.OpenFile(d.readPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Warnf("error opening log file: %s", err)
		return
	}

	d.readCnt = 0
}

func (d *debugLog) rotateWriteLog() {
	d.writeMx.Lock()
	defer d.writeMx.Unlock()

	if d.writeCnt == 0 {
		return
	}

	err := d.rotate(d.writeLog, d.writePath)
	if err != nil {
		log.Warnf("error rotating write log: %s", err)
		return
	}

	d.writeLog, err = os.OpenFile(d.writePath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Warnf("error opening write log file: %s", err)
		return
	}

	d.writeCnt = 0
}

func (d *debugLog) rotateDeleteLog() {
	d.deleteMx.Lock()
	defer d.deleteMx.Unlock()

	if d.deleteCnt == 0 {
		return
	}

	err := d.rotate(d.deleteLog, d.deletePath)
	if err != nil {
		log.Warnf("error rotating delete log: %s", err)
		return
	}

	d.deleteLog, err = os.OpenFile(d.deletePath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Warnf("error opening delete log file: %s", err)
		return
	}

	d.deleteCnt = 0
}

func (d *debugLog) rotateStackLog() {
	d.stackMx.Lock()
	defer d.stackMx.Unlock()

	if d.stackCnt == 0 {
		return
	}

	err := d.rotate(d.stackLog, d.stackPath)
	if err != nil {
		log.Warnf("error rotating stack log: %s", err)
		return
	}

	d.stackLog, err = os.OpenFile(d.stackPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Warnf("error opening stack log file: %s", err)
		return
	}

	d.stackCnt = 0
}

func (d *debugLog) rotate(f *os.File, path string) error {
	err := f.Close()
	if err != nil {
		return xerrors.Errorf("error closing file: %w", err)
	}

	arxivPath := fmt.Sprintf("%s-%d", path, time.Now().Unix())
	err = os.Rename(path, arxivPath)
	if err != nil {
		return xerrors.Errorf("error moving file: %w", err)
	}

	go func() {
		cmd := exec.Command("gzip", arxivPath)
		err := cmd.Run()
		if err != nil {
			log.Warnf("error compressing log: %s", err)
		}
	}()

	return nil
}

func (d *debugLog) Close() error {
	if d == nil {
		return nil
	}

	d.readMx.Lock()
	err1 := d.readLog.Close()
	d.readMx.Unlock()

	d.writeMx.Lock()
	err2 := d.writeLog.Close()
	d.writeMx.Unlock()

	d.deleteMx.Lock()
	err3 := d.deleteLog.Close()
	d.deleteMx.Unlock()

	d.stackMx.Lock()
	err4 := d.stackLog.Close()
	d.stackMx.Unlock()

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
		d.stackCnt++

		_, err := fmt.Fprintf(d.stackLog, "%s\n%s\n", repr, sk)
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
