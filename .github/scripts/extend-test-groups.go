package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

type group map[string]interface{}

func main() {
	var outputFileName string
	var defaultGroupsConfig string
	flag.StringVar(&outputFileName, "output", "", "Output file name to write the matrix")
	flag.StringVar(&defaultGroupsConfig, "config", "[]", "Default groups configuration")
	flag.Parse()

	var defaultGroups []group
	err := json.Unmarshal([]byte(defaultGroupsConfig), &defaultGroups)
	if err != nil {
		panic(fmt.Errorf("error parsing default groups configuration: %v", err))
	}

	coveredPackages := []string{}
	groupsByName := make(map[string]group)
	for _, g := range defaultGroups {
		name, ok := g["name"].(string)
		if !ok {
			panic(fmt.Errorf("error parsing group name: %v", g))
		}
		packagesOpt, ok := g["packages"]
		if ok {
			packages, ok := packagesOpt.([]interface{})
			if !ok {
				panic(fmt.Errorf("error parsing packages: %v", packagesOpt))
			}
			ps := []string{}
			for _, p := range packages {
				p, ok := p.(string)
				if !ok {
					panic(fmt.Errorf("error parsing package: %v", p))
				}
				ps = append(ps, p)
				p = strings.TrimSuffix(p, "/...")
				if !slices.Contains(coveredPackages, p) {
					coveredPackages = append(coveredPackages, p)
				}
			}
			g["packages"] = ps
		}
		groupsByName[name] = g
	}

	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), "_test.go") {
			dir := filepath.Dir(path)
			rootDir := strings.Split(dir, string(os.PathSeparator))[0]

			if slices.Contains(coveredPackages, rootDir) {
				return nil
			}

			name := "unit-rest"
			if rootDir == "itests" {
				name = "itests-" + strings.TrimSuffix(info.Name(), "_test.go")
			}

			g, ok := groupsByName[name]
			if !ok {
				g = group{}
			}

			if rootDir == "itests" {
				g["packages"] = []string{path}
			} else {
				packagesOpt := g["packages"]
				if packagesOpt == nil {
					packagesOpt = []string{}
				}
				packages, ok := packagesOpt.([]string)
				if !ok {
					panic(fmt.Errorf("error parsing packages: %v", packagesOpt))
					return nil
				}
				if !slices.Contains(packages, fmt.Sprintf("%s/...", rootDir)) {
					packages = append(packages, fmt.Sprintf("%s/...", rootDir))
					sort.Strings(packages)
					g["packages"] = packages
				}
			}
			groupsByName[name] = g
		}
		return nil
	})

	if err != nil {
		panic(fmt.Errorf("error walking through files: %v", err))
	}

	groups := []group{}
	for name, group := range groupsByName {
		group["name"] = name
		groups = append(groups, group)
	}

	jsonRepresentation, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		panic(fmt.Errorf("error generating JSON: %v", err))
	}

	fmt.Println(string(jsonRepresentation))

	if outputFileName != "" {
		f, err := os.OpenFile(outputFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(fmt.Errorf("error opening the file: %v", err))
		}
		_, err = f.Write([]byte(fmt.Sprintf("groups=%s\n", jsonRepresentation)))
		if err != nil {
			panic(fmt.Errorf("error writing to file: %v", err))
		}
		err = f.Close()
		if err != nil {
			panic(fmt.Errorf("error closing the file: %v", err))
		}
	}
}
