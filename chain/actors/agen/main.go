package main

import (
	"bytes"
	"fmt"
	"golang.org/x/xerrors"
	"io/ioutil"
	"path/filepath"
	"text/template"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		return
	}
}

func run() error {
	versions := []int{0, 2, 3, 4}

	versionImports := map[int]string{
		0: "/",
		2: "/v2/",
		3: "/v3/",
		4: "/v4/",
	}

	actors := map[string][]int{
		"account": versions,
	}

	for act, versions := range actors {
		actDir := filepath.Join("chain/actors/builtin", act)

		{
			af, err := ioutil.ReadFile(filepath.Join(actDir, "state.go.template"))
			if err != nil {
				return xerrors.Errorf("loading state adapter template: %w", err)
			}

			for _, version := range versions {
				tpl := template.Must(template.New("").Funcs(template.FuncMap{}).Parse(string(af)))

				var b bytes.Buffer

				err := tpl.Execute(&b, map[string]interface{}{
					"v": version,
					"import": versionImports[version],
				})
				if err != nil {
					return err
				}

				if err := ioutil.WriteFile(filepath.Join(actDir, fmt.Sprintf("v%d.go", version)), b.Bytes(), 0666); err != nil {
					return err
				}
			}
		}

		{
			af, err := ioutil.ReadFile(filepath.Join(actDir, "actor.go.template"))
			if err != nil {
				return xerrors.Errorf("loading actor template: %w", err)
			}

			tpl := template.Must(template.New("").Funcs(template.FuncMap{
				"import": func(v int) string { return versionImports[v] },
			}).Parse(string(af)))

			var b bytes.Buffer

			err = tpl.Execute(&b, map[string]interface{}{
				"versions": versions,
			})
			if err != nil {
				return err
			}

			if err := ioutil.WriteFile(filepath.Join(actDir, fmt.Sprintf("%s.go", act)), b.Bytes(), 0666); err != nil {
				return err
			}
		}
	}

	return nil
}
