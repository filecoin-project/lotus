package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

//go:generate go run ./gen.go ..

//go:embed template.yml
var templateFile embed.FS

type (
	dirs  = []string
	suite = string
)

// groupedUnitTests maps suite names to top-level directories that should be
// included in that suite. The program adds an implicit group "rest" that
// includes all other top-level directories.
var groupedUnitTests = map[suite]dirs{
	"unit-node":    {"node"},
	"unit-storage": {"storage", "extern"},
	"unit-cli":     {"cli", "cmd", "api"},
}

func main() {
	if len(os.Args) != 2 {
		panic("expected path to repo as argument")
	}

	repo := os.Args[1]

	tmpl := template.New("template.yml")
	tmpl.Delims("[[", "]]")
	tmpl.Funcs(template.FuncMap{
		"stripSuffix": func(in string) string {
			return strings.TrimSuffix(in, "_test.go")
		},
	})
	tmpl = template.Must(tmpl.ParseFS(templateFile, "*"))

	// list all itests.
	itests, err := filepath.Glob(filepath.Join(repo, "./itests/*_test.go"))
	if err != nil {
		panic(err)
	}

	// strip the dir from all entries.
	for i, f := range itests {
		itests[i] = filepath.Base(f)
	}

	// calculate the exclusion set of unit test directories to exclude because
	// they are already included in a grouped suite.
	var excluded = map[string]struct{}{}
	for _, ss := range groupedUnitTests {
		for _, s := range ss {
			e, err := filepath.Abs(filepath.Join(repo, s))
			if err != nil {
				panic(err)
			}
			excluded[e] = struct{}{}
		}
	}

	// all unit tests top-level dirs that are not itests, nor included in other suites.
	var rest = map[string]struct{}{}
	err = filepath.Walk(repo, func(path string, f os.FileInfo, err error) error {
		// include all tests that aren't in the itests directory.
		if strings.Contains(path, "itests") {
			return filepath.SkipDir
		}
		// exclude all tests included in other suites
		if f.IsDir() {
			if _, ok := excluded[path]; ok {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, "_test.go") {
			rel, err := filepath.Rel(repo, path)
			if err != nil {
				panic(err)
			}
			// take the first directory
			rest[strings.Split(rel, string(os.PathSeparator))[0]] = struct{}{}
		}
		return err
	})
	if err != nil {
		panic(err)
	}

	// add other directories to a 'rest' suite.
	for k := range rest {
		groupedUnitTests["unit-rest"] = append(groupedUnitTests["unit-rest"], k)
	}

	// map iteration guarantees no order, so sort the array in-place.
	sort.Strings(groupedUnitTests["unit-rest"])

	// form the input data.
	type data struct {
		ItestFiles []string
		UnitSuites map[string]string
	}
	in := data{
		ItestFiles: itests,
		UnitSuites: func() map[string]string {
			ret := make(map[string]string)
			for name, dirs := range groupedUnitTests {
				for i, d := range dirs {
					dirs[i] = fmt.Sprintf("./%s/...", d) // turn into package
				}
				ret[name] = strings.Join(dirs, " ")
			}
			return ret
		}(),
	}

	out, err := os.Create("./config.yml")
	if err != nil {
		panic(err)
	}
	defer out.Close()

	// execute the template.
	if err := tmpl.Execute(out, in); err != nil {
		panic(err)
	}
}
