package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/mod/semver"

	"github.com/filecoin-project/lotus/build"
)

var _tags []string

func getTags() []string {
	if _tags == nil {
		output, err := exec.Command("git", "tag").Output()
		if err != nil {
			log.Fatal(err)
		}
		_tags = strings.Split(string(output), "\n")
	}
	return _tags
}

func isPrerelease(version string) bool {
	return semver.Prerelease("v"+version) != ""
}

func isLatest(name, version string) bool {
	if isPrerelease(version) {
		return false
	}
	prefix := getPrefix(name)
	tags := getTags()
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			v := strings.TrimPrefix(t, prefix)
			if !isPrerelease(v) {
				if semver.Compare("v"+v, "v"+version) > 0 {
					return false
				}
			}
		}
	}
	return true
}

func getPrevious(name, version string) string {
	prerelease := isPrerelease(version)
	prefix := getPrefix(name)
	tags := getTags()
	previous := ""
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			v := strings.TrimPrefix(t, prefix)
			if prerelease || !isPrerelease(v) {
				if semver.Compare("v"+v, "v"+version) < 0 {
					if previous == "" || semver.Compare("v"+v, "v"+previous) > 0 {
						previous = v
					}
				}
			}
		}
	}
	if previous == "" {
		return ""
	}
	return prefix + previous
}

func getBinaries(name string) []string {
	if name == "node" {
		return []string{"lotus"}
	}
	if name == "miner" {
		return []string{"lotus-miner", "lotus-worker"}
	}
	return nil
}

func isReleased(tag string) bool {
	tags := getTags()
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func getPrefix(name string) string {
	if name == "node" {
		return "v"
	}
	return name + "/v"
}

type project struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Tag        string   `json:"tag"`
	Previous   string   `json:"previous"`
	Latest     bool     `json:"latest"`
	Prerelease bool     `json:"prerelease"`
	Released   bool     `json:"released"`
	Binaries   []string `json:"binaries"`
}

func getProject(name, version string) project {
	tag := getPrefix(name) + version
	return project{
		Name:       name,
		Version:    version,
		Tag:        getPrefix(name) + version,
		Previous:   getPrevious(name, version),
		Latest:     isLatest(name, version),
		Prerelease: isPrerelease(version),
		Released:   isReleased(tag),
		Binaries:   getBinaries(name),
	}
}

func main() {
	app := &cli.App{
		Name:  "release",
		Usage: "Lotus release tool",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Format output as JSON",
			},
		},
		Before: func(c *cli.Context) error {
			if c.Bool("json") {
				log.SetFormatter(&log.JSONFormatter{})
			} else {
				log.SetFormatter(&log.TextFormatter{
					TimestampFormat: "2006-01-02 15:04:05",
					FullTimestamp:   true,
				})
			}
			log.SetOutput(os.Stdout)
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "list-projects",
				Usage: "List all projects",
				Action: func(c *cli.Context) error {
					projects := []project{
						getProject("node", build.NodeBuildVersion),
						getProject("miner", build.MinerBuildVersion),
					}
					b, err := json.MarshalIndent(projects, "", "  ")
					if err != nil {
						log.Fatal(err)
					}
					log.Info(string(b))
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
