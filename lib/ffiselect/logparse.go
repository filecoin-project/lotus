package ffiselect

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/zap"
)

var log = logging.Logger("ffiselect")

type LogWriter struct {
	ctx    []any
	errOut io.Writer
	re     *regexp.Regexp
}

func NewLogWriter(logctx []any, errOut io.Writer) *LogWriter {
	re := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3})\s+(\w+)\s+(.*)$`)
	return &LogWriter{
		ctx:    logctx,
		errOut: errOut,
		re:     re,
	}
}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	reader := bufio.NewReader(bytes.NewReader(p))
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}

		lineStr := string(line)
		// trim trailing \n
		lineStr = strings.TrimSpace(lineStr)

		matches := lw.re.FindStringSubmatch(lineStr)
		if matches == nil {
			// Line didn't match the expected format, write it to stderr as-is
			_, err := lw.errOut.Write(line)
			if err != nil {
				return 0, err
			}
			continue
		}

		timestamp, logLevel, message := matches[1], matches[2], matches[3]
		logTime, err := time.Parse("2006-01-02T15:04:05.000", timestamp)
		if err != nil {
			_, err := lw.errOut.Write(line)
			if err != nil {
				return 0, err
			}
			continue
		}

		var zapLevel zap.AtomicLevel
		switch logLevel {
		case "DEBUG":
			zapLevel = zap.NewAtomicLevelAt(zap.DebugLevel)
		case "INFO":
			zapLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
		case "WARN":
			zapLevel = zap.NewAtomicLevelAt(zap.WarnLevel)
		case "ERROR":
			zapLevel = zap.NewAtomicLevelAt(zap.ErrorLevel)
		default:
			_, err := lw.errOut.Write(line)
			if err != nil {
				return 0, err
			}
			continue
		}

		log.With(zap.Time("timestamp", logTime)).Logw(zapLevel.Level(), message, lw.ctx...)
	}
	return len(p), nil
}
