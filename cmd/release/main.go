package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"

	masterminds "github.com/Masterminds/semver/v3"
	"github.com/Masterminds/sprig/v3"
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
	return slices.Contains(tags, tag)
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

const releaseDateStringPattern = `^(Week of )?\d{4}-\d{2}-\d{2}( \(estimate\))?$`

const (
	releaseFlowAuto = "auto"
	releaseFlowRC   = "rc"
	releaseFlowNoRC = "no-rc"
)

var (
	// commentWrappedActionRegexp matches a template action wrapped in an HTML comment, e.g. `<!--{{if .Foo}}-->`.
	commentWrappedActionRegexp = regexp.MustCompile(`<!--[ \t]*(\{\{.*?\}\})[ \t]*-->`)
	// controlLineRegexp matches a line that, once unwrapped, holds nothing but template actions.
	controlLineRegexp = regexp.MustCompile(`^[ \t]*(\{\{.*\}\})[ \t]*$`)
)

// findMalformedActionWrapper returns the first line holding a template action whose HTML comment
// wrapper commentWrappedActionRegexp can't match, or "" when there is none.
func findMalformedActionWrapper(templateSource string) string {
	for _, line := range strings.Split(templateSource, "\n") {
		if !strings.Contains(line, "{{") {
			continue
		}
		if strings.Contains(commentWrappedActionRegexp.ReplaceAllString(line, "$1"), "<!--") {
			return line
		}
	}
	return ""
}

// stripControlLines appends the actions of every control-only line to the end of the preceding
// line, so the line they occupied contributes no newline of its own to the rendered issue.
func stripControlLines(templateSource string) string {
	lines := strings.Split(templateSource, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if match := controlLineRegexp.FindStringSubmatch(line); match != nil && len(kept) > 0 {
			kept[len(kept)-1] += match[1]
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
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
					&cli.BoolFlag{
						Name:  "create-on-github",
						Usage: "Whether to create the issue on github rather than print the issue content. $GITHUB_TOKEN must be set.",
						Value: false,
					},
					&cli.StringFlag{
						Name:     "type",
						Usage:    "What's the type of the release? (Options: node, miner, both)",
						Value:    "both",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "tag",
						Usage:    "What's the tag of the release? (e.g., 1.30.1)",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "level",
						Usage:    "What's the level of the release? (Options: minor, patch)",
						Value:    "patch",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "network-upgrade",
						Usage:    "What's the version of the network upgrade this release is related to? (e.g., 25)",
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
						Name:     "release-flow",
						Usage:    "Which release flow should the issue use? (Options: auto, rc, no-rc). auto uses rc for network upgrades and no-rc otherwise.",
						Value:    releaseFlowAuto,
						Required: false,
					},
					&cli.StringFlag{
						Name:     "rc1-date",
						Usage:    fmt.Sprintf("What's the expected shipping date for RC1? (Pattern: '%s')", releaseDateStringPattern),
						Value:    "TBD",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "stable-date",
						Usage:    fmt.Sprintf("What's the expected shipping date for the stable release? (Pattern: '%s'))", releaseDateStringPattern),
						Value:    "TBD",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "repo",
						Usage:    "Which full repository name (i.e., OWNER/REPOSITORY) to create the issue under.",
						Value:    "filecoin-project/lotus",
						Required: false,
					},
				},
				Action: func(c *cli.Context) error {
					lotusReleaseCliString := strings.Join(os.Args, " ")

					// Read and validate the flag values
					createOnGitHub := c.Bool("create-on-github")

					releaseType := c.String("type")
					switch releaseType {
					case "node":
						releaseType = "Node"
					case "miner":
						releaseType = "Miner"
					case "both":
						releaseType = "Node and Miner"
					default:
						return fmt.Errorf("invalid value for the 'type' flag. Allowed values are 'node', 'miner', and 'both'")
					}

					releaseTag := c.String("tag")
					releaseVersion, err := masterminds.StrictNewVersion(releaseTag)
					if err != nil {
						return fmt.Errorf("invalid value for the 'tag' flag. Must be a valid semantic version (e.g. 1.30.1)")
					}

					releaseLevel := c.String("level")
					if releaseLevel != "minor" && releaseLevel != "patch" {
						return fmt.Errorf("invalid value for the 'level' flag. Allowed values are 'minor' and 'patch'")
					}

					networkUpgrade := c.String("network-upgrade")
					discussionLink := c.String("discussion-link")
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
					}

					requestedReleaseFlow := c.String("release-flow")
					switch requestedReleaseFlow {
					case releaseFlowAuto, releaseFlowRC, releaseFlowNoRC:
					default:
						return fmt.Errorf("invalid value for the 'release-flow' flag. Allowed values are 'auto', 'rc', and 'no-rc'")
					}
					releaseFlow := requestedReleaseFlow
					if releaseFlow == releaseFlowAuto {
						if networkUpgrade != "" {
							releaseFlow = releaseFlowRC
						} else {
							releaseFlow = releaseFlowNoRC
						}
					}
					if releaseFlow == releaseFlowNoRC && networkUpgrade != "" {
						return fmt.Errorf("invalid value for the 'release-flow' flag. no-rc releases are not allowed for network upgrades; use 'auto' or 'rc'")
					}

					changelogLink := c.String("changelog-link")
					if changelogLink != "" {
						_, err := url.ParseRequestURI(changelogLink)
						if err != nil {
							return fmt.Errorf("invalid value for the 'changelog-link' flag. Must be a valid URL")
						}
					}

					releaseDateStringRegexp := regexp.MustCompile(releaseDateStringPattern)

					rc1Date := c.String("rc1-date")
					if releaseFlow == releaseFlowNoRC {
						rc1Date = "n/a"
					} else if rc1Date != "TBD" {
						matches := releaseDateStringRegexp.FindStringSubmatch(rc1Date)
						if matches == nil {
							return fmt.Errorf("rc1-date must be of form %s", releaseDateStringPattern)
						}
					}

					stableDate := c.String("stable-date")
					if stableDate != "TBD" {
						matches := releaseDateStringRegexp.FindStringSubmatch(stableDate)
						if matches == nil {
							return fmt.Errorf("stable-date must be of form %s", releaseDateStringPattern)
						}
					}

					repoFullName := c.String("repo")
					repoRegexp := regexp.MustCompile(`^([^/]+)/([^/]+)$`)
					matches := repoRegexp.FindStringSubmatch(repoFullName)
					if matches == nil {
						return fmt.Errorf("invalid repository name format. Must be 'owner/repo'")
					}
					repoOwner := matches[1]
					repoName := matches[2]

					firstReleaseTarget := "Stable Release"
					releaseTargets := []string{"Stable Release"}
					if releaseFlow == releaseFlowRC {
						firstReleaseTarget = "rc1"
						releaseTargets = []string{"rc1", "rcX", "Stable Release"}
					}

					// Prepare template data
					data := map[string]any{
						"ContentGeneratedWithLotusReleaseCli": true,
						"LotusReleaseCliString":               lotusReleaseCliString,
						"Type":                                releaseType,
						"Tag":                                 releaseVersion.String(),
						"NextTag":                             releaseVersion.IncPatch().String(),
						"Level":                               releaseLevel,
						"RequestedReleaseFlow":                requestedReleaseFlow,
						"ReleaseFlow":                         releaseFlow,
						"NoRCRelease":                         releaseFlow == releaseFlowNoRC,
						"RCRelease":                           releaseFlow == releaseFlowRC,
						"FirstReleaseTarget":                  firstReleaseTarget,
						"ReleaseTargets":                      releaseTargets,
						"NetworkUpgrade":                      networkUpgrade,
						"NetworkUpgradeDiscussionLink":        discussionLink,
						"NetworkUpgradeChangelogEntryLink":    changelogLink,
						"RC1DateString":                       rc1Date,
						"StableDateString":                    stableDate,
					}

					// Render the issue template
					issueTemplate, err := os.ReadFile("documentation/misc/RELEASE_ISSUE_TEMPLATE.md")
					if err != nil {
						return fmt.Errorf("failed to read issue template: %w", err)
					}
					// A wrapper that doesn't match, e.g. `<!--{{if .Foo}})-->`, would leave a stray `<!--`
					// in the issue body rather than failing, so reject it up front.
					if line := findMalformedActionWrapper(string(issueTemplate)); line != "" {
						return fmt.Errorf("malformed comment-wrapped template action in issue template: %s", line)
					}

					// Control flow in the template lives inside HTML comments so that the template file
					// parses as clean markdown on its own.  That means Go's {{- -}} trim markers can't be
					// used to swallow the newline a control statement leaves behind, since the whitespace
					// sits outside the action but inside the comment.  Strip the comment wrappers here
					// instead: an action that had a line to itself no longer contributes a newline, while
					// actions embedded in a content line are unaffected.  The result is that whitespace in
					// the generated issue matches the whitespace in the template.
					templateSource := commentWrappedActionRegexp.ReplaceAllString(string(issueTemplate), "$1")
					templateSource = stripControlLines(templateSource)

					// Sprig used for String contains and Lists
					tmpl, err := template.New("issue").Funcs(sprig.FuncMap()).Parse(templateSource)
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
					if networkUpgrade != "" {
						issueTitle += fmt.Sprintf(" (nv%s)", networkUpgrade)
					}
					issueBody := issueBodyBuffer.String()

					if !createOnGitHub {
						// Create the URL-encoded parameters
						params := url.Values{}
						params.Add("title", issueTitle)
						params.Add("body", issueBody)
						params.Add("labels", "tpm")

						// Construct the URL
						issueURL := fmt.Sprintf("https://github.com/%s/issues/new?%s", repoFullName, params.Encode())

						debugFormat := `
Issue Details:
=============
Title: %s

Body:
-----
%s

URL to create issue:
-------------------
%s
`
						_, _ = fmt.Fprintf(c.App.Writer, debugFormat, issueTitle, issueBody, issueURL)
					} else {
						// Set up the GitHub client
						if os.Getenv("GITHUB_TOKEN") == "" {
							return fmt.Errorf("GITHUB_TOKEN environment variable must be set when using --create-on-github")
						}
						client := github.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

						// Check if the issue already exists
						issues, _, err := client.Search.Issues(context.Background(), fmt.Sprintf("%s in:title state:open repo:%s is:issue", issueTitle, repoFullName), &github.SearchOptions{})
						if err != nil {
							return fmt.Errorf("failed to list issues: %w", err)
						}
						if issues.GetTotal() > 0 {
							return fmt.Errorf("issue already exists: %s", issues.Issues[0].GetHTMLURL())
						}

						// Create the issue
						issue, _, err := client.Issues.Create(context.Background(), repoOwner, repoName, &github.IssueRequest{
							Title: &issueTitle,
							Body:  &issueBody,
							Labels: &[]string{
								"tpm",
							},
						})
						if err != nil {
							return fmt.Errorf("failed to create issue: %w", err)
						}
						_, _ = fmt.Fprintf(c.App.Writer, "Issue created: %s", issue.GetHTMLURL())
					}

					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
