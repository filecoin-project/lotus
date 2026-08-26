package main

import (
	"bytes"
	"os"
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
			for _, flow := range releaseFlows {
				t.Run(releaseType+"/"+releaseLevel+"/"+flow.name, func(t *testing.T) {
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
				})
			}
		}
	}
}
