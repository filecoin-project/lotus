package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGitLog(t *testing.T) {
	output := "\x1eaaa\x1fAlice\x1ffeat(api): add a thing (#10)\n\n 3 files changed, 10 insertions(+), 2 deletions(-)\n" +
		"\x1ebbb\x1fBob\x1ffix: remove a thing (#11)\n\n 1 file changed, 4 deletions(-)\n" +
		"\x1eccc\x1fCarol\x1fbuild: release Lotus Node v1.0.0\n" +
		"\x1eddd\x1fDave\x1ffix: cherry-picked (#5) (#12)\n\n 1 file changed, 1 insertion(+)\n"
	commits, err := parseGitLog(output)
	require.NoError(t, err)
	require.Equal(t, []commitInfo{
		{Hash: "aaa", Author: "Alice", Subject: "feat(api): add a thing (#10)", PR: 10, Files: 3, Insertions: 10, Deletions: 2},
		{Hash: "bbb", Author: "Bob", Subject: "fix: remove a thing (#11)", PR: 11, Files: 1, Deletions: 4},
		{Hash: "ccc", Author: "Carol", Subject: "build: release Lotus Node v1.0.0"},
		{Hash: "ddd", Author: "Dave", Subject: "fix: cherry-picked (#5) (#12)", PR: 12, Files: 1, Insertions: 1},
	}, commits)

	_, err = parseGitLog("\x1emissing fields\n")
	require.Error(t, err)
}

func TestExtractReleaseNote(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "no block",
			body: "## Proposed Changes\n\n- stuff\n",
		},
		{
			name: "template placeholder only",
			body: "## Release note\n<!-- Optional. Replaces the PR title in the release notes. -->\n\n## Checklist\n- [ ] done\n",
		},
		{
			name: "block ends at the next level-two heading",
			body: "## Proposed Changes\n\n- stuff\n\n## Release note\n<!-- placeholder -->\nOperators must do `x`.\n\n### Detail\nMore.\n## Checklist\n- [x] done\n",
			want: "Operators must do `x`.\n\n### Detail\nMore.",
		},
		{
			name: "CRLF line endings and lower-case heading",
			body: "## release notes\r\nUse `--y` instead.\r\n",
			want: "Use `--y` instead.",
		},
		{
			name: "shell comment in a code fence does not end the block",
			body: "## Release note\nRestart the daemon:\n```sh\n# Stop the daemon\nlotus daemon stop\n```\n## Checklist\n",
			want: "Restart the daemon:\n```sh\n# Stop the daemon\nlotus daemon stop\n```",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, extractReleaseNote(tc.body))
		})
	}
}

func TestNotability(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		pr                    pullRequestInfo
		notable, upgradeWarns bool
	}{
		{name: "a plain feature", pr: pullRequestInfo{Title: "feat(api): add a thing"}},
		{name: "a plain fix", pr: pullRequestInfo{Title: "fix: a bug"}},
		{name: "an unconventional title", pr: pullRequestInfo{Title: "not conventional"}},
		{name: "a placeholder-only release note block", pr: pullRequestInfo{Title: "fix: a bug", Body: "## Release note\n<!-- placeholder -->\n"}},
		{name: "a release note block", pr: pullRequestInfo{Title: "fix: a bug", Body: "## Release note\nOperators must do x.\n"}, notable: true},
		{name: "the highlight label", pr: pullRequestInfo{Title: "chore: something big", Labels: []string{labelHighlight}}, notable: true},
		{name: "a breaking change without a scope", pr: pullRequestInfo{Title: "build!: raise the minimum Go version"}, notable: true, upgradeWarns: true},
		{name: "a breaking change with a scope", pr: pullRequestInfo{Title: "feat(api)!: remove a method"}, notable: true, upgradeWarns: true},
		{name: "the upgrade-warning label", pr: pullRequestInfo{Title: "fix(cli): remove commands", Labels: []string{"area/cli", labelUpgradeWarning}}, notable: true, upgradeWarns: true},
		{name: "a breaking change with the highlight label", pr: pullRequestInfo{Title: "feat(api)!: remove a method", Labels: []string{labelHighlight}}, notable: true, upgradeWarns: true},
		{name: "an exclamation mark later in the title", pr: pullRequestInfo{Title: "fix: stop crashing!"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notable, upgradeWarning := notability(tc.pr)
			require.Equal(t, tc.notable, notable)
			require.Equal(t, tc.upgradeWarns, upgradeWarning)
		})
	}
}

func TestReleasedPullRequests(t *testing.T) {
	const changelog = `# Lotus changelog

# UNRELEASED

- unreleased ([filecoin-project/lotus#1](https://github.com/filecoin-project/lotus/pull/1))

# Node and Miner v1.39.0-rc1 / 2026-11-01

- in rc1 ([filecoin-project/lotus#2](https://github.com/filecoin-project/lotus/pull/2))

# Node v1.38.3 / 2026-10-21

- in v1.38.3 ([filecoin-project/lotus#3](https://github.com/filecoin-project/lotus/pull/3), [filecoin-project/lotus#4](https://github.com/filecoin-project/lotus/pull/4))

# Node v1.3.9 / 2020-01-01

- not a v1.39 pre-release ([filecoin-project/lotus#5](https://github.com/filecoin-project/lotus/pull/5))
`
	released := map[int]bool{}
	releasedPullRequests(changelog, "1.39.0", released)
	require.Equal(t, map[int]bool{3: true, 4: true, 5: true}, released)

	released = map[int]bool{}
	releasedPullRequests(changelog, "1.39.0-rc2", released)
	require.Equal(t, map[int]bool{3: true, 4: true, 5: true}, released)

	released = map[int]bool{}
	releasedPullRequests(changelog, "1.40.0", released)
	require.Equal(t, map[int]bool{2: true, 3: true, 4: true, 5: true}, released)
}

func TestPreviousStableTag(t *testing.T) {
	tags := []string{"v1.38.0", "v1.38.1", "v1.38.2", "v1.39.0-rc1", "miner/v1.38.0", "miner/v1.37.0", "vnot-a-version", ""}
	require.Equal(t, "v1.38.2", previousStableTag(tags, "node", "1.39.0"))
	require.Equal(t, "v1.38.2", previousStableTag(tags, "node", "1.39.0-rc2"))
	require.Equal(t, "v1.38.1", previousStableTag(tags, "node", "1.38.2"))
	require.Equal(t, "miner/v1.38.0", previousStableTag(tags, "miner", "1.39.0"))
	require.Equal(t, "", previousStableTag(tags, "miner", "1.37.0"))
}

func TestFormatChangelogEntry(t *testing.T) {
	const link = " ([#7](https://github.com/filecoin-project/lotus/pull/7))"
	require.Equal(t, "- fix: a bug"+link+"\n", formatChangelogEntry("fix: a bug", 7))
	require.Equal(t, "- A note."+link+"\n", formatChangelogEntry("- A note.", 7))
	require.Equal(t, "- First line.\n\n  Second paragraph."+link+"\n", formatChangelogEntry("First line.\n\nSecond paragraph.", 7))
	// A closing code fence stays on its own line, with the link in a paragraph after it.
	require.Equal(t, "- Run:\n  ```sh\n  lotus daemon stop\n  ```\n\n  "+link[1:]+"\n", formatChangelogEntry("Run:\n```sh\nlotus daemon stop\n```", 7))
}

func TestGenerateChangelogSection(t *testing.T) {
	commits := []commitInfo{
		{Hash: "a1", Author: "Alice", Subject: "feat(api): add a thing (#10)", PR: 10, Files: 3, Insertions: 10, Deletions: 2},
		{Hash: "b2", Author: "Bob", Subject: "fix(cli): remove commands (#11)", PR: 11, Files: 1, Deletions: 40},
		{Hash: "c3", Author: "Carol", Subject: "fix: already released through a backport (#3)", PR: 3, Files: 9, Insertions: 900},
		{Hash: "d4", Author: "dependabot[bot]", Subject: "build(deps): bump x (#12)", PR: 12, Files: 2, Insertions: 1, Deletions: 1},
		{Hash: "e5", Author: "Alice", Subject: "fix: a bug (#13)", PR: 13, Files: 1, Insertions: 5},
		{Hash: "f6", Author: "Bob", Subject: "chore: no PR number", Files: 1, Insertions: 1},
		{Hash: "g7", Author: "Carol", Subject: "build!: raise the minimum Go version (#14)", PR: 14, Files: 4, Insertions: 4, Deletions: 4},
		{Hash: "h8", Author: "Alice", Subject: "perf: faster (#15)", PR: 15, Files: 1, Insertions: 1},
		{Hash: "i9", Author: "Carol", Subject: "docs: renamed after merge (#16)", PR: 16, Files: 1, Insertions: 2},
	}
	prs := map[int]pullRequestInfo{
		10: {Title: "feat(api): add a thing", Labels: []string{labelHighlight}},
		11: {
			Title:  "fix(cli): remove commands",
			Labels: []string{labelUpgradeWarning},
			Body:   "## Proposed Changes\n\n- stuff\n\n## Release note\n<!-- placeholder -->\nThe `lotus filplus` commands are removed.\n\n## Checklist\n",
		},
		12: {Title: "build(deps): bump x"},
		13: {Title: "fix: a bug", Body: "## Release note\n<!-- placeholder -->\n\n## Checklist\n"},
		14: {Title: "build!: raise the minimum Go version"},
		15: {Title: "perf: faster"},
		16: {Title: "docs: renamed after merge"},
	}
	fetchPR := func(number int) (pullRequestInfo, error) {
		pr, ok := prs[number]
		if !ok {
			return pullRequestInfo{}, fmt.Errorf("not found")
		}
		return pr, nil
	}
	release := changelogRelease{
		Type:    "Node and Miner",
		Version: "1.39.0",
		Date:    "2026-11-05",
		CompareLinks: []compareLink{
			{Project: "Node", From: "v1.38.3", To: "v1.39.0"},
			{Project: "Miner", From: "miner/v1.38.0", To: "miner/v1.39.0"},
		},
	}

	var errOut bytes.Buffer
	section, err := generateChangelogSection(&errOut, release, commits, map[int]bool{3: true}, fetchPR)
	require.NoError(t, err)
	require.Equal(t, `# Node and Miner v1.39.0 / 2026-11-05

## ✨ Notable Changes

- ⚠️ **Upgrade warning:** The `+"`lotus filplus`"+` commands are removed. ([#11](https://github.com/filecoin-project/lotus/pull/11))
- ⚠️ **Upgrade warning:** build!: raise the minimum Go version ([#14](https://github.com/filecoin-project/lotus/pull/14))
- feat(api): add a thing ([#10](https://github.com/filecoin-project/lotus/pull/10))

## 📝 All Changes

- Node: [Changes since v1.38.3](https://github.com/filecoin-project/lotus/compare/v1.38.3...v1.39.0)
- Miner: [Changes since miner/v1.38.0](https://github.com/filecoin-project/lotus/compare/miner/v1.38.0...miner/v1.39.0)

<details><summary>All PRs (7)</summary>

- feat(api): add a thing ([#10](https://github.com/filecoin-project/lotus/pull/10))
- fix(cli): remove commands ([#11](https://github.com/filecoin-project/lotus/pull/11))
- build(deps): bump x ([#12](https://github.com/filecoin-project/lotus/pull/12))
- fix: a bug ([#13](https://github.com/filecoin-project/lotus/pull/13))
- build!: raise the minimum Go version ([#14](https://github.com/filecoin-project/lotus/pull/14))
- perf: faster ([#15](https://github.com/filecoin-project/lotus/pull/15))
- docs: renamed after merge ([#16](https://github.com/filecoin-project/lotus/pull/16))

</details>

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Bob | 2 | +1/-40 | 2 |
| Alice | 3 | +16/-2 | 5 |
| Carol | 2 | +6/-4 | 5 |
| dependabot[bot] | 1 | +1/-1 | 2 |

`, section)
	require.Contains(t, errOut.String(), `warning: f6 "chore: no PR number" has no PR, so it is not listed`)

	// With nothing notable, the Notable Changes heading stays as a placeholder for the releaser.
	release.CompareLinks = release.CompareLinks[:1]
	section, err = generateChangelogSection(&errOut, release, commits[4:5], nil, fetchPR)
	require.NoError(t, err)
	require.Contains(t, section, "## ✨ Notable Changes\n\n<!-- No PR was marked as notable. List what users should know about this release here, or delete this section. -->\n\n## 📝 All Changes\n")
	require.Contains(t, section, "[Changes since v1.38.3](https://github.com/filecoin-project/lotus/compare/v1.38.3...v1.39.0)\n\n<details><summary>All PRs (1)</summary>")

	// An upgrade warning whose note starts with a code fence puts the marker on its own line, so the fence still opens
	// the block.
	prs[13] = pullRequestInfo{Title: "fix!: a bug", Body: "## Release note\n```sh\nlotus daemon stop\n```\n"}
	section, err = generateChangelogSection(&errOut, release, commits[4:5], nil, fetchPR)
	require.NoError(t, err)
	require.Contains(t, section, "- ⚠️ **Upgrade warning:**\n\n  ```sh\n  lotus daemon stop\n  ```\n\n  ([#13](https://github.com/filecoin-project/lotus/pull/13))\n")

	// A PR that can't be read stops generation, and every such PR is reported at once.
	delete(prs, 13)
	delete(prs, 16)
	_, err = generateChangelogSection(&errOut, release, commits, nil, fetchPR)
	require.ErrorContains(t, err, "#13: not found")
	require.ErrorContains(t, err, "#16: not found")
}

func TestInsertChangelogSection(t *testing.T) {
	const changelog = "# Lotus changelog\n\nPreamble.\n\n# Node v1.38.3 / 2026-10-21\n\n- old entry\n"
	const section = "# Node and Miner v1.39.0 / 2026-11-05\n\n## 🐛 Bug Fixes\n\n- new entry\n\n"
	names := []string{"Node", "Miner"}

	updated, err := insertChangelogSection(changelog, section, "1.39.0", names, false)
	require.NoError(t, err)
	require.Equal(t, "# Lotus changelog\n\nPreamble.\n\n"+section+"# Node v1.38.3 / 2026-10-21\n\n- old entry\n", updated)

	// The release workflow must find the generated section for both projects.
	for project, tag := range map[string]string{"node": "v1.39.0", "miner": "miner/v1.39.0"} {
		found, err := findChangelogSection(updated, project, tag)
		require.NoError(t, err)
		require.True(t, found.Versioned, project)
		require.Equal(t, "## 🐛 Bug Fixes\n\n- new entry\n\n", found.Body, project)
	}

	_, err = insertChangelogSection(updated, section, "1.39.0", names, false)
	require.ErrorContains(t, err, "--force")
	// A miner-only release of the same version also collides with the combined section.
	_, err = insertChangelogSection(updated, section, "1.39.0", []string{"Miner"}, false)
	require.ErrorContains(t, err, "--force")
	// An RC section is a different release.
	_, err = insertChangelogSection(updated, section, "1.39.0-rc1", names, false)
	require.NoError(t, err)

	const replacement = "# Node and Miner v1.39.0 / 2026-11-06\n\n- replaced\n\n"
	replaced, err := insertChangelogSection(updated, replacement, "1.39.0", names, true)
	require.NoError(t, err)
	require.Equal(t, "# Lotus changelog\n\nPreamble.\n\n"+replacement+"# Node v1.38.3 / 2026-10-21\n\n- old entry\n", replaced)
}

func TestResolveCommitPullRequests(t *testing.T) {
	commits := []commitInfo{
		{Hash: "a1", Subject: "fix: squash-merged (#10)", PR: 10},
		{Hash: "b2", Subject: "fix: first commit of a rebase-merged PR"},
		{Hash: "c3", Subject: "build: pushed directly"},
		{Hash: "d4", Subject: "lookup fails"},
	}
	var looked []string
	err := resolveCommitPullRequests(commits, func(hash string) (int, error) {
		looked = append(looked, hash)
		switch hash {
		case "b2":
			return 11, nil
		case "d4":
			return 0, fmt.Errorf("rate limited")
		}
		return 0, nil
	})
	// A commit with no merged PR is fine, but a failed lookup is an error so the PR isn't silently left out.
	require.ErrorContains(t, err, "d4: rate limited")
	require.NotContains(t, err.Error(), "c3")
	require.Equal(t, []string{"b2", "c3", "d4"}, looked)
	require.Equal(t, []int{10, 11, 0, 0}, []int{commits[0].PR, commits[1].PR, commits[2].PR, commits[3].PR})
}

// TestReleasedPullRequestsFromGeneratedEntries guards against the generated link format drifting from what later runs skip.
func TestReleasedPullRequestsFromGeneratedEntries(t *testing.T) {
	changelog := "# Lotus changelog\n\n# Node v1.39.0 / 2026-11-05\n\n" + formatChangelogEntry("fix: a bug", 42)
	released := map[int]bool{}
	releasedPullRequests(changelog, "1.40.0", released)
	require.Equal(t, map[int]bool{42: true}, released)
}

func TestReleaseCompareLinks(t *testing.T) {
	tags := []string{"v1.38.2", "v1.38.3", "miner/v1.38.0"}
	both, bothNames := []string{"node", "miner"}, []string{"Node", "Miner"}

	links, err := releaseCompareLinks(tags, both, bothNames, "1.39.0", "")
	require.NoError(t, err)
	require.Equal(t, []compareLink{
		{Project: "Node", From: "v1.38.3", To: "v1.39.0"},
		{Project: "Miner", From: "miner/v1.38.0", To: "miner/v1.39.0"},
	}, links)

	// An explicit --from is the start of both the listed range and the first project's compare link.
	links, err = releaseCompareLinks(tags, both, bothNames, "1.39.0", "v1.38.2")
	require.NoError(t, err)
	require.Equal(t, []compareLink{
		{Project: "Node", From: "v1.38.2", To: "v1.39.0"},
		{Project: "Miner", From: "miner/v1.38.0", To: "miner/v1.39.0"},
	}, links)

	links, err = releaseCompareLinks(tags, []string{"miner"}, []string{"Miner"}, "1.39.0", "abc123")
	require.NoError(t, err)
	require.Equal(t, []compareLink{{Project: "Miner", From: "abc123", To: "miner/v1.39.0"}}, links)

	// Without tags, an explicit --from is needed.
	_, err = releaseCompareLinks(nil, []string{"node"}, []string{"Node"}, "1.39.0", "")
	require.ErrorContains(t, err, "--from")
	_, err = releaseCompareLinks(nil, []string{"node"}, []string{"Node"}, "1.39.0", "v1.0.0")
	require.NoError(t, err)
}
