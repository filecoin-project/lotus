package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTrustedPullRequest(t *testing.T) {
	merged := time.Date(2026, 10, 26, 11, 0, 0, 0, time.UTC)
	before, after, later := merged.Add(-time.Hour), merged.Add(time.Hour), merged.Add(2*time.Hour)
	const (
		original = "Proposed changes."
		noted    = "Proposed changes.\n\n## Release note\nThe commands are removed."
		sneaky   = "Proposed changes.\n\n## Release note\nThis release also makes you rich."
	)
	maintainers := func(login string) (bool, error) {
		require.NotEmpty(t, login, "a deleted account's permission shouldn't be looked up")
		return login == "maintainer", nil
	}

	for _, tc := range []struct {
		name      string
		history   pullRequestHistory
		wantTitle string
		wantBody  string
		wantErr   []string
	}{
		{
			name:      "never edited",
			history:   pullRequestHistory{Title: "fix: a bug", Body: original},
			wantTitle: "fix: a bug",
			wantBody:  original,
		},
		{
			name: "edits before the merge are part of the merged version, whoever made them",
			history: pullRequestHistory{Title: "fix: a bug", Body: noted, BodyEdits: []bodyEdit{
				{At: before.Add(-time.Hour), Editor: "contributor", Body: original},
				{At: before, Editor: "contributor", Body: noted},
			}},
			wantTitle: "fix: a bug",
			wantBody:  noted,
		},
		{
			name: "a maintainer's edit after the merge is used",
			history: pullRequestHistory{Title: "fix: a bug", Body: noted, BodyEdits: []bodyEdit{
				{At: before, Editor: "contributor", Body: original},
				{At: after, Editor: "maintainer", Body: noted},
			}},
			wantTitle: "fix: a bug",
			wantBody:  noted,
		},
		{
			name: "a contributor's edit after the merge stops generation",
			history: pullRequestHistory{Title: "fix: a bug", Body: sneaky, BodyEdits: []bodyEdit{
				{At: before, Editor: "contributor", Body: noted},
				{At: after, Editor: "contributor", Body: sneaky},
			}},
			wantTitle: "fix: a bug",
			wantBody:  noted,
			wantErr:   []string{"the description was edited after merge by contributor", `from "The commands are removed." to "This release also makes you rich."`},
		},
		{
			name: "a maintainer's later edit covers a contributor's edit",
			history: pullRequestHistory{Title: "fix: a bug", Body: noted, BodyEdits: []bodyEdit{
				{At: before, Editor: "contributor", Body: original},
				{At: after, Editor: "contributor", Body: sneaky},
				{At: later, Editor: "maintainer", Body: noted},
			}},
			wantTitle: "fix: a bug",
			wantBody:  noted,
		},
		{
			name: "an edit by a deleted account is not trusted",
			history: pullRequestHistory{Title: "fix: a bug", Body: sneaky, BodyEdits: []bodyEdit{
				{At: before, Editor: "contributor", Body: original},
				{At: after, Body: sneaky},
			}},
			wantTitle: "fix: a bug",
			wantBody:  original,
			wantErr:   []string{"by a deleted account"},
		},
		{
			name: "a maintainer's rename after the merge is used",
			history: pullRequestHistory{Title: "fix(cli)!: remove commands", Body: original, Renames: []titleRename{
				{At: before, Actor: "contributor", From: "remove commands", To: "fix(cli): remove commands"},
				{At: after, Actor: "maintainer", From: "fix(cli): remove commands", To: "fix(cli)!: remove commands"},
			}},
			wantTitle: "fix(cli)!: remove commands",
			wantBody:  original,
		},
		{
			name: "a contributor's rename after the merge stops generation, as does an edit",
			history: pullRequestHistory{Title: "feat: everything is faster", Body: sneaky,
				Renames:   []titleRename{{At: after, Actor: "contributor", From: "fix: a bug", To: "feat: everything is faster"}},
				BodyEdits: []bodyEdit{{At: before, Editor: "contributor", Body: original}, {At: later, Editor: "contributor", Body: sneaky}},
			},
			wantTitle: "fix: a bug",
			wantBody:  original,
			wantErr:   []string{`the title was changed after merge by contributor`, `to "feat: everything is faster"; the changelog would use "fix: a bug"`, "the description was edited after merge"},
		},
		{
			name: "the merged description's text was deleted from the history",
			history: pullRequestHistory{Title: "fix: a bug", Body: noted, BodyEdits: []bodyEdit{
				{At: before, Editor: "contributor", Deleted: true},
				{At: after, Editor: "contributor", Body: noted},
			}},
			wantErr: []string{"the description was edited after merge"},
		},
		{
			name: "a deleted version no longer matters once a maintainer edits after it",
			history: pullRequestHistory{Title: "fix: a bug", Body: noted, BodyEdits: []bodyEdit{
				{At: before, Editor: "contributor", Deleted: true},
				{At: after, Editor: "maintainer", Body: noted},
			}},
			wantTitle: "fix: a bug",
			wantBody:  noted,
		},
		{
			name: "the merged description's text was deleted, and a later version was trusted but then deleted too",
			history: pullRequestHistory{Title: "fix: a bug", Body: sneaky, BodyEdits: []bodyEdit{
				{At: before, Editor: "contributor", Deleted: true},
				{At: after, Editor: "maintainer", Deleted: true},
				{At: later, Editor: "maintainer", Body: sneaky},
			}},
			wantTitle: "fix: a bug",
			wantBody:  sneaky,
		},
		{
			name: "an older version the changelog needs was deleted",
			history: pullRequestHistory{Title: "fix: a bug", Body: sneaky, BodyEdits: []bodyEdit{
				{At: before, Editor: "contributor", Deleted: true},
				{At: after, Editor: "maintainer", Deleted: true},
				{At: later, Editor: "contributor", Body: sneaky},
			}},
			wantErr: []string{"would change from a version deleted from the edit history to"},
		},
		{
			name: "the current description is used when every edit was before the merge, even if an old version was deleted",
			history: pullRequestHistory{Title: "fix: a bug", Body: noted, BodyEdits: []bodyEdit{
				{At: before.Add(-time.Hour), Editor: "contributor", Deleted: true},
				{At: before, Editor: "contributor", Body: noted},
			}},
			wantTitle: "fix: a bug",
			wantBody:  noted,
		},
		{
			name:    "a history without the merged description",
			history: pullRequestHistory{Title: "fix: a bug", Body: noted, BodyEdits: []bodyEdit{{At: after, Editor: "maintainer", Body: noted}}},
			wantErr: []string{"no version from before the merge"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.history.MergedAt = merged
			tc.history.Labels = []string{labelUpgradeWarning}
			pr, err := trustedPullRequest(tc.history, maintainers)
			for _, want := range tc.wantErr {
				require.ErrorContains(t, err, want)
			}
			if tc.wantErr == nil {
				require.NoError(t, err)
			}
			if tc.wantTitle != "" {
				require.Equal(t, pullRequestInfo{Title: tc.wantTitle, Body: tc.wantBody, Labels: []string{labelUpgradeWarning}}, pr)
			}
		})
	}

	// A failed permission lookup stops generation rather than trusting or ignoring the edit.
	_, err := trustedPullRequest(pullRequestHistory{MergedAt: merged, BodyEdits: []bodyEdit{
		{At: before, Editor: "contributor"}, {At: after, Editor: "someone"},
	}}, func(string) (bool, error) { return false, fmt.Errorf("rate limited") })
	require.ErrorContains(t, err, "rate limited")
}

func TestParsePullRequestHistory(t *testing.T) {
	h, err := parsePullRequestHistory([]byte(`{"data": {"repository": {"pullRequest": {
		"mergedAt": "2026-10-26T11:24:31Z",
		"title": "fix(cli): remove deprecated filplus commands",
		"body": "current",
		"labels": {"nodes": [{"name": "release-note/upgrade-warning"}]},
		"timelineItems": {"pageInfo": {"hasNextPage": false}, "nodes": [
			{"createdAt": "2026-10-26T10:30:00Z", "actor": {"login": "LexLuthr"}, "previousTitle": "remove", "currentTitle": "fix(cli): remove deprecated filplus commands"}
		]},
		"userContentEdits": {"pageInfo": {"hasNextPage": false}, "nodes": [
			{"editedAt": "2026-11-01T16:20:09Z", "editor": {"login": "BigLep"}, "diff": "current"},
			{"editedAt": "2026-10-26T10:00:56Z", "editor": null, "diff": "original"}
		]}
	}}}}`))
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 26, 11, 24, 31, 0, time.UTC), h.MergedAt)
	require.Equal(t, []string{labelUpgradeWarning}, h.Labels)
	require.Equal(t, []titleRename{{At: time.Date(2026, 10, 26, 10, 30, 0, 0, time.UTC), Actor: "LexLuthr", From: "remove", To: "fix(cli): remove deprecated filplus commands"}}, h.Renames)
	// GitHub lists edits newest first; they're sorted oldest first, and a deleted editor has no login.
	require.Equal(t, []bodyEdit{
		{At: time.Date(2026, 10, 26, 10, 0, 56, 0, time.UTC), Body: "original"},
		{At: time.Date(2026, 11, 1, 16, 20, 9, 0, time.UTC), Editor: "BigLep", Body: "current"},
	}, h.BodyEdits)

	// Edits made in the same second keep GitHub's order, and a version whose text was deleted is marked as such.
	h, err = parsePullRequestHistory([]byte(`{"data": {"repository": {"pullRequest": {
		"mergedAt": "2026-10-26T11:24:31Z",
		"userContentEdits": {"pageInfo": {"hasNextPage": false}, "nodes": [
			{"editedAt": "2026-11-01T16:20:09Z", "editor": {"login": "contributor"}, "diff": "sneaky"},
			{"editedAt": "2026-11-01T16:20:09Z", "editor": {"login": "maintainer"}, "diff": "noted"},
			{"editedAt": "2026-10-26T10:00:56Z", "editor": {"login": "contributor"}, "diff": null}
		]}
	}}}}`))
	require.NoError(t, err)
	require.Equal(t, []bodyEdit{
		{At: time.Date(2026, 10, 26, 10, 0, 56, 0, time.UTC), Editor: "contributor", Deleted: true},
		{At: time.Date(2026, 11, 1, 16, 20, 9, 0, time.UTC), Editor: "maintainer", Body: "noted"},
		{At: time.Date(2026, 11, 1, 16, 20, 9, 0, time.UTC), Editor: "contributor", Body: "sneaky"},
	}, h.BodyEdits)

	for _, tc := range []struct{ response, wantErr string }{
		{`{"data": {"repository": {"pullRequest": null}}}`, "PR not found"},
		{`{"data": {"repository": {"pullRequest": {"mergedAt": null}}}}`, "not merged"},
		{`{"data": {"repository": {"pullRequest": {"mergedAt": "2026-10-26T11:24:31Z", "userContentEdits": {"pageInfo": {"hasNextPage": true}}}}}}`, "more than 100"},
		{`{"errors": [{"message": "API rate limit exceeded"}]}`, "API rate limit exceeded"},
	} {
		_, err := parsePullRequestHistory([]byte(tc.response))
		require.ErrorContains(t, err, tc.wantErr)
	}
}
