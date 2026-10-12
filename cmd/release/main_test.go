package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/stretchr/testify/require"
)

func TestStripControlLines(t *testing.T) {
	for _, tc := range []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "control-only line contributes no newline",
			template: "before\n{{if .Foo}}\ninside\n{{end}}\nafter",
			expected: "before{{if .Foo}}\ninside{{end}}\nafter",
		},
		{
			name:     "indented control-only line is stripped too",
			template: "before\n  {{if .Foo}}\ninside\n  {{end}}\nafter",
			expected: "before{{if .Foo}}\ninside{{end}}\nafter",
		},
		{
			name:     "consecutive control-only lines collapse onto the same line",
			template: "before\n{{if .Foo}}\n{{if .Bar}}\ninside\n{{end}}\n{{end}}\nafter",
			expected: "before{{if .Foo}}{{if .Bar}}\ninside{{end}}{{end}}\nafter",
		},
		{
			name:     "an action within a content line is left alone",
			template: "before\nvalue: {{.Foo}}\nafter",
			expected: "before\nvalue: {{.Foo}}\nafter",
		},
		{
			name:     "blank lines around control-only lines survive into the body",
			template: "before\n\n{{if .Foo}}\ninside\n{{end}}\n\nafter",
			expected: "before\n{{if .Foo}}\ninside{{end}}\n\nafter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, stripControlLines(tc.template))
		})
	}
}

func TestFindMalformedActionWrapper(t *testing.T) {
	require.Empty(t, findMalformedActionWrapper("<!--{{if .Foo}}-->\ncontent\n<!--{{end}}-->"))
	require.Empty(t, findMalformedActionWrapper("<!-- a plain comment -->\ncontent"))
	require.Empty(t, findMalformedActionWrapper("prefix<!--{{if .Foo}}-->inline<!--{{end}}-->suffix"))
	require.Equal(t, `<!--{{if .Foo}})-->`, findMalformedActionWrapper("<!--{{if .Foo}})-->\ncontent"))
}

// TestReleaseIssueTemplateRenders guards against the issue template drifting out of sync with the
// pre-processing above: it must have no malformed wrappers, and must render for every combination
// of the flags that drive its control flow.
func TestReleaseIssueTemplateRenders(t *testing.T) {
	issueTemplate, err := os.ReadFile("../../documentation/misc/RELEASE_ISSUE_TEMPLATE.md")
	require.NoError(t, err)

	require.Empty(t, findMalformedActionWrapper(string(issueTemplate)))

	templateSource := commentWrappedActionRegexp.ReplaceAllString(string(issueTemplate), "$1")
	templateSource = stripControlLines(templateSource)
	tmpl, err := template.New("issue").Funcs(sprig.FuncMap()).Parse(templateSource)
	require.NoError(t, err)

	releaseFlows := []struct {
		name                 string
		requestedReleaseFlow string
		releaseFlow          string
		noRCRelease          bool
		rcRelease            bool
		firstReleaseTarget   string
		releaseTargets       []string
		networkUpgrade       string
	}{
		{
			name:                 "explicit no-rc",
			requestedReleaseFlow: releaseFlowNoRC,
			releaseFlow:          releaseFlowNoRC,
			noRCRelease:          true,
			firstReleaseTarget:   "Stable Release",
			releaseTargets:       []string{"Stable Release"},
		},
		{
			name:                 "explicit rc without network upgrade",
			requestedReleaseFlow: releaseFlowRC,
			releaseFlow:          releaseFlowRC,
			rcRelease:            true,
			firstReleaseTarget:   "rc1",
			releaseTargets:       []string{"rc1", "rcX", "Stable Release"},
		},
		{
			name:                 "explicit rc with network upgrade",
			requestedReleaseFlow: releaseFlowRC,
			releaseFlow:          releaseFlowRC,
			rcRelease:            true,
			firstReleaseTarget:   "rc1",
			releaseTargets:       []string{"rc1", "rcX", "Stable Release"},
			networkUpgrade:       "28",
		},
		{
			name:                 "auto resolves to no-rc",
			requestedReleaseFlow: releaseFlowAuto,
			releaseFlow:          releaseFlowNoRC,
			noRCRelease:          true,
			firstReleaseTarget:   "Stable Release",
			releaseTargets:       []string{"Stable Release"},
		},
		{
			name:                 "auto resolves to rc",
			requestedReleaseFlow: releaseFlowAuto,
			releaseFlow:          releaseFlowRC,
			rcRelease:            true,
			firstReleaseTarget:   "rc1",
			releaseTargets:       []string{"rc1", "rcX", "Stable Release"},
			networkUpgrade:       "28",
		},
	}

	for _, releaseType := range []string{"Node", "Miner", "Node and Miner"} {
		for _, releaseLevel := range []string{"minor", "patch"} {
			for _, baseTag := range []string{"", "1.29.9"} {
				if baseTag != "" && releaseLevel != "patch" {
					continue
				}
				for _, flow := range releaseFlows {
					t.Run(releaseType+"/"+releaseLevel+"/base="+baseTag+"/"+flow.name, func(t *testing.T) {
						var buffer bytes.Buffer
						err := tmpl.Execute(&buffer, map[string]any{
							"ContentGeneratedWithLotusReleaseCli": true,
							"LotusReleaseCliString":               "release create-issue",
							"Type":                                releaseType,
							"Tag":                                 "1.30.0",
							"NextTag":                             "1.30.1",
							"Level":                               releaseLevel,
							"RequestedReleaseFlow":                flow.requestedReleaseFlow,
							"ReleaseFlow":                         flow.releaseFlow,
							"NoRCRelease":                         flow.noRCRelease,
							"RCRelease":                           flow.rcRelease,
							"FirstReleaseTarget":                  flow.firstReleaseTarget,
							"ReleaseTargets":                      flow.releaseTargets,
							"BaseTag":                             baseTag,
							"NetworkUpgrade":                      flow.networkUpgrade,
							"NetworkUpgradeDiscussionLink":        "https://example.com/discussion?a=1&b=2",
							"NetworkUpgradeChangelogEntryLink":    "https://example.com/changelog?a=1&b=2",
							"RC1DateString":                       "TBD",
							"StableDateString":                    "TBD",
						})
						require.NoError(t, err)
						// A leaked comment delimiter means a control statement reached the issue body.
						require.NotContains(t, buffer.String(), "<!--{{")
						// The issue body is Markdown, so links must not be HTML-escaped.
						require.NotContains(t, buffer.String(), "&amp;")
						// A heading directly after an HTML line such as `</details>` is swallowed into the
						// HTML block and renders as literal text, so headings need a blank line before them.
						lines := strings.Split(buffer.String(), "\n")
						for i, line := range lines {
							if i > 0 && strings.HasPrefix(line, "#") && strings.HasPrefix(strings.TrimSpace(lines[i-1]), "<") {
								t.Errorf("heading %q directly follows HTML line %q", line, lines[i-1])
							}
						}
						if baseTag != "" {
							// A release cut from an earlier tag must never be told to fork from master or skip backports.
							require.NotContains(t, buffer.String(), "origin/master:refs/heads/")
							require.NotContains(t, buffer.String(), "No additional backport PR is needed")
							require.Contains(t, buffer.String(), "v"+baseTag+"^{commit}:refs/heads/release/")
							require.Contains(t, buffer.String(), "build: backport changes for")
						}
					})
				}
			}
		}
	}
}

const testChangelog = `# Lotus changelog

Preamble.

# UNRELEASED

## New Features

- unreleased entry

# UNRELEASED v9.9.9

- pending v9.9.9 entry

# Node and Miner v1.37.0 / 2026-10-06

- combined v1.37.0 entry

# Node and Miner v1.37.0-rc1 / 2026-09-22

- combined v1.37.0-rc1 entry

# Node v1.36.3 / 2026-09-10

- node v1.36.3 entry

# Node v1.36.0 / 2026-05-13

- node v1.36.0 entry

# Miner v1.36.0 / 2026-05-13

- miner v1.36.0 entry

# v1.2.3 / 2020-01-01

- legacy v1.2.3 entry
`

func TestWriteChangelogSection(t *testing.T) {
	// The bottom-most UNRELEASED section in testChangelog is the fallback.
	const unreleasedHeader = "# UNRELEASED v9.9.9"
	const unreleasedBody = "- pending v9.9.9 entry\n\n"
	for _, tc := range []struct {
		name       string
		changelog  string
		project    string
		tag        string
		wantHeader string
		wantBody   string
		// wantProblem is the annotation title expected when the release has no populated section of its own, or "" for none.
		wantProblem string
	}{
		{
			name:       "node stable from a combined header",
			project:    "node",
			tag:        "v1.37.0",
			wantHeader: "# Node and Miner v1.37.0 / 2026-10-06",
			wantBody:   "- combined v1.37.0 entry\n\n",
		},
		{
			name:       "miner stable from a combined header",
			project:    "miner",
			tag:        "miner/v1.37.0",
			wantHeader: "# Node and Miner v1.37.0 / 2026-10-06",
			wantBody:   "- combined v1.37.0 entry\n\n",
		},
		{
			name:       "node rc is not confused with stable",
			project:    "node",
			tag:        "v1.37.0-rc1",
			wantHeader: "# Node and Miner v1.37.0-rc1 / 2026-09-22",
			wantBody:   "- combined v1.37.0-rc1 entry\n\n",
		},
		{
			name:        "missing rc does not match stable",
			project:     "node",
			tag:         "v1.37.0-rc2",
			wantHeader:  unreleasedHeader,
			wantBody:    unreleasedBody,
			wantProblem: "No CHANGELOG section",
		},
		{
			name:       "node only section",
			project:    "node",
			tag:        "v1.36.3",
			wantHeader: "# Node v1.36.3 / 2026-09-10",
			wantBody:   "- node v1.36.3 entry\n\n",
		},
		{
			name:        "node only section does not match a miner tag",
			project:     "miner",
			tag:         "miner/v1.36.3",
			wantHeader:  unreleasedHeader,
			wantBody:    unreleasedBody,
			wantProblem: "No CHANGELOG section",
		},
		{
			name:       "node release skips a miner section of the same version",
			project:    "node",
			tag:        "v1.36.0",
			wantHeader: "# Node v1.36.0 / 2026-05-13",
			wantBody:   "- node v1.36.0 entry\n\n",
		},
		{
			name:        "versioned UNRELEASED header is only a fallback",
			project:     "node",
			tag:         "v9.9.9",
			wantHeader:  unreleasedHeader,
			wantBody:    unreleasedBody,
			wantProblem: "No CHANGELOG section",
		},
		{
			name:        "partial version does not match",
			changelog:   "# UNRELEASED\n\n- unreleased entry\n\n# Node v9.9.9 / 2030-01-01\n\n- node v9.9.9 entry\n",
			project:     "node",
			tag:         "v9.9",
			wantHeader:  "# UNRELEASED",
			wantBody:    "- unreleased entry\n\n",
			wantProblem: "No CHANGELOG section",
		},
		{
			name:        "missing section without an UNRELEASED fallback",
			changelog:   "# Lotus changelog\n\n# Node v1.0.0 / 2020-01-01\n\n- node v1.0.0 entry\n",
			project:     "node",
			tag:         "v1.0.1",
			wantProblem: "No CHANGELOG section",
		},
		{
			name:        "section with only headings at the end of the file is empty",
			changelog:   "# UNRELEASED\n\n- unreleased entry\n\n# Node v1.0.0 / 2020-01-01\n\n## Bug Fixes",
			project:     "node",
			tag:         "v1.0.0",
			wantHeader:  "# Node v1.0.0 / 2020-01-01",
			wantBody:    "## Bug Fixes",
			wantProblem: "Empty CHANGELOG section",
		},
		{
			name:       "legacy header",
			project:    "node",
			tag:        "v1.2.3",
			wantHeader: "# v1.2.3 / 2020-01-01",
			wantBody:   "- legacy v1.2.3 entry\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changelog := tc.changelog
			if changelog == "" {
				changelog = testChangelog
			}

			section, err := findChangelogSection(changelog, tc.project, tc.tag)
			require.NoError(t, err)
			require.Equal(t, tc.wantHeader, section.Header)
			require.Equal(t, tc.wantBody, section.Body)

			for _, publishing := range []bool{false, true} {
				var out, errOut bytes.Buffer
				err := writeChangelogSection(&out, &errOut, changelog, tc.project, tc.tag, publishing, true)
				require.Equal(t, tc.wantBody, out.String())
				switch {
				case tc.wantProblem == "":
					require.NoError(t, err)
					require.NotContains(t, errOut.String(), "::")
				case publishing:
					require.ErrorContains(t, err, "::error title="+tc.wantProblem+"::")
					require.NotContains(t, errOut.String(), "::warning")
				default:
					require.NoError(t, err)
					require.Contains(t, errOut.String(), "::warning title="+tc.wantProblem+"::")
				}
			}
		})
	}
}

func TestWriteChangelogSectionUnknownProject(t *testing.T) {
	var out, errOut bytes.Buffer
	require.Error(t, writeChangelogSection(&out, &errOut, testChangelog, "worker", "v1.37.0", false, false))
}

func TestWorkflowAnnotation(t *testing.T) {
	require.Equal(t, "::warning title=T::m", workflowAnnotation(true, "warning", "T", "m"))
	require.Equal(t, "warning: T: m", workflowAnnotation(false, "warning", "T", "m"))
}
