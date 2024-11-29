package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	masterminds "github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v66/github"
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
			{
				Name:  "create-issue",
				Usage: "Create a new release issue from the template",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "type",
						Usage:    "What's the type of the release? (node, miner, both)",
						Value:    "both",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "tag",
						Usage:    "What's the tag of the release? (e.g. 1.30.1)",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "level",
						Usage:    "What's the level of the release? (major, minor, patch)",
						Value:    "patch",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "network-upgrade",
						Usage:    "What's the version of the network upgrade this release is related to? (e.g. 23)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "discussion-link",
						Usage:    "What's a link to the GitHub Discussions topic for the network upgrade?",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "changelog-link",
						Usage:    "What's a link to the Lotus CHANGELOG entry for the network upgrade?",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "rc1-date",
						Usage:    "What's the expected shipping date for RC1 (YYYY-MM-DD)?",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "rc1-precision",
						Usage:    "How precise is the RC1 date? (day, week)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "rc1-confidence",
						Usage:    "How confident is the RC1 date? (estimated, confirmed)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "stable-date",
						Usage:    "What's the expected shipping date for the stable release (YYYY-MM-DD)?",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "stable-precision",
						Usage:    "How precise is the stable release date? (day, week)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "stable-confidence",
						Usage:    "How confident is the stable release date? (estimated, confirmed)",
						Required: false,
					},
				},
				Action: func(c *cli.Context) error {
					// Read the flag values
					releaseType := c.String("type")
					releaseTag := c.String("tag")
					releaseLevel := c.String("level")
					networkUpgrade := c.String("network-upgrade")
					discussionLink := c.String("discussion-link")
					changelogLink := c.String("changelog-link")
					rc1Date := c.String("rc1-date")
					rc1Precision := c.String("rc1-precision")
					rc1Confidence := c.String("rc1-confidence")
					stableDate := c.String("stable-date")
					stablePrecision := c.String("stable-precision")
					stableConfidence := c.String("stable-confidence")

					// Validate the flag values
					if releaseType != "node" && releaseType != "miner" && releaseType != "both" {
						return fmt.Errorf("invalid value for the 'type' flag. Allowed values are 'node', 'miner', and 'both'")
					}
					releaseVersion, err := masterminds.StrictNewVersion(releaseTag)
					if err != nil {
						return fmt.Errorf("invalid value for the 'tag' flag. Must be a valid semantic version (e.g. 1.30.1)")
					}
					if releaseLevel != "major" && releaseLevel != "minor" && releaseLevel != "patch" {
						return fmt.Errorf("invalid value for the 'level' flag. Allowed values are 'major', 'minor', and 'patch'")
					}
					if networkUpgrade != "" {
						_, err := strconv.ParseUint(networkUpgrade, 10, 64)
						if err != nil {
							return fmt.Errorf("invalid value for the 'network-upgrade' flag. Must be a valid uint (e.g. 23)")
						}
						if discussionLink != "" {
							_, err := url.ParseRequestURI(discussionLink)
							if err != nil {
								return fmt.Errorf("invalid value for the 'discussion-link' flag. Must be a valid URL")
							}
						}
						if changelogLink != "" {
							_, err := url.ParseRequestURI(changelogLink)
							if err != nil {
								return fmt.Errorf("invalid value for the 'changelog-link' flag. Must be a valid URL")
							}
						}
					}
					if rc1Date != "" {
						_, err := time.Parse("2006-01-02", rc1Date)
						if err != nil {
							return fmt.Errorf("invalid value for the 'rc1-date' flag. Must be a valid date (YYYY-MM-DD)")
						}
						if rc1Precision != "" {
							if rc1Precision != "day" && rc1Precision != "week" {
								return fmt.Errorf("invalid value for the 'rc1-precision' flag. Allowed values are 'day' and 'week'")
							}
						}
						if rc1Confidence != "" {
							if rc1Confidence != "estimated" && rc1Confidence != "confirmed" {
								return fmt.Errorf("invalid value for the 'rc1-confidence' flag. Allowed values are 'estimated' and 'confirmed'")
							}
						}
					}
					if stableDate != "" {
						_, err := time.Parse("2006-01-02", stableDate)
						if err != nil {
							return fmt.Errorf("invalid value for the 'stable-date' flag. Must be a valid date (YYYY-MM-DD)")
						}
						if stablePrecision != "" {
							if stablePrecision != "day" && stablePrecision != "week" {
								return fmt.Errorf("invalid value for the 'stable-precision' flag. Allowed values are 'day' and 'week'")
							}
						}
						if stableConfidence != "" {
							if stableConfidence != "estimated" && stableConfidence != "confirmed" {
								return fmt.Errorf("invalid value for the 'stable-confidence' flag. Allowed values are 'estimated' and 'confirmed'")
							}
						}
					}

					// Prepare template data
					data := make(map[string]interface{})
					data["Type"] = releaseType
					data["Tag"] = releaseVersion.String()
					data["NextTag"] = releaseVersion.IncPatch().String()
					data["Level"] = releaseLevel
					data["NetworkUpgrade"] = networkUpgrade
					data["NetworkUpgradeDiscussionLink"] = discussionLink
					data["NetworkUpgradeChangelogEntryLink"] = changelogLink
					data["ReleaseCandidateDate"] = rc1Date
					data["ReleaseCandidatePrecision"] = rc1Precision
					data["ReleaseCandidateConfidence"] = rc1Confidence
					data["StableDate"] = stableDate
					data["StablePrecision"] = stablePrecision
					data["StableConfidence"] = stableConfidence

					// Render the issue template
					issueTemplate, err := os.ReadFile("documentation/misc/RELEASE_ISSUE_TEMPLATE.md")
					if err != nil {
						return fmt.Errorf("failed to read issue template: %w", err)
					}
					tmpl, err := template.New("issue").Parse(string(issueTemplate))
					if err != nil {
						return fmt.Errorf("failed to parse issue template: %w", err)
					}
					var issueBodyBuffer bytes.Buffer
					err = tmpl.Execute(&issueBodyBuffer, data)
					if err != nil {
						return fmt.Errorf("failed to execute issue template: %w", err)
					}

					// Prepare issue creation options
					issueTitle := fmt.Sprintf("Lotus %s v%s Release", releaseType, releaseTag)
					issueBody := issueBodyBuffer.String()

					// Set up the GitHub client
					client := github.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

					// Check if the issue already exists
					issues, _, err := client.Search.Issues(context.Background(), issueTitle+" in:title state:open", &github.SearchOptions{})
					if err != nil {
						return fmt.Errorf("failed to list issues: %w", err)
					}
					if issues.GetTotal() > 0 {
						return fmt.Errorf("issue already exists: %s", issues.Issues[0].GetHTMLURL())
					}

					// Create the issue
					issue, _, err := client.Issues.Create(context.Background(), "filecoin", "lotus", &github.IssueRequest{
						Title: &issueTitle,
						Body:  &issueBody,
						Labels: &[]string{
							"tpm",
						},
					})
					if err != nil {
						return fmt.Errorf("failed to create issue: %w", err)
					}
					fmt.Println("Issue created:", issue.GetHTMLURL())

					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
