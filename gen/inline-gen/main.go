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
    json.Unmarshal(db, &data)

    filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
        if info.IsDir() {
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

        inTemplate, inGen := false, false
        templateLines := make([]string, 0)

        for _, line := range lines {
            if strings.TrimSpace(line) == `/* inline-gen template` {
                inTemplate = true
                outLines = append(outLines, line)
                continue
            }
            if strings.TrimSpace(line) == `/* inline-gen start */` {
                inGen = true
                continue
            }
            if strings.TrimSpace(line) == `/* inline-gen end */` {
                inGen = false
            }

            if inTemplate {
                templateLines = append(templateLines, line)
                outLines = append(outLines, line)
            }
            if !inGen {
                outLines = append(outLines, line)
            }

            if !inTemplate && !inGen {
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
                    fmt.Printf("%s: parsing template: %s\n", path, err)
                    os.Exit(1)
                }

                var b bytes.Buffer
                err = tpl.Execute(&b, data)
                if err != nil {
                    fmt.Printf("%s: executing template: %s\n", path, err)
                    os.Exit(1)
                }

                outLines = append(outLines, strings.Split(b.String(), "\n")...)
                outLines = append(outLines, line)
                templateLines = nil
            }
        }

        ioutil.WriteFile(path, []byte(strings.Join(outLines, "\n")), 0644)
        return nil
    })
}

