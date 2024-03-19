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
	var defaultGroupConfig string
	var groupsConfig string
	var skip bool
	flag.StringVar(&outputFileName, "output", "", "Output file name to write the matrix")
	flag.StringVar(&defaultGroupConfig, "defaults", "{}", "Default group configuration")
	flag.StringVar(&groupsConfig, "groups", "[]", "Groups configuration")
	flag.BoolVar(&skip, "skip", false, "Skip groups by default")
	flag.Parse()

	var defaultGroup group
	err := json.Unmarshal([]byte(defaultGroupConfig), &defaultGroup)
	if err != nil {
		panic(fmt.Errorf("error parsing default group configuration: %v", err))
	}

	var groups []group
	err = json.Unmarshal([]byte(groupsConfig), &groups)
	if err != nil {
		panic(fmt.Errorf("error parsing groups configuration: %v", err))
	}

	coveredRootDirs := []string{}
	groupsByName := make(map[string]group)
	for _, g := range groups {
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
				p = strings.TrimPrefix(p, "./")
				p = strings.TrimSuffix(p, "/...")
				if !slices.Contains(coveredRootDirs, p) {
					coveredRootDirs = append(coveredRootDirs, p)
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

			if slices.Contains(coveredRootDirs, rootDir) {
				return nil
			}

			name := "unit-rest"
			if rootDir == "itests" {
				name = "itest-" + strings.TrimSuffix(info.Name(), "_test.go")
			}

			g, ok := groupsByName[name]
			if !ok {
				g = group{}
			}

			if rootDir == "itests" {
				g["packages"] = []string{fmt.Sprintf("./%s", path)}
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
				p := fmt.Sprintf("./%s/...", rootDir)
				if !slices.Contains(packages, p) {
					packages = append(packages, p)
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

	groups = []group{}
	for name, group := range groupsByName {
		group["name"] = name
		// Iterate over keys of the default group and set them if they are not set in the group
		for k, v := range defaultGroup {
			if _, ok := group[k]; !ok {
				group[k] = v
			}
		}
		s, ok := group["skip"].(bool)
		if !skip || (ok && !s) {
			groups = append(groups, group)
		}
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
		_, err = f.Write([]byte(fmt.Sprintf("groups<<EOF\n%s\nEOF\n", jsonRepresentation)))
		if err != nil {
			panic(fmt.Errorf("error writing to file: %v", err))
		}
		err = f.Close()
		if err != nil {
			panic(fmt.Errorf("error closing the file: %v", err))
		}
	}
}
