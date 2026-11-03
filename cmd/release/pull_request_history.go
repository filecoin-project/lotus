package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Contributors can keep editing a PR's title and description after it merges, and GitHub notifies no one, so text
// read at release time could include changes no maintainer has seen. The changelog therefore uses a PR's title and
// description as they were at merge, updated only by later edits from people with write access, and stops when a
// PR's latest edit came from anyone else, so that a maintainer looks at it first.

// pullRequestHistory is a merged PR's current title, description and labels, and how its title and description
// changed over time.
type pullRequestHistory struct {
	MergedAt time.Time
	Title    string
	Body     string
	Labels   []string
	// Renames are the title changes, oldest first.
	Renames []titleRename
	// BodyEdits are full versions of the description, oldest first. It is empty when the description was never edited.
	BodyEdits []bodyEdit
}

type titleRename struct {
	At    time.Time
	Actor string
	From  string
	To    string
}

type bodyEdit struct {
	At     time.Time
	Editor string
	Body   string
	// Deleted is set when this version's text was deleted from the edit history, leaving only who edited it and when.
	Deleted bool
}

// trustedPullRequest returns the title and description the changelog should use for a PR: the versions at merge,
// updated by later edits from people hasWriteAccess accepts. It returns an error when the latest post-merge change
// to either came from someone else. An empty login is a deleted account, which is never trusted.
func trustedPullRequest(h pullRequestHistory, hasWriteAccess func(login string) (bool, error)) (pullRequestInfo, error) {
	trusted := func(login string) (bool, error) {
		if login == "" {
			return false, nil
		}
		return hasWriteAccess(login)
	}
	editedBy := func(login string) string {
		if login == "" {
			return "a deleted account"
		}
		return login
	}
	fix := "a maintainer must edit the PR to accept or revert the change"
	var problems []error

	// Renames and BodyEdits are oldest first, so everything from the first change after the merge onwards is post-merge.
	title := h.Title
	var untrustedRename *titleRename
	if first := slices.IndexFunc(h.Renames, func(r titleRename) bool { return r.At.After(h.MergedAt) }); first >= 0 {
		title = h.Renames[first].From
		for i := first; i < len(h.Renames); i++ {
			ok, err := trusted(h.Renames[i].Actor)
			if err != nil {
				return pullRequestInfo{}, err
			}
			if ok {
				title, untrustedRename = h.Renames[i].To, nil
			} else {
				untrustedRename = &h.Renames[i]
			}
		}
	}
	if untrustedRename != nil {
		problems = append(problems, fmt.Errorf("the title was changed after merge by %s at %s to %q; the changelog would use %q, so %s",
			editedBy(untrustedRename.Actor), untrustedRename.At.Format(time.RFC3339), untrustedRename.To, title, fix))
	}

	// Each edit holds the whole description, and the first is the original, so the last edit before the merge is the
	// description as it was merged.
	body, bodyDeleted := h.Body, false
	var untrustedEdit *bodyEdit
	// version returns the text of an edit. The newest edit is the current description, which is always readable.
	version := func(i int) (string, bool) {
		if i == len(h.BodyEdits)-1 {
			return h.Body, false
		}
		return h.BodyEdits[i].Body, h.BodyEdits[i].Deleted
	}
	if first := slices.IndexFunc(h.BodyEdits, func(e bodyEdit) bool { return e.At.After(h.MergedAt) }); first >= 0 {
		if first == 0 {
			return pullRequestInfo{}, errors.New("the description's edit history has no version from before the merge")
		}
		body, bodyDeleted = version(first - 1)
		for i := first; i < len(h.BodyEdits); i++ {
			ok, err := trusted(h.BodyEdits[i].Editor)
			if err != nil {
				return pullRequestInfo{}, err
			}
			if ok {
				body, bodyDeleted = version(i)
				untrustedEdit = nil
			} else {
				untrustedEdit = &h.BodyEdits[i]
			}
		}
	}
	// A deleted version is only ever needed when a later, untrusted edit follows it: otherwise the newest edit is used.
	if untrustedEdit != nil {
		trustedNote := fmt.Sprintf("%q", extractReleaseNote(body))
		if bodyDeleted {
			trustedNote = "a version deleted from the edit history"
		}
		problems = append(problems, fmt.Errorf("the description was edited after merge by %s at %s; its release note would change from %s to %q, so %s",
			editedBy(untrustedEdit.Editor), untrustedEdit.At.Format(time.RFC3339), trustedNote, extractReleaseNote(untrustedEdit.Body), fix))
	}

	return pullRequestInfo{Title: title, Body: body, Labels: h.Labels}, errors.Join(problems...)
}

// githubRequestTimeout bounds each request for a PR's history or a user's permission, so a stalled request can't hang
// generation.
const githubRequestTimeout = 30 * time.Second

// pullRequestHistoryQuery reads everything trustedPullRequest needs in one request.
const pullRequestHistoryQuery = `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      mergedAt
      title
      body
      labels(first: 100) { nodes { name } }
      timelineItems(itemTypes: [RENAMED_TITLE_EVENT], first: 100) {
        pageInfo { hasNextPage }
        nodes { ... on RenamedTitleEvent { createdAt actor { login } previousTitle currentTitle } }
      }
      userContentEdits(first: 100) {
        pageInfo { hasNextPage }
        nodes { editedAt editor { login } diff }
      }
    }
  }
}`

// parsePullRequestHistory decodes the response to pullRequestHistoryQuery.
func parsePullRequestHistory(response []byte) (pullRequestHistory, error) {
	type login struct {
		Login string `json:"login"`
	}
	type pageInfo struct {
		HasNextPage bool `json:"hasNextPage"`
	}
	var decoded struct {
		Data struct {
			Repository struct {
				PullRequest *struct {
					MergedAt *time.Time `json:"mergedAt"`
					Title    string     `json:"title"`
					Body     string     `json:"body"`
					Labels   struct {
						Nodes []struct {
							Name string `json:"name"`
						} `json:"nodes"`
					} `json:"labels"`
					TimelineItems struct {
						PageInfo pageInfo `json:"pageInfo"`
						Nodes    []struct {
							CreatedAt     time.Time `json:"createdAt"`
							Actor         *login    `json:"actor"`
							PreviousTitle string    `json:"previousTitle"`
							CurrentTitle  string    `json:"currentTitle"`
						} `json:"nodes"`
					} `json:"timelineItems"`
					UserContentEdits struct {
						PageInfo pageInfo `json:"pageInfo"`
						Nodes    []struct {
							EditedAt time.Time `json:"editedAt"`
							Editor   *login    `json:"editor"`
							Diff     *string   `json:"diff"`
						} `json:"nodes"`
					} `json:"userContentEdits"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		return pullRequestHistory{}, fmt.Errorf("failed to decode the PR history: %w", err)
	}
	if len(decoded.Errors) > 0 {
		var messages []string
		for _, e := range decoded.Errors {
			messages = append(messages, e.Message)
		}
		return pullRequestHistory{}, fmt.Errorf("failed to read the PR history: %s", strings.Join(messages, "; "))
	}
	pr := decoded.Data.Repository.PullRequest
	switch {
	case pr == nil:
		return pullRequestHistory{}, fmt.Errorf("PR not found")
	case pr.MergedAt == nil:
		return pullRequestHistory{}, fmt.Errorf("PR is not merged")
	case pr.TimelineItems.PageInfo.HasNextPage || pr.UserContentEdits.PageInfo.HasNextPage:
		return pullRequestHistory{}, fmt.Errorf("PR has more than 100 title changes or description edits; check it by hand")
	}

	loginOf := func(l *login) string {
		if l == nil {
			return ""
		}
		return l.Login
	}
	h := pullRequestHistory{MergedAt: *pr.MergedAt, Title: pr.Title, Body: pr.Body}
	for _, l := range pr.Labels.Nodes {
		h.Labels = append(h.Labels, l.Name)
	}
	for _, r := range pr.TimelineItems.Nodes {
		h.Renames = append(h.Renames, titleRename{At: r.CreatedAt, Actor: loginOf(r.Actor), From: r.PreviousTitle, To: r.CurrentTitle})
	}
	for _, e := range pr.UserContentEdits.Nodes {
		edit := bodyEdit{At: e.EditedAt, Editor: loginOf(e.Editor), Deleted: e.Diff == nil}
		if e.Diff != nil {
			edit.Body = *e.Diff
		}
		h.BodyEdits = append(h.BodyEdits, edit)
	}
	// GitHub lists edits newest first; reversing before the stable sort keeps edits made in the same second in order.
	slices.Reverse(h.BodyEdits)
	slices.SortStableFunc(h.Renames, func(a, b titleRename) int { return a.At.Compare(b.At) })
	slices.SortStableFunc(h.BodyEdits, func(a, b bodyEdit) int { return a.At.Compare(b.At) })
	return h, nil
}

// fetchPullRequestHistory reads a PR's history from GitHub's GraphQL API.
func fetchPullRequestHistory(ctx context.Context, token string, number int) (pullRequestHistory, error) {
	request, err := json.Marshal(map[string]any{
		"query":     pullRequestHistoryQuery,
		"variables": map[string]any{"owner": githubOwner, "repo": githubRepo, "number": number},
	})
	if err != nil {
		return pullRequestHistory{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(request))
	if err != nil {
		return pullRequestHistory{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return pullRequestHistory{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var response bytes.Buffer
	if _, err := response.ReadFrom(resp.Body); err != nil {
		return pullRequestHistory{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return pullRequestHistory{}, fmt.Errorf("GitHub GraphQL API returned %s", resp.Status)
	}
	return parsePullRequestHistory(response.Bytes())
}
