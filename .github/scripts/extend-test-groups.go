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
		fmt.Printf("Error parsing default groups configuration: %v\n", err)
		return
	}

	coveredPackages := []string{}
	groupsByName := make(map[string]group)
	for _, g := range defaultGroups {
		nameOpt, ok := g["name"]
		if !ok {
			fmt.Printf("Error parsing group name: %v\n", g)
			return
		}
		name, ok := nameOpt.(string)
		if !ok {
			fmt.Printf("Error parsing group name: %v\n", nameOpt)
			return
		}
		packagesOpt, ok := g["packages"]
		if ok {
			packages, ok := packagesOpt.([]string)
			if !ok {
				fmt.Printf("Error parsing packages: %v\n", packagesOpt)
				return
			}
			for _, p := range packages {
				if !slices.Contains(coveredPackages, strings.TrimSuffix(p, "/...")) {
					coveredPackages = append(coveredPackages, strings.TrimSuffix(p, "/..."))
				}
			}
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
					fmt.Printf("Error parsing packages: %v\n", packagesOpt)
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
		fmt.Printf("Error walking through files: %v\n", err)
		return
	}

	groups := []group{}
	for name, group := range groupsByName {
		group["name"] = name
		groups = append(groups, group)
	}

	jsonRepresentation, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		fmt.Printf("Error generating JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonRepresentation))

	if outputFileName != "" {
		f, err := os.OpenFile(outputFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("Error opening the file: %v\n", err)
			return
		}
		_, err = f.Write([]byte(fmt.Sprintf("groups=%s\n", jsonRepresentation)))
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
			return
		}
		err = f.Close()
		if err != nil {
			fmt.Printf("Error closing the file: %v\n", err)
			return
		}
	}
}
