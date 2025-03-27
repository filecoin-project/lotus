package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/tools/imports"
)

var (
	// groupByPrefixes is the list of import prefixes that should _each_ be grouped separately.
	// See: imports.LocalPrefix.
	groupByPrefixes = []string{
		"github.com/filecoin-project",
		"github.com/filecoin-project/lotus",
	}
	newline                  = []byte("\n")
	importBlockRegex         = regexp.MustCompile(`(?s)import\s*\((.*?)\)`)
	consecutiveNewlinesRegex = regexp.MustCompile(`\n\s*\n`)
)

type fileContent struct {
	path     string
	original []byte
	current  []byte
	changed  bool
}

func main() {
	numWorkers := runtime.NumCPU()

	// Collect all the filenames that we want to process
	var files []string
	if err := filepath.Walk(".", func(path string, info fs.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case // Skip the entire "./extern/..." directory and its contents.
			strings.HasPrefix(path, "extern/"):
			return filepath.SkipDir
		case // Skip directories, generated cborgen go files and any other non-go files.
			info.IsDir(),
			strings.HasSuffix(info.Name(), "_cbor_gen.go"),
			!strings.HasSuffix(info.Name(), ".go"):
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// Read all file contents in parallel
	fileContents := readFilesParallel(files, numWorkers)

	// Because we have multiple ways of separating imports, we have to imports.Process for each one
	// but imports.LocalPrefix is a global, so we have to set it for each group and process files
	// in parallel.
	for _, prefix := range groupByPrefixes {
		imports.LocalPrefix = prefix
		processFilesParallel(fileContents, numWorkers)
	}

	// Write modified files in parallel
	writeFilesParallel(fileContents, numWorkers)
}

func readFilesParallel(files []string, numWorkers int) []*fileContent {
	var readErrors int64
	var wg sync.WaitGroup
	fileContents := make([]*fileContent, len(files))
	filesChan := make(chan int, len(files))

	// Fill a queue with file indices that we can consume in parallel
	for i := range files {
		filesChan <- i
	}
	close(filesChan)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range filesChan {
				path := files[i]
				content, err := os.ReadFile(path)
				if err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "Error reading file %s: %v\n", path, err)
					atomic.AddInt64(&readErrors, 1)
					continue
				}

				// Collapse is a cheap operation to do here
				collapsed := collapseImportNewlines(content)
				fileContents[i] = &fileContent{
					path:     path,
					original: content,
					current:  collapsed,
					changed:  !bytes.Equal(content, collapsed),
				}
			}
		}()
	}

	wg.Wait()

	if readErrors > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to read %d files\n", readErrors)
		os.Exit(1)
	}

	return fileContents
}

func processFilesParallel(fileContents []*fileContent, numWorkers int) {
	var processErrors int64
	var wg sync.WaitGroup
	filesChan := make(chan int, len(fileContents))

	// Fill a queue with file indices that we can consume in parallel
	for i := range fileContents {
		if fileContents[i] != nil { // shouldn't be nil, but just in case
			filesChan <- i
		}
	}
	close(filesChan)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range filesChan {
				file := fileContents[i]
				formatted, err := imports.Process(file.path, file.current, nil)
				if err != nil {
					atomic.AddInt64(&processErrors, 1)
					_, _ = fmt.Fprintf(os.Stderr, "Error processing %s: %v", file.path, err)
					continue
				}

				if !bytes.Equal(file.current, formatted) {
					file.current = formatted
					file.changed = true
				}
			}
		}()
	}

	wg.Wait()

	if processErrors > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to process %d files\n", processErrors)
		os.Exit(1)
	}
}

func writeFilesParallel(fileContents []*fileContent, numWorkers int) {
	var writeErrors int64
	var wg sync.WaitGroup

	// Only process changed files
	changedFiles := make(chan *fileContent, len(fileContents))
	for _, file := range fileContents {
		if file != nil && file.changed {
			changedFiles <- file
		}
	}
	close(changedFiles)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range changedFiles {
				// Only write if content has actually changed from original
				if !bytes.Equal(file.original, file.current) {
					if err := os.WriteFile(file.path, file.current, 0666); err != nil {
						atomic.AddInt64(&writeErrors, 1)
						_, _ = fmt.Fprintf(os.Stderr, "Error writing file %s: %v\n", file.path, err)
					}
				}
			}
		}()
	}

	wg.Wait()

	if writeErrors > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to write %d files\n", writeErrors)
		os.Exit(1)
	}
}

func collapseImportNewlines(content []byte) []byte {
	return importBlockRegex.ReplaceAllFunc(content, func(importBlock []byte) []byte {
		// Replace consecutive newlines with a single newline within the import block
		return consecutiveNewlinesRegex.ReplaceAll(importBlock, newline)
	})
}
