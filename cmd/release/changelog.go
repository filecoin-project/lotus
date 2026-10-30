package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/google/go-github/v66/github"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// The release-note labels a maintainer can apply to a PR to pull it out of its type's section.
const (
	labelUpgradeWarning = "release-note/upgrade-warning"
	labelHighlight      = "release-note/highlight"
)

// releaseNoteSection is a section of a generated release, in the order the sections are written.
type releaseNoteSection int

const (
	sectionUpgradeWarnings releaseNoteSection = iota
	sectionHighlights
	sectionNewFeatures
	sectionBugFixes
	sectionImprovements
	sectionOther
	sectionCount
)

var releaseNoteHeadings = [sectionCount]string{
	sectionUpgradeWarnings: "## ☢️ Upgrade Warnings ☢️",
	sectionHighlights:      "## ✨ Highlights",
	sectionNewFeatures:     "## ⭐ New Features",
	sectionBugFixes:        "## 🐛 Bug Fixes",
	sectionImprovements:    "## 👌 Improvements",
}

var (
	// prNumberRegexp matches the "(#N)" GitHub appends to a squash-merge commit subject.
	prNumberRegexp = regexp.MustCompile(`\s*\(#(\d+)\)\s*$`)
	// conventionalTitleRegexp matches the "type(scope)!:" prefix of a Conventional Commits title.
	conventionalTitleRegexp = regexp.MustCompile(`^([a-zA-Z]+)(\([^)]*\))?(!)?:`)
	// pullLinkRegexp matches a link to a lotus PR in CHANGELOG.md.
	pullLinkRegexp = regexp.MustCompile(`github\.com/filecoin-project/lotus/pull/(\d+)`)
	// releaseNoteHeadingRegexp matches the heading of the optional release-note block in a PR description.
	releaseNoteHeadingRegexp = regexp.MustCompile(`(?i)^##\s+release notes?\s*$`)
	htmlCommentRegexp        = regexp.MustCompile(`(?s)<!--.*?-->`)
	// majorVersionRegexp matches the major-version element at the end of a module path, e.g. the v4 of go-amt-ipld/v4.
	majorVersionRegexp = regexp.MustCompile(`^v[0-9]+$`)
)

// commitInfo is one commit in the range a release is generated from.
type commitInfo struct {
	Hash    string
	Author  string
	Subject string
	// PR is the number from the subject's trailing "(#N)", or 0 when there is none.
	PR         int
	Files      int
	Insertions int
	Deletions  int
}

// pullRequestInfo is what the generator reads from a PR on GitHub.
type pullRequestInfo struct {
	Title  string
	Body   string
	Labels []string
}

// compareLink is one project's line in a release's "Changelog" section.
type compareLink struct {
	Project string
	From    string
	To      string
}

// changelogRelease describes the release a section is generated for.
type changelogRelease struct {
	// Type is "Node", "Miner" or "Node and Miner", as in the section header.
	Type string
	// Version has no leading "v", e.g. 1.39.0 or 1.39.0-rc1.
	Version      string
	Date         string
	CompareLinks []compareLink
}

// parseGitLog parses the output of `git log --shortstat --format=%x1e%H%x1f%aN%x1f%s`.
func parseGitLog(output string) ([]commitInfo, error) {
	var commits []commitInfo
	for _, record := range strings.Split(output, "\x1e") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		header, stat, _ := strings.Cut(record, "\n")
		fields := strings.Split(header, "\x1f")
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected git log record %q", header)
		}
		commit := commitInfo{Hash: fields[0], Author: fields[1], Subject: fields[2]}
		if match := prNumberRegexp.FindStringSubmatch(commit.Subject); match != nil {
			commit.PR, _ = strconv.Atoi(match[1])
		}
		// A shortstat line looks like " 3 files changed, 10 insertions(+), 2 deletions(-)"; either count can be missing.
		for _, part := range strings.Split(strings.TrimSpace(stat), ",") {
			count, event, ok := strings.Cut(strings.TrimSpace(part), " ")
			if !ok {
				continue
			}
			n, err := strconv.Atoi(count)
			if err != nil {
				return nil, fmt.Errorf("unexpected shortstat %q for %s", stat, commit.Hash)
			}
			switch {
			case strings.HasPrefix(event, "file"):
				commit.Files = n
			case strings.HasPrefix(event, "insertion"):
				commit.Insertions = n
			case strings.HasPrefix(event, "deletion"):
				commit.Deletions = n
			}
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

// isSameBaseVersionHeader reports whether a section header is for version or another pre-release of the same base version.
// For 1.39.0-rc2 that is any of v1.39.0, v1.39.0-rc1, v1.39.0-rc2 and so on.
func isSameBaseVersionHeader(header, version string) bool {
	base := "v" + strings.SplitN(version, "-", 2)[0]
	for _, token := range strings.Fields(strings.ReplaceAll(header, "/", " ")) {
		if token == base || strings.HasPrefix(token, base+"-") {
			return true
		}
	}
	return false
}

// releasedPullRequests adds the PRs linked from released sections of a changelog to released.
// UNRELEASED sections are not released, and sections for version and its other pre-releases are left out,
// because each RC and the stable release list every change since the previous stable release.
func releasedPullRequests(changelog, version string, released map[int]bool) {
	for _, section := range splitChangelogSections(changelog) {
		header, _, _ := strings.Cut(section, "\n")
		if strings.HasPrefix(header, "# UNRELEASED") || isSameBaseVersionHeader(header, version) {
			continue
		}
		for _, match := range pullLinkRegexp.FindAllStringSubmatch(section, -1) {
			n, _ := strconv.Atoi(match[1])
			released[n] = true
		}
	}
}

// extractReleaseNote returns the content of the "## Release note" block in a PR description, without HTML comments.
// The block ends at the next heading of level one or two.
func extractReleaseNote(body string) string {
	var note []string
	inside := false
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if releaseNoteHeadingRegexp.MatchString(strings.TrimSpace(line)) {
			inside = true
			continue
		}
		if inside && (strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ")) {
			break
		}
		if inside {
			note = append(note, line)
		}
	}
	return strings.TrimSpace(htmlCommentRegexp.ReplaceAllString(strings.Join(note, "\n"), ""))
}

// classifyPullRequest picks the section a PR is listed in.
// The release-note labels and a breaking-change "!" come first, then the Conventional Commits type of the title.
func classifyPullRequest(title string, labels []string) releaseNoteSection {
	if slices.Contains(labels, labelUpgradeWarning) {
		return sectionUpgradeWarnings
	}
	if slices.Contains(labels, labelHighlight) {
		return sectionHighlights
	}
	match := conventionalTitleRegexp.FindStringSubmatch(title)
	if match == nil {
		return sectionImprovements
	}
	if match[3] == "!" {
		return sectionUpgradeWarnings
	}
	switch strings.ToLower(match[1]) {
	case "feat":
		return sectionNewFeatures
	case "fix":
		return sectionBugFixes
	case "build", "chore", "ci", "docs", "style", "test":
		return sectionOther
	default:
		return sectionImprovements
	}
}

// formatChangelogEntry formats a list item for a PR. Lines after the first are indented to stay in the item.
func formatChangelogEntry(text string, pr int) string {
	lines := strings.Split(strings.TrimPrefix(strings.TrimSpace(text), "- "), "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			lines[i] = "  " + lines[i]
		}
	}
	lines[len(lines)-1] += fmt.Sprintf(" ([filecoin-project/lotus#%d](https://github.com/filecoin-project/lotus/pull/%d))", pr, pr)
	return "- " + strings.Join(lines, "\n") + "\n"
}

// contributorStats is one row of a release's Contributors table.
type contributorStats struct {
	Author     string
	Commits    int
	Insertions int
	Deletions  int
	Files      int
}

func writeContributors(b *strings.Builder, commits []commitInfo) {
	byAuthor := map[string]*contributorStats{}
	for _, c := range commits {
		s, ok := byAuthor[c.Author]
		if !ok {
			s = &contributorStats{Author: c.Author}
			byAuthor[c.Author] = s
		}
		s.Commits++
		s.Insertions += c.Insertions
		s.Deletions += c.Deletions
		s.Files += c.Files
	}
	rows := make([]*contributorStats, 0, len(byAuthor))
	for _, s := range byAuthor {
		rows = append(rows, s)
	}
	sort.Slice(rows, func(i, j int) bool {
		li, lj := rows[i].Insertions+rows[i].Deletions, rows[j].Insertions+rows[j].Deletions
		if li != lj {
			return li > lj
		}
		return rows[i].Author < rows[j].Author
	})
	b.WriteString("## 👨‍👩‍👧‍👦 Contributors\n\n")
	b.WriteString("| Contributor | Commits | Lines ± | Files Changed |\n")
	b.WriteString("|-------------|---------|---------|---------------|\n")
	for _, s := range rows {
		_, _ = fmt.Fprintf(b, "| %s | %d | +%d/-%d | %d |\n", s.Author, s.Commits, s.Insertions, s.Deletions, s.Files)
	}
}

// dependencyPrefix selects the dependencies whose commits count towards a release's Contributors table.
const dependencyPrefix = "github.com/filecoin-project/"

// dependencyChange is a dependency whose version differs between the go.mod files at the ends of a release's range.
type dependencyChange struct {
	// Repo is the module path's repository, e.g. github.com/filecoin-project/go-state-types.
	Repo   string
	OldRef string
	NewRef string
}

// moduleVersionRef returns the git ref of a module version: the commit of a pseudo-version, or else the tag,
// which a module in a subdirectory of its repository prefixes with that subdirectory.
func moduleVersionRef(subdir, version string) string {
	if module.IsPseudoVersion(version) {
		if rev, err := module.PseudoVersionRev(version); err == nil {
			return rev
		}
	}
	version = strings.TrimSuffix(version, "+incompatible")
	if subdir != "" {
		return subdir + "/" + version
	}
	return version
}

// changedDependencies returns the required modules under prefix whose version differs between two go.mod files, sorted by repository.
func changedDependencies(oldGoMod, newGoMod []byte, prefix string) ([]dependencyChange, error) {
	oldFile, err := modfile.ParseLax("go.mod", oldGoMod, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the old go.mod: %w", err)
	}
	newFile, err := modfile.ParseLax("go.mod", newGoMod, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the new go.mod: %w", err)
	}
	oldVersions := map[string]string{}
	for _, r := range oldFile.Require {
		oldVersions[r.Mod.Path] = r.Mod.Version
	}
	var changes []dependencyChange
	seen := map[dependencyChange]bool{}
	for _, r := range newFile.Require {
		oldVersion, ok := oldVersions[r.Mod.Path]
		if !strings.HasPrefix(r.Mod.Path, prefix) || !ok || oldVersion == r.Mod.Version {
			continue
		}
		parts := strings.Split(r.Mod.Path, "/")
		if len(parts) < 3 {
			continue
		}
		// A trailing /vN is the major version, not a subdirectory.
		subdirParts := parts[3:]
		if n := len(subdirParts); n > 0 && majorVersionRegexp.MatchString(subdirParts[n-1]) {
			subdirParts = subdirParts[:n-1]
		}
		subdir := strings.Join(subdirParts, "/")
		change := dependencyChange{
			Repo:   strings.Join(parts[:3], "/"),
			OldRef: moduleVersionRef(subdir, oldVersion),
			NewRef: moduleVersionRef(subdir, r.Mod.Version),
		}
		if !seen[change] {
			seen[change] = true
			changes = append(changes, change)
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Repo < changes[j].Repo })
	return changes, nil
}

// dependencyCommits returns the commits between the old and new version of each dependency, from bare clones kept in cacheDir.
// A dependency that can't be cloned or resolved is left out with a warning.
func dependencyCommits(errOut io.Writer, cacheDir string, changes []dependencyChange) []commitInfo {
	var commits []commitInfo
	for _, change := range changes {
		dir := filepath.Join(cacheDir, change.Repo+".git")
		if _, err := os.Stat(dir); err != nil {
			_, _ = fmt.Fprintf(errOut, "Cloning %s for the Contributors table\n", change.Repo)
			if _, err := gitOutput("clone", "--quiet", "--bare", "https://"+change.Repo, dir); err != nil {
				_, _ = fmt.Fprintf(errOut, "warning: leaving %s out of the Contributors table: %v\n", change.Repo, err)
				continue
			}
		}
		rangeSpec := change.OldRef + ".." + change.NewRef
		if _, err := gitOutput("-C", dir, "rev-parse", "--verify", "--quiet", change.NewRef+"^{commit}"); err != nil {
			_, _ = gitOutput("-C", dir, "fetch", "--quiet", "--tags", "origin", "+refs/heads/*:refs/heads/*")
		}
		output, err := gitOutput("-C", dir, "log", "--no-merges", "--use-mailmap", "--shortstat", "--format=%x1e%H%x1f%aN%x1f%s", rangeSpec)
		if err == nil {
			var parsed []commitInfo
			if parsed, err = parseGitLog(output); err == nil {
				commits = append(commits, parsed...)
				continue
			}
		}
		_, _ = fmt.Fprintf(errOut, "warning: leaving %s %s out of the Contributors table: %v\n", change.Repo, rangeSpec, err)
	}
	return commits
}

// generateChangelogSection renders the CHANGELOG.md section for a release from the commits in its range, oldest first.
// Commits whose PR is in released are skipped: the previous tag is not always an ancestor of the release, so the
// range can hold changes that already shipped through a backport.
// fetchPR reads a PR from GitHub; when it fails, the entry falls back to the commit subject and a warning is logged.
// dependencyCommits are counted in the Contributors table only.
func generateChangelogSection(errOut io.Writer, release changelogRelease, commits []commitInfo, released map[int]bool, fetchPR func(int) (pullRequestInfo, error), dependencyCommits []commitInfo) string {
	var entries [sectionCount][]string
	var contributors []commitInfo
	listed := map[int]bool{}
	for _, c := range commits {
		if c.PR == 0 {
			_, _ = fmt.Fprintf(errOut, "warning: %s %q has no (#N) PR number, so it is not listed\n", c.Hash[:min(len(c.Hash), 12)], c.Subject)
			contributors = append(contributors, c)
			continue
		}
		if released[c.PR] {
			continue
		}
		contributors = append(contributors, c)
		if listed[c.PR] {
			continue
		}
		listed[c.PR] = true

		pr, err := fetchPR(c.PR)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "warning: using the commit subject for #%d: %v\n", c.PR, err)
			pr = pullRequestInfo{Title: prNumberRegexp.ReplaceAllString(c.Subject, "")}
		}
		section := classifyPullRequest(pr.Title, pr.Labels)
		text := pr.Title
		if note := extractReleaseNote(pr.Body); note != "" {
			text = note
		}
		entries[section] = append(entries[section], formatChangelogEntry(text, c.PR))
	}

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "# %s v%s / %s\n\n", release.Type, release.Version, release.Date)
	for section := sectionUpgradeWarnings; section < sectionOther; section++ {
		if len(entries[section]) == 0 && section != sectionHighlights {
			continue
		}
		b.WriteString(releaseNoteHeadings[section] + "\n\n")
		if len(entries[section]) == 0 {
			// The release issue asks the releaser to draft highlights here, or to delete the heading.
			b.WriteString("<!-- Draft the highlights of this release here, or delete this section. -->\n\n")
			continue
		}
		b.WriteString(strings.Join(entries[section], "") + "\n")
	}
	if len(entries[sectionOther]) > 0 {
		b.WriteString("<details><summary>Other changes</summary>\n\n")
		b.WriteString(strings.Join(entries[sectionOther], ""))
		b.WriteString("\n</details>\n\n")
	}
	b.WriteString("## 📝 Changelog\n\n")
	for _, link := range release.CompareLinks {
		prefix := "- " + link.Project + ": "
		if len(release.CompareLinks) == 1 {
			prefix = ""
		}
		_, _ = fmt.Fprintf(&b, "%s[Changes since %s](https://github.com/filecoin-project/lotus/compare/%s...%s)\n", prefix, link.From, link.From, link.To)
	}
	b.WriteString("\n")
	writeContributors(&b, append(contributors, dependencyCommits...))
	b.WriteString("\n")
	return b.String()
}

// insertChangelogSection adds section to changelog below its title and preamble, which must come first.
// An existing section for the same version and any of names is an error unless force is set, in which case it is replaced.
func insertChangelogSection(changelog, section, version string, names []string, force bool) (string, error) {
	sections := splitChangelogSections(changelog)
	for i, s := range sections {
		header, _, _ := strings.Cut(s, "\n")
		if strings.HasPrefix(header, "# UNRELEASED") || !slices.ContainsFunc(names, func(name string) bool { return isVersionHeader(header, "v"+version, name) }) {
			continue
		}
		if !force {
			return "", fmt.Errorf("the changelog already has a section for v%s (%q); pass --force to replace it", version, header)
		}
		sections[i] = section
		return strings.Join(sections, ""), nil
	}
	if len(sections) == 0 {
		return section, nil
	}
	return sections[0] + section + strings.Join(sections[1:], ""), nil
}

// previousStableTag returns the newest stable tag of a project that is older than version, or "" when there is none.
func previousStableTag(tags []string, name, version string) string {
	prefix := getPrefix(name)
	previous := ""
	for _, t := range tags {
		v, ok := strings.CutPrefix(t, prefix)
		if !ok || !semver.IsValid("v"+v) || isPrerelease(v) {
			continue
		}
		if semver.Compare("v"+v, "v"+version) < 0 && (previous == "" || semver.Compare("v"+v, "v"+previous) > 0) {
			previous = v
		}
	}
	if previous == "" {
		return ""
	}
	return prefix + previous
}

// githubToken returns $GITHUB_TOKEN, or else the token of the GitHub CLI.
func githubToken() (string, error) {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}
	output, err := exec.Command("gh", "auth", "token").Output()
	if token := strings.TrimSpace(string(output)); err == nil && token != "" {
		return token, nil
	}
	return "", fmt.Errorf("set GITHUB_TOKEN or log in with `gh auth login` to read PR titles, labels and descriptions")
}

// gitOutput runs git with args and returns its standard output.
func gitOutput(args ...string) (string, error) {
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

type changelogGenerateOptions struct {
	ReleaseType   string // node, miner or both
	Version       string
	From          string
	To            string
	Date          string
	ChangelogPath string
	Repo          string
	Force         bool
	DryRun        bool
	// SkipDependencies leaves commits in filecoin-project dependencies out of the Contributors table.
	SkipDependencies bool
}

// runChangelogGenerate generates the section for a release and writes it into the changelog, or to out for a dry run.
func runChangelogGenerate(out, errOut io.Writer, opts changelogGenerateOptions) error {
	version := strings.TrimPrefix(opts.Version, "v")
	if !semver.IsValid("v" + version) {
		return fmt.Errorf("invalid value for the 'tag' flag. Must be a valid semantic version (e.g. 1.39.0 or 1.39.0-rc1)")
	}
	var projects, names []string
	switch opts.ReleaseType {
	case "node":
		projects, names = []string{"node"}, []string{"Node"}
	case "miner":
		projects, names = []string{"miner"}, []string{"Miner"}
	case "both":
		projects, names = []string{"node", "miner"}, []string{"Node", "Miner"}
	default:
		return fmt.Errorf("invalid value for the 'type' flag. Allowed values are 'node', 'miner', and 'both'")
	}
	owner, repo, ok := strings.Cut(opts.Repo, "/")
	if !ok {
		return fmt.Errorf("invalid repository name format. Must be 'owner/repo'")
	}

	release := changelogRelease{Type: strings.Join(names, " and "), Version: version, Date: opts.Date}
	for i, project := range projects {
		previous := previousStableTag(getTags(), project, version)
		if previous == "" {
			return fmt.Errorf("no stable %s tag older than v%s; fetch tags with `git fetch origin --tags`", project, version)
		}
		release.CompareLinks = append(release.CompareLinks, compareLink{Project: names[i], From: previous, To: getPrefix(project) + version})
	}
	from := opts.From
	if from == "" {
		// The commit range follows the first project; for a combined release that is the node.
		from = release.CompareLinks[0].From
	}
	for _, ref := range []string{from, opts.To} {
		if _, err := gitOutput("rev-parse", "--verify", ref+"^{commit}"); err != nil {
			return fmt.Errorf("%s does not name a commit: %w", ref, err)
		}
	}
	_, _ = fmt.Fprintf(errOut, "Generating the v%s changelog from %s..%s\n", version, from, opts.To)

	changelog, err := os.ReadFile(opts.ChangelogPath)
	if err != nil {
		return fmt.Errorf("failed to read changelog: %w", err)
	}
	released := map[int]bool{}
	releasedPullRequests(string(changelog), version, released)
	// The previous release's own CHANGELOG.md has its section even if it was never copied back to this branch.
	if fromChangelog, err := gitOutput("show", from+":CHANGELOG.md"); err == nil {
		releasedPullRequests(fromChangelog, version, released)
	}

	gitLog, err := gitOutput("log", "--reverse", "--no-merges", "--use-mailmap", "--shortstat", "--format=%x1e%H%x1f%aN%x1f%s", from+".."+opts.To)
	if err != nil {
		return err
	}
	commits, err := parseGitLog(gitLog)
	if err != nil {
		return err
	}

	token, err := githubToken()
	if err != nil {
		return err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	fetchPR := func(number int) (pullRequestInfo, error) {
		pr, _, err := client.PullRequests.Get(context.Background(), owner, repo, number)
		if err != nil {
			return pullRequestInfo{}, err
		}
		info := pullRequestInfo{Title: pr.GetTitle(), Body: pr.GetBody()}
		for _, label := range pr.Labels {
			info.Labels = append(info.Labels, label.GetName())
		}
		return info, nil
	}

	var depCommits []commitInfo
	if !opts.SkipDependencies {
		depCommits, err = changedDependencyCommits(errOut, from, opts.To)
		if err != nil {
			return err
		}
	}

	section := generateChangelogSection(errOut, release, commits, released, fetchPR, depCommits)
	if opts.DryRun {
		_, err := io.WriteString(out, section)
		return err
	}
	updated, err := insertChangelogSection(string(changelog), section, version, names, opts.Force)
	if err != nil {
		return err
	}
	if err := os.WriteFile(opts.ChangelogPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to write changelog: %w", err)
	}
	_, _ = fmt.Fprintf(errOut, "Wrote the v%s section to %s\n", version, opts.ChangelogPath)
	return nil
}

// changedDependencyCommits returns the commits in filecoin-project dependencies whose version changed between from and to,
// cloning them into the user's cache directory.
func changedDependencyCommits(errOut io.Writer, from, to string) ([]commitInfo, error) {
	oldGoMod, err := gitOutput("show", from+":go.mod")
	if err != nil {
		return nil, err
	}
	newGoMod, err := gitOutput("show", to+":go.mod")
	if err != nil {
		return nil, err
	}
	changes, err := changedDependencies([]byte(oldGoMod), []byte(newGoMod), dependencyPrefix)
	if err != nil {
		return nil, err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to find a cache directory for dependency clones: %w", err)
	}
	return dependencyCommits(errOut, filepath.Join(cacheDir, "lotus-release"), changes), nil
}
