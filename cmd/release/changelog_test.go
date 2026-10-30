package main

import (
	"bytes"
	"fmt"
	"io"
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, extractReleaseNote(tc.body))
		})
	}
}

func TestClassifyPullRequest(t *testing.T) {
	for _, tc := range []struct {
		title  string
		labels []string
		want   releaseNoteSection
	}{
		{title: "feat(api): add a thing", want: sectionNewFeatures},
		{title: "fix: a bug", want: sectionBugFixes},
		{title: "perf(chain): faster", want: sectionImprovements},
		{title: "refactor: tidy", want: sectionImprovements},
		{title: `Revert "feat: add a thing"`, want: sectionImprovements},
		{title: "not conventional", want: sectionImprovements},
		{title: "chore(deps): bump x", want: sectionOther},
		{title: "build(deps): bump x", want: sectionOther},
		{title: "ci: faster", want: sectionOther},
		{title: "test: more", want: sectionOther},
		{title: "docs: words", want: sectionOther},
		{title: "style: gofmt", want: sectionOther},
		{title: "build!: raise the minimum Go version", want: sectionUpgradeWarnings},
		{title: "feat(api)!: remove a method", want: sectionUpgradeWarnings},
		{title: "fix(cli): remove commands", labels: []string{"area/cli", labelUpgradeWarning}, want: sectionUpgradeWarnings},
		{title: "chore: something big", labels: []string{labelHighlight}, want: sectionHighlights},
		{title: "feat: both labels", labels: []string{labelHighlight, labelUpgradeWarning}, want: sectionUpgradeWarnings},
	} {
		t.Run(tc.title, func(t *testing.T) {
			require.Equal(t, tc.want, classifyPullRequest(tc.title, tc.labels))
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
		// #16 is missing, so its entry falls back to the commit subject.
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
	section := generateChangelogSection(&errOut, release, commits, map[int]bool{3: true}, fetchPR, nil)
	require.Equal(t, `# Node and Miner v1.39.0 / 2026-11-05

## ☢️ Upgrade Warnings ☢️

- The `+"`lotus filplus`"+` commands are removed. ([#11](https://github.com/filecoin-project/lotus/pull/11))
- build!: raise the minimum Go version ([#14](https://github.com/filecoin-project/lotus/pull/14))

## ✨ Highlights

- feat(api): add a thing ([#10](https://github.com/filecoin-project/lotus/pull/10))

## 🐛 Bug Fixes

- fix: a bug ([#13](https://github.com/filecoin-project/lotus/pull/13))

## 👌 Improvements

- perf: faster ([#15](https://github.com/filecoin-project/lotus/pull/15))

<details><summary>Other changes</summary>

- build(deps): bump x ([#12](https://github.com/filecoin-project/lotus/pull/12))
- docs: renamed after merge ([#16](https://github.com/filecoin-project/lotus/pull/16))

</details>

## 📝 Changelog

- Node: [Changes since v1.38.3](https://github.com/filecoin-project/lotus/compare/v1.38.3...v1.39.0)
- Miner: [Changes since miner/v1.38.0](https://github.com/filecoin-project/lotus/compare/miner/v1.38.0...miner/v1.39.0)

## 👨‍👩‍👧‍👦 Contributors

| Contributor | Commits | Lines ± | Files Changed |
|-------------|---------|---------|---------------|
| Bob | 2 | +1/-40 | 2 |
| Alice | 3 | +16/-2 | 5 |
| Carol | 2 | +6/-4 | 5 |
| dependabot[bot] | 1 | +1/-1 | 2 |

`, section)
	require.Contains(t, errOut.String(), `warning: f6 "chore: no PR number" has no PR, so it is not listed`)
	require.Contains(t, errOut.String(), "warning: using the commit subject for #16")

	// With nothing labelled as a highlight, the Highlights heading stays as a placeholder for the releaser.
	release.CompareLinks = release.CompareLinks[:1]
	section = generateChangelogSection(&errOut, release, commits[4:5], nil, fetchPR, nil)
	require.Contains(t, section, "## ✨ Highlights\n\n<!-- Draft the highlights of this release here, or delete this section. -->\n\n## 🐛 Bug Fixes\n")
	require.Contains(t, section, "## 📝 Changelog\n\n[Changes since v1.38.3](https://github.com/filecoin-project/lotus/compare/v1.38.3...v1.39.0)\n")
	require.NotContains(t, section, "Other changes")
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

func TestChangedDependencies(t *testing.T) {
	oldGoMod := []byte(`module github.com/filecoin-project/lotus

go 1.26

require (
	github.com/filecoin-project/go-state-types v0.16.0
	github.com/filecoin-project/go-amt-ipld/v4 v4.4.0
	github.com/filecoin-project/go-f3 v0.0.0-20260101000000-aaaaaaaaaaaa
	github.com/filecoin-project/specs-actors/v8 v8.0.1
	github.com/filecoin-project/go-crypto v0.1.0 // indirect
	github.com/filecoin-project/go-paramfetch/sub v0.1.0
	github.com/filecoin-project/old-dep v0.1.0
	github.com/ipfs/go-cid v0.5.0
)
`)
	newGoMod := []byte(`module github.com/filecoin-project/lotus

go 1.26

require (
	github.com/filecoin-project/go-state-types v0.17.0
	github.com/filecoin-project/go-amt-ipld/v4 v4.5.0
	github.com/filecoin-project/go-f3 v0.0.0-20260201000000-bbbbbbbbbbbb
	github.com/filecoin-project/specs-actors/v8 v8.0.1
	github.com/filecoin-project/go-crypto v0.2.0+incompatible // indirect
	github.com/filecoin-project/go-paramfetch/sub v0.2.0
	github.com/filecoin-project/new-dep v0.1.0
	github.com/ipfs/go-cid v0.6.0
)
`)
	changes, err := changedDependencies(oldGoMod, newGoMod, dependencyPrefix)
	require.NoError(t, err)
	require.Equal(t, []dependencyChange{
		{Repo: "github.com/filecoin-project/go-amt-ipld", OldRef: "v4.4.0", NewRef: "v4.5.0"},
		{Repo: "github.com/filecoin-project/go-crypto", OldRef: "v0.1.0", NewRef: "v0.2.0"},
		{Repo: "github.com/filecoin-project/go-f3", OldRef: "aaaaaaaaaaaa", NewRef: "bbbbbbbbbbbb"},
		{Repo: "github.com/filecoin-project/go-paramfetch", OldRef: "sub/v0.1.0", NewRef: "sub/v0.2.0"},
		{Repo: "github.com/filecoin-project/go-state-types", OldRef: "v0.16.0", NewRef: "v0.17.0"},
	}, changes)
}

func TestGenerateChangelogSectionCountsDependencyContributors(t *testing.T) {
	commits := []commitInfo{{Hash: "a1", Author: "Alice", Subject: "fix: a bug (#13)", PR: 13, Files: 1, Insertions: 5}}
	dependencyCommits := []commitInfo{
		{Hash: "d1", Author: "Alice", Subject: "fix the dependency (#99)", PR: 99, Files: 2, Insertions: 3, Deletions: 1},
		{Hash: "d2", Author: "Erin", Subject: "a dependency change", Files: 1, Insertions: 100},
	}
	fetchPR := func(int) (pullRequestInfo, error) { return pullRequestInfo{Title: "fix: a bug"}, nil }
	release := changelogRelease{Type: "Node", Version: "1.39.0", Date: "2026-11-05", CompareLinks: []compareLink{{Project: "Node", From: "v1.38.3", To: "v1.39.0"}}}

	var errOut bytes.Buffer
	section := generateChangelogSection(&errOut, release, commits, nil, fetchPR, dependencyCommits)
	require.Contains(t, section, "| Erin | 1 | +100/-0 | 1 |\n| Alice | 2 | +8/-1 | 3 |\n")
	// Dependency commits are not listed as changes, and their PR numbers are not lotus PRs.
	require.NotContains(t, section, "pull/99")
	require.Empty(t, errOut.String())
}

func TestResolveCommitPullRequests(t *testing.T) {
	commits := []commitInfo{
		{Hash: "a1", Subject: "fix: squash-merged (#10)", PR: 10},
		{Hash: "b2", Subject: "fix: first commit of a rebase-merged PR"},
		{Hash: "c3", Subject: "build: pushed directly"},
		{Hash: "d4", Subject: "lookup fails"},
	}
	var looked []string
	resolveCommitPullRequests(io.Discard, commits, func(hash string) (int, error) {
		looked = append(looked, hash)
		switch hash {
		case "b2":
			return 11, nil
		case "d4":
			return 0, fmt.Errorf("rate limited")
		}
		return 0, nil
	})
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
