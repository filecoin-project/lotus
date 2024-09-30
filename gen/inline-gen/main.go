package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/sync/errgroup"
)

const (
	stateGlobal = iota
	stateTemplate
	stateGen
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <dir> <data.json>\n", os.Args[0])
		os.Exit(1)
	}
	rootDir := os.Args[1]
	jsonDataPath := os.Args[2]

	data, err := readDataFromFile(jsonDataPath)
	if err != nil {
		fmt.Printf("Failed to read JSON data from %s: %v\n", jsonDataPath, err)
		os.Exit(1)
	}

	var g errgroup.Group
	if err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() || filepath.Ext(path) != ".go":
			return nil
		default:
			g.Go(func() error {
				return processFile(path, data)
			})
			return nil
		}
	}); err != nil {
		fmt.Printf("Failed to walk path %s: %v\n", rootDir, err)
		os.Exit(1)
	}
	if err := g.Wait(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("All inline-gen files processed successfully.")
}

// readDataFromFile reads the JSON file and unmarshals it into a map.
func readDataFromFile(filePath string) (map[string]any, error) {
	db, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(db, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// processFile processes the Go file and applies inline template generation.
func processFile(path string, data map[string]any) error {
	fb, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(fb), "\n")
	outLines := make([]string, 0, len(lines))
	var templateLines []string
	state := stateGlobal
	rewrite := false

	// Process each line in the file
	for i, line := range lines {
		ln := i + 1
		switch state {
		case stateGlobal:
			outLines = append(outLines, line)
			if strings.TrimSpace(line) == `/* inline-gen template` {
				state = stateTemplate
				fmt.Printf("template section start %s:%d\n", path, ln)
			}
		case stateTemplate:
			outLines = append(outLines, line) // output all template lines

			if strings.TrimSpace(line) == `/* inline-gen start */` {
				state = stateGen
				fmt.Printf("generated section start %s:%d\n", path, ln)
				continue
			}
			templateLines = append(templateLines, line)
		case stateGen:
			if strings.TrimSpace(line) != `/* inline-gen end */` {
				continue
			}
			fmt.Printf("generated section end %s:%d\n", path, ln)

			state = stateGlobal
			rewrite = true

			// Generate template output
			generated, err := generateFromTemplate(templateLines, data, path, ln)
			if err != nil {
				return err
			}
			outLines = append(outLines, generated...)
			outLines = append(outLines, line) // Append end line
			templateLines = nil
		}
	}

	// If the file was modified, rewrite it
	if rewrite {
		fmt.Printf("write %s\n", path)
		return writeFile(path, outLines)
	}
	return nil
}

// generateFromTemplate parses and executes a template from lines
func generateFromTemplate(templateLines []string, data map[string]any, path string, ln int) ([]string, error) {
	tpl, err := template.New("").Funcs(template.FuncMap{
		"import": func(v float64) string {
			if v == 0 {
				return "/"
			}
			return fmt.Sprintf("/v%d/", int(v))
		},
		"add": func(a, b float64) float64 {
			return a + b
		},
	}).Parse(strings.Join(templateLines, "\n"))
	if err != nil {
		return nil, fmt.Errorf("%s:%d: parsing template: %w", path, ln, err)
	}

	var b bytes.Buffer
	err = tpl.Execute(&b, data)
	if err != nil {
		return nil, fmt.Errorf("%s:%d: executing template: %w", path, ln, err)
	}

	return strings.Split(b.String(), "\n"), nil
}

// writeFile writes the modified file back to disk
func writeFile(path string, lines []string) error {
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0664)
}
