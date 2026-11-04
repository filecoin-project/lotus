// Package parsehelper provides small helpers for working with go/ast and go/parser.
package parsehelper

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ParseDir parses every *.go file directly in dir and returns the files belonging
// to package pkgName, keyed by absolute file path. Files in other packages
// (e.g. _test packages) are skipped.
//
// It is a focused replacement for the deprecated go/parser.ParseDir + *ast.Package
// combo for tools that walk a single package's ASTs. It does not consider build
// tags; if you need build-aware loading, use golang.org/x/tools/go/packages.
func ParseDir(fset *token.FileSet, dir, pkgName string, mode parser.Mode) (map[string]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make(map[string]*ast.File)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path, err := filepath.Abs(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(fset, path, nil, mode)
		if err != nil {
			return nil, err
		}
		if f.Name.Name != pkgName {
			continue
		}
		files[path] = f
	}
	return files, nil
}
