package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

func main() {
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
		return fixGoImports(path)
	}); err != nil {
		fmt.Printf("Error fixing go imports: %v\n", err)
		os.Exit(1)
	}
}

func fixGoImports(path string) error {
	sourceFile, err := os.OpenFile(path, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	source, err := io.ReadAll(sourceFile)
	if err != nil {
		return err
	}
	formatted := collapseImportNewlines(source)
	for _, prefix := range groupByPrefixes {
		imports.LocalPrefix = prefix
		formatted, err = imports.Process(path, formatted, nil)
		if err != nil {
			return err
		}
	}
	if !bytes.Equal(source, formatted) {
		if err := replaceFileContent(sourceFile, formatted); err != nil {
			return err
		}
	}
	return nil
}

func replaceFileContent(target *os.File, replacement []byte) error {
	if _, err := target.Seek(0, io.SeekStart); err != nil {
		return err
	}
	written, err := target.Write(replacement)
	if err != nil {
		return err
	}
	return target.Truncate(int64(written))
}

func collapseImportNewlines(content []byte) []byte {
	return importBlockRegex.ReplaceAllFunc(content, func(importBlock []byte) []byte {
		// Replace consecutive newlines with a single newline within the import block
		return consecutiveNewlinesRegex.ReplaceAll(importBlock, newline)
	})
}
