package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/icza/backscanner"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

var (
	panicLog           = logging.Logger("panic-reporter")
	defaultJournalTail = 500
)

// PanicReportingPath is the name of the subdir created within the repoPath
// path provided to GeneratePanicReport
var PanicReportingPath = "panic-reports"

// PanicReportJournalTail is the number of lines captured from the end of
// the lotus journal to be included in the panic report.
var PanicReportJournalTail = defaultJournalTail

// GenerateNodePanicReport produces a timestamped dump of the application state
// for inspection and debugging purposes. Call this function from any place
// where a panic or severe error needs to be examined. `persistPath` is the
// path where the reports should be saved. `repoPath` is the path where the
// journal should be read from. `label` is an optional string to include
// next to the report timestamp.
//
// This function should be called for panics originating from the Lotus daemon.
func GenerateNodePanicReport(persistPath, repoPath, label string) {
	generatePanicReport(NodeUserVersion(), persistPath, repoPath, label)
}

// GenerateMinerPanicReport produces a timestamped dump of the application state
// for inspection and debugging purposes. Call this function from any place
// where a panic or severe error needs to be examined. `persistPath` is the
// path where the reports should be saved. `repoPath` is the path where the
// journal should be read from. `label` is an optional string to include
// next to the report timestamp.
//
// This function should be called for panics originating from the Lotus miner.
func GenerateMinerPanicReport(persistPath, repoPath, label string) {
	generatePanicReport(MinerUserVersion(), persistPath, repoPath, label)
}

func generatePanicReport(buildVersion BuildVersion, persistPath, repoPath, label string) {
	// make sure we always dump the latest logs on the way out
	// especially since we're probably panicking
	defer panicLog.Sync() //nolint:errcheck

	if persistPath == "" && repoPath == "" {
		panicLog.Warn("missing persist and repo paths, aborting panic report creation")
		return
	}

	reportPath := filepath.Join(repoPath, PanicReportingPath, generateReportName(label))
	if persistPath != "" {
		reportPath = filepath.Join(persistPath, generateReportName(label))
	}
	panicLog.Warnf("generating panic report at %s", reportPath)

	tl := os.Getenv("LOTUS_PANIC_JOURNAL_LOOKBACK")
	if tl != "" && PanicReportJournalTail == defaultJournalTail {
		i, err := strconv.Atoi(tl)
		if err == nil {
			PanicReportJournalTail = i
		}
	}

	err := os.MkdirAll(reportPath, 0755)
	if err != nil {
		panicLog.Error(err.Error())
		return
	}

	writeAppVersion(buildVersion, filepath.Join(reportPath, "version"))
	writeStackTrace(filepath.Join(reportPath, "stacktrace.dump"))
	writeProfile("goroutines", filepath.Join(reportPath, "goroutines.pprof.gz"))
	writeProfile("heap", filepath.Join(reportPath, "heap.pprof.gz"))
	writeJournalTail(PanicReportJournalTail, repoPath, filepath.Join(reportPath, "journal.ndjson"))
}

func writeAppVersion(buildVersion BuildVersion, file string) {
	f, err := os.Create(file)
	if err != nil {
		panicLog.Error(err.Error())
	}
	defer f.Close() //nolint:errcheck

	versionString := []byte(string(buildVersion) + buildconstants.BuildTypeString() + CurrentCommit + "\n")
	if _, err := f.Write(versionString); err != nil {
		panicLog.Error(err.Error())
	}
}

func writeStackTrace(file string) {
	f, err := os.Create(file)
	if err != nil {
		panicLog.Error(err.Error())
	}
	defer f.Close() //nolint:errcheck

	if _, err := f.Write(debug.Stack()); err != nil {
		panicLog.Error(err.Error())
	}

}

func writeProfile(profileType string, file string) {
	p := pprof.Lookup(profileType)
	if p == nil {
		panicLog.Warnf("%s profile not available", profileType)
		return
	}
	f, err := os.Create(file)
	if err != nil {
		panicLog.Error(err.Error())
		return
	}
	defer f.Close() //nolint:errcheck

	if err := p.WriteTo(f, 0); err != nil {
		panicLog.Error(err.Error())
	}
}

func writeJournalTail(tailLen int, repoPath, file string) {
	if repoPath == "" {
		panicLog.Warn("repo path is empty, aborting copy of journal log")
		return
	}

	f, err := os.Create(file)
	if err != nil {
		panicLog.Error(err.Error())
		return
	}
	defer f.Close() //nolint:errcheck

	jPath, err := getLatestJournalFilePath(repoPath)
	if err != nil {
		panicLog.Warnf("failed getting latest journal: %s", err.Error())
		return
	}
	j, err := os.OpenFile(jPath, os.O_RDONLY, 0400)
	if err != nil {
		panicLog.Error(err.Error())
		return
	}
	js, err := j.Stat()
	if err != nil {
		panicLog.Error(err.Error())
		return
	}
	jScan := backscanner.New(j, int(js.Size()))
	linesWritten := 0
	for {
		if linesWritten > tailLen {
			break
		}
		line, _, err := jScan.LineBytes()
		if err != nil {
			if err != io.EOF {
				panicLog.Error(err.Error())
			}
			break
		}
		if _, err := f.Write(line); err != nil {
			panicLog.Error(err.Error())
			break
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			panicLog.Error(err.Error())
			break
		}
		linesWritten++
	}
}

func getLatestJournalFilePath(repoPath string) (string, error) {
	journalPath := filepath.Join(repoPath, "journal")
	entries, err := os.ReadDir(journalPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(journalPath, entries[len(entries)-1].Name()), nil
}

func generateReportName(label string) string {
	label = strings.ReplaceAll(label, " ", "")
	return fmt.Sprintf("report_%s_%s", label, time.Now().Format("2006-01-02T150405"))
}
