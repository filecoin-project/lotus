package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	stateGlobal = iota
	stateTemplate
	stateGen
)

func main() {
	db, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(db, &data); err != nil {
		panic(err)
	}

	err = filepath.WalkDir(os.Args[1], func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		fb, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		lines := strings.Split(string(fb), "\n")

		outLines := make([]string, 0, len(lines))
		var templateLines []string

		state := stateGlobal

		rewrite := false

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
					fmt.Printf("%s:%d: parsing template: %s\n", path, ln, err)
					os.Exit(1)
				}

				var b bytes.Buffer
				err = tpl.Execute(&b, data)
				if err != nil {
					fmt.Printf("%s:%d: executing template: %s\n", path, ln, err)
					os.Exit(1)
				}

				outLines = append(outLines, strings.Split(b.String(), "\n")...)
				outLines = append(outLines, line)
				templateLines = nil
			}
		}

		if rewrite {
			fmt.Printf("write %s\n", path)
			if err := ioutil.WriteFile(path, []byte(strings.Join(outLines, "\n")), 0664); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		panic(err)
	}
}
