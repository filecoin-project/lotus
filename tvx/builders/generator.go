package builders

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/filecoin-project/oni/tvx/schema"
)

// Generator is a batch generator and organizer of test vectors.
//
// Test vector scripts are simple programs (main function). Test vector scripts
// can delegate to the Generator to handle the execution, reporting and capture
// of emitted test vectors into files.
//
// Generator supports the following CLI flags:
//
//  -o <directory>
//		directory where test vector JSON files will be saved; if omitted,
//		vectors will be written to stdout.
//
//  -f <regex>
//		regex filter to select a subset of vectors to execute; matched against
//	 	the vector's ID.
//
// Scripts can bundle test vectors into "groups". The generator will execute
// each group in parallel, and will write each vector in a file:
// <output_dir>/<group>--<vector_id>.json
type Generator struct {
	OutputPath string
	Filter     *regexp.Regexp

	wg sync.WaitGroup
}

type MessageVectorGenItem struct {
	Metadata *schema.Metadata
	Func     func(*Builder)
}

func NewGenerator() *Generator {
	// Consume CLI parameters.
	var (
		outputDir = flag.String("o", "", "directory where test vector JSON files will be saved; if omitted, vectors will be written to stdout")
		filter    = flag.String("f", "", "regex filter to select a subset of vectors to execute; matched against the vector's ID")
	)

	flag.Parse()

	ret := new(Generator)

	// If output directory is provided, we ensure it exists, or create it.
	// Else, we'll output to stdout.
	if dir := *outputDir; dir != "" {
		err := ensureDirectory(dir)
		if err != nil {
			log.Fatal(err)
		}
		ret.OutputPath = dir
	}

	// If a filter has been provided, compile it into a regex.
	if *filter != "" {
		exp, err := regexp.Compile(*filter)
		if err != nil {
			log.Fatalf("supplied regex %s is invalid: %s", *filter, err)
		}
		ret.Filter = exp
	}

	return ret
}

func (g *Generator) Wait() {
	g.wg.Wait()
}

func (g *Generator) MessageVectorGroup(group string, vectors ...*MessageVectorGenItem) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()

		var wg sync.WaitGroup
		for _, item := range vectors {
			if id := item.Metadata.ID; g.Filter != nil && !g.Filter.MatchString(id) {
				log.Printf("skipping %s", id)
				continue
			}

			var w io.Writer
			if g.OutputPath == "" {
				w = os.Stdout
			} else {
				file := filepath.Join(g.OutputPath, fmt.Sprintf("%s--%s.json", group, item.Metadata.ID))
				out, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					log.Printf("failed to write to file %s: %s", file, err)
					return
				}
				w = out
			}

			wg.Add(1)
			go func(item *MessageVectorGenItem) {
				g.generateOne(w, item, w != os.Stdout)
				wg.Done()
			}(item)
		}

		wg.Wait()
	}()
}

func (g *Generator) generateOne(w io.Writer, b *MessageVectorGenItem, indent bool) {
	log.Printf("generating test vector: %s", b.Metadata.ID)

	vector := MessageVector(b.Metadata)

	// TODO: currently if an assertion fails, we call os.Exit(1), which
	//  aborts all ongoing vector generations. The Asserter should
	//  call runtime.Goexit() instead so only that goroutine is
	//  cancelled. The assertion error must bubble up somehow.
	b.Func(vector)

	buf := new(bytes.Buffer)
	vector.Finish(buf)

	final := buf
	if indent {
		// reparse and reindent.
		final = new(bytes.Buffer)
		if err := json.Indent(final, buf.Bytes(), "", "\t"); err != nil {
			log.Printf("failed to indent json: %s", err)
		}
	}

	n, err := w.Write(final.Bytes())
	if err != nil {
		log.Printf("failed to write to output: %s", err)
		return
	}

	log.Printf("generated test vector: %s (size: %d bytes)", b.Metadata.ID, n)
}

// ensureDirectory checks if the provided path is a directory. If yes, it
// returns nil. If the path doesn't exist, it creates the directory and
// returns nil. If the path is not a directory, or another error occurs, an
// error is returned.
func ensureDirectory(path string) error {
	switch stat, err := os.Stat(path); {
	case os.IsNotExist(err):
		// create directory.
		log.Printf("creating directory %s", path)
		err := os.MkdirAll(path, 0700)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %s", path, err)
		}

	case err == nil && !stat.IsDir():
		return fmt.Errorf("path %s exists, but it's not a directory", path)

	case err != nil:
		return fmt.Errorf("failed to stat directory %s: %w", path, err)
	}
	return nil
}
