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

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
)

type debugLog struct {
	readPath, writePath, movePath, stackPath string
	readMx, writeMx, moveMx, stackMx         sync.Mutex
	readLog, writeLog, moveLog, stackLog     *os.File
	readCnt, writeCnt, moveCnt, stackCnt     int
	stackMap                                 map[string]struct{}
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

	movePath := filepath.Join(basePath, "move.log")
	moveFile, err := os.OpenFile(movePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		_ = readFile.Close()
		_ = writeFile.Close()
		return nil, xerrors.Errorf("error opening move log: %w", err)
	}

	stackPath := filepath.Join(basePath, "stack.log")
	stackFile, err := os.OpenFile(stackPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		_ = readFile.Close()
		_ = writeFile.Close()
		_ = moveFile.Close()
		return nil, xerrors.Errorf("error opening stack log: %w", err)
	}

	return &debugLog{
		readPath:  readPath,
		writePath: writePath,
		movePath:  movePath,
		stackPath: stackPath,
		readLog:   readFile,
		writeLog:  writeFile,
		moveLog:   moveFile,
		stackLog:  stackFile,
		stackMap:  make(map[string]struct{}),
	}, nil
}

func (d *debugLog) LogReadMiss(curTs *types.TipSet, cid cid.Cid) {
	if d == nil {
		return
	}

	stack := d.getStack()

	var epoch abi.ChainEpoch
	if curTs != nil {
		epoch = curTs.Height()
	}

	d.readMx.Lock()
	defer d.readMx.Unlock()

	d.readCnt++

	_, err := fmt.Fprintf(d.readLog, "%s %d %s %s\n", time.Now(), epoch, cid, stack)
	if err != nil {
		log.Warnf("error writing read log: %s", err)
	}
}

func (d *debugLog) LogWrite(curTs *types.TipSet, blk blocks.Block, writeEpoch abi.ChainEpoch) {
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

	_, err := fmt.Fprintf(d.writeLog, "%s %d %s %d%s\n", time.Now(), curTs.Height(), blk.Cid(), writeEpoch, stack)
	if err != nil {
		log.Warnf("error writing write log: %s", err)
	}
}

func (d *debugLog) LogWriteMany(curTs *types.TipSet, blks []blocks.Block, writeEpoch abi.ChainEpoch) {
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

	now := time.Now()
	for _, blk := range blks {
		_, err := fmt.Fprintf(d.writeLog, "%s %d %s %d%s\n", now, curTs.Height(), blk.Cid(), writeEpoch, stack)
		if err != nil {
			log.Warnf("error writing write log: %s", err)
			break
		}
	}
}

func (d *debugLog) LogMove(curTs *types.TipSet, cid cid.Cid) {
	if d == nil {
		return
	}

	d.moveMx.Lock()
	defer d.moveMx.Unlock()

	d.moveCnt++

	_, err := fmt.Fprintf(d.moveLog, "%d %s\n", curTs.Height(), cid)
	if err != nil {
		log.Warnf("error writing move log: %s", err)
	}
}

func (d *debugLog) Flush() {
	if d == nil {
		return
	}

	// rotate non-empty logs
	d.rotateReadLog()
	d.rotateWriteLog()
	d.rotateMoveLog()
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

func (d *debugLog) rotateMoveLog() {
	d.moveMx.Lock()
	defer d.moveMx.Unlock()

	if d.moveCnt == 0 {
		return
	}

	err := d.rotate(d.moveLog, d.movePath)
	if err != nil {
		log.Warnf("error rotating move log: %s", err)
		return
	}

	d.moveLog, err = os.OpenFile(d.movePath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Warnf("error opening move log file: %s", err)
		return
	}

	d.moveCnt = 0
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

	d.moveMx.Lock()
	err3 := d.moveLog.Close()
	d.moveMx.Unlock()

	d.stackMx.Lock()
	err4 := d.stackLog.Close()
	d.stackMx.Unlock()

	return multierr.Combine(err1, err2, err3, err4)
}

func (d *debugLog) getStack() string {
	sk := d.getNormalizedStackTrace()
	hash := sha256.Sum256([]byte(sk))
	key := string(hash[:])
	repr := hex.EncodeToString(hash[:])

	d.stackMx.Lock()
	_, ok := d.stackMap[key]

	if !ok {
		_, err := fmt.Fprintf(d.stackLog, "%s\n%s\n", repr, sk)
		if err != nil {
			log.Warnf("error writing stack trace: %s", err)
		}
	}

	d.stackMap[key] = struct{}{}
	d.stackCnt++
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
		if line[len(line)-1] == ')' {
			idx := strings.LastIndex(line, "(")
			if idx < 0 {
				continue
			}
			lines[i] = line[:idx]
		}
	}

	return strings.Join(lines, "\n")
}
