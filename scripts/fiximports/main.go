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

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/imports"
	"golang.org/x/xerrors"
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
	fileContents, err := readFilesParallel(files, numWorkers)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error reading files: %v\n", err)
		os.Exit(1)
	}

	// Because we have multiple ways of separating imports, we have to imports.Process for each one
	// but imports.LocalPrefix is a global, so we have to set it for each group and process files
	// in parallel.
	for _, prefix := range groupByPrefixes {
		imports.LocalPrefix = prefix
		if err := processFilesParallel(fileContents, numWorkers); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error processing files with prefix %s: %v\n", prefix, err)
			os.Exit(1)
		}
	}

	// Write modified files in parallel
	if err := writeFilesParallel(fileContents, numWorkers); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error writing files: %v\n", err)
		os.Exit(1)
	}
}

func readFilesParallel(files []string, numWorkers int) ([]*fileContent, error) {
	fileContents := make([]*fileContent, len(files))

	var g errgroup.Group
	g.SetLimit(numWorkers)

	for i, path := range files {
		g.Go(func() error {
			content, err := os.ReadFile(path)
			if err != nil {
				return xerrors.Errorf("reading %s: %w", path, err)
			}

			// Collapse is a cheap operation to do here
			collapsed := collapseImportNewlines(content)
			fileContents[i] = &fileContent{
				path:     path,
				original: content,
				current:  collapsed,
				changed:  !bytes.Equal(content, collapsed),
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return fileContents, nil
}

func processFilesParallel(fileContents []*fileContent, numWorkers int) error {
	var g errgroup.Group
	g.SetLimit(numWorkers)

	for _, file := range fileContents {
		if file == nil {
			continue
		}
		g.Go(func() error {
			formatted, err := imports.Process(file.path, file.current, nil)
			if err != nil {
				return xerrors.Errorf("processing %s: %w", file.path, err)
			}

			if !bytes.Equal(file.current, formatted) {
				file.current = formatted
				file.changed = true
			}
			return nil
		})
	}

	return g.Wait()
}

func writeFilesParallel(fileContents []*fileContent, numWorkers int) error {
	var g errgroup.Group
	g.SetLimit(numWorkers)

	for _, file := range fileContents {
		if file == nil || !file.changed {
			continue
		}
		g.Go(func() error {
			if err := os.WriteFile(file.path, file.current, 0666); err != nil {
				return xerrors.Errorf("writing %s: %w", file.path, err)
			}
			return nil
		})
	}

	return g.Wait()
}

func collapseImportNewlines(content []byte) []byte {
	return importBlockRegex.ReplaceAllFunc(content, func(importBlock []byte) []byte {
		// Replace consecutive newlines with a single newline within the import block
		return consecutiveNewlinesRegex.ReplaceAll(importBlock, newline)
	})
}
