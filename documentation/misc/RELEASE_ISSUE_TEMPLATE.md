[//]: # (Below are non-visible steps intended for the issue creator)
<!--{{if .ContentGeneratedWithLotusReleaseCli}}-->
[//]: # (This content was generated using `{{.LotusReleaseCliString}}`.)
[//]: # (Learn more at https://github.com/filecoin-project/lotus/tree/master/cmd/release#readme.)
<!--{{end}}-->
[//]: # (Complete the steps below as part of creating a release issue and mark them complete with an X or checkmark when done.)
[//]: # (Agent/operator guide for completing this release issue:)
[//]: # (1. Treat this issue as the mutable release ledger. Edit it for concrete release progress: links, checked boxes, dates, release URLs, CI/release status, announcement comment links, and short release-specific facts.)
[//]: # (2. Put process/template improvements in the release-template-improvements PR from Release Setup, not directly in this issue.)
[//]: # (3. Work top-to-bottom. Do not start release PR work for a target until its Dependencies for releases section has linked blockers or an explicit "No additional dependencies" entry and the dependency checkpoint is complete.)
[//]: # (4. For regular releases, create release branches from origin/master after dependencies are resolved. For critical security patches, follow the visible release/vX.Y.x guidance.)
[//]: # (5. Keep the release issue and linked PRs synchronized as each step completes.)
[//]: # (6. Step-specific hints are in "agent:" HTML comments beneath the relevant checklist items. Read the raw issue body to see them.)
[//]: # (7. Never push to a release branch directly, even if your token can bypass the PR rule. Undoing a direct push needs a force-push, which branch protection blocks. The combined-release miner fast-forward is the only exception.)
[//]: # (8. Before treating a CI failure as a regression, check whether the same test also fails in master's recent CI runs.)
[//]: # (9. Rebase-merge backport PRs into the release branch.)
[//]: # (10. Squash-merge release PRs, which carry only the version bump and changelog, into the release branch. Fixes go through a backport PR from master, never the release PR.)
<!--{{if not .ContentGeneratedWithLotusReleaseCli}}-->
[//]: # ([ ] Start an issue with title "Lotus {{.Type}} v{{.Tag}} Release{{if .NetworkUpgrade}} (nv{{.NetworkUpgrade}}){{end}}" and adjust the title for whether it's a Node or Miner release.)
[//]: # ([ ] Copy in the content of https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md)
[//]: # ([ ] Find all the "go templating" "control" logic that is in \{\{ \}\} blocks and mimic the logic manually.)
[//]: # ([ ] Adjust the "Meta" section values)
[//]: # ([ ] Apply the `tpm` label to the issue)
[//]: # ([ ] Create the issue)
<!--{{end}}-->
<!-- At least as of 2025-03-20, it isn't possible to programmatically pin issues. -->
[//]: # ([ ] Pin the issue on GitHub)

# Meta
* Type: {{.Type}}
* Level: {{.Level}}
* Release flow: {{.ReleaseFlow}}<!--{{if ne .RequestedReleaseFlow .ReleaseFlow}}--> (resolved from {{.RequestedReleaseFlow}})<!--{{end}}-->
* Related network upgrade version: <!--{{if not .NetworkUpgrade}}-->n/a<!--{{else}}-->nv{{.NetworkUpgrade}}
   * Scope, dates, and epochs: {{.NetworkUpgradeDiscussionLink}}
   * Lotus changelog with Lotus specifics: {{.NetworkUpgradeChangelogEntryLink}}
<!--{{end}}-->

# Estimated shipping date

[//]: # (If/when we know an exact date, remove the "Week of " prefix.)
[//]: # (If a date week is an estimate, annotate with "estimate".)

| Candidate | Expected Release Date | Release URL |
|-----------|-----------------------|-------------|
<!--{{if .RCRelease}}-->
| RC1 | {{.RC1DateString}} | |
<!--{{end}}-->
| Stable Release | {{.StableDateString}} | |

# Dependencies for releases
> [!NOTE]
> 1. This is the set of changes that need to make it in for a given release target.
> 2. They can be checked as done once they land in `master`.
> 3. They are presented here for quick reference, but backporting is tracked in the corresponding release checklist.

<!--{{range $target := .ReleaseTargets}}-->
## {{$target}}
- [ ] Add linked PRs/issues for changes that must land before {{$target}}, or write "No additional dependencies for {{$target}}" and mark the corresponding dependency checkpoint complete when confirmed.

<!--{{end}}-->
# Release Checklist

## Release Setup
<details open>
  <summary>Section</summary>

- [ ] Use the exact Go version from `go.mod` for generation and builds: `export GOTOOLCHAIN="go$(awk '$1 == "go" {print $2}' go.mod)"`
   <!-- agent:
   - Rerun this after every branch switch. A newer installed Go does not downgrade and can generate code that fails CI.
   -->

<!--{{if ne .NetworkUpgrade ""}}-->
- [ ] Make sure all [Lotus dependencies are updated to the correct versions for the network upgrade](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/Update_Dependencies_Lotus.md)
   - Link to Lotus PR:
<!--{{end}}-->
- [ ] Open PR against [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) with title `docs(release): v{{.Tag}} release template improvements` for improving future releases.
   - Link to PR:
   - Open this as a draft PR and use it to collect release-process improvements discovered while running this checklist.
   - Suggested branch: `docs/release-v{{.Tag}}-template-improvements`
   - This will get merged in a `Post-Release` step.
<!--{{if eq .Level "patch"}}-->
<!--  {{if contains "Node" .Type}}-->
- [ ] Fork a new `release/v{{.Tag}}` branch from the `master` branch and make any further release-related changes to this branch.
   - For regular releases, use `origin/master` after confirming every {{.FirstReleaseTarget}} dependency above has landed.
   - Suggested commands:
      ```sh
      git fetch origin master --tags
      git push origin origin/master:refs/heads/release/v{{.Tag}}
      git ls-remote --heads origin release/v{{.Tag}}
      ```
   - Note: For critical security patches, fork a new branch from the last stable `release/vX.Y.x` to expedite the release process.
<!--  {{end}}-->
<!--  {{if contains "Miner" .Type}}-->
- [ ] Fork a new `release/miner/v{{.Tag}}` branch from the `master` branch and make any further release-related changes to this branch.
   - For regular releases, use `origin/master` after confirming every {{.FirstReleaseTarget}} dependency above has landed.
   - Suggested commands:
      ```sh
      git fetch origin master --tags
      git push origin origin/master:refs/heads/release/miner/v{{.Tag}}
      git ls-remote --heads origin release/miner/v{{.Tag}}
      ```
   - Note: For critical security patches, fork a new branch from the last stable `release/vX.Y.x` to expedite the release process.
<!--  {{end}}-->
<!--{{end}}-->
<!--{{if eq .Level "minor"}}-->
<!--  {{if contains "Node" .Type}}-->
- [ ] Fork a new `release/v{{.Tag}}` branch from `master` and make any further release-related changes to this branch.
   - For regular releases, use `origin/master` after confirming every {{.FirstReleaseTarget}} dependency above has landed.
   - Suggested commands:
      ```sh
      git fetch origin master --tags
      git push origin origin/master:refs/heads/release/v{{.Tag}}
      git ls-remote --heads origin release/v{{.Tag}}
      ```
<!--  {{end}}-->
<!--  {{if contains "Miner" .Type}}-->
- [ ] Fork a new `release/miner/v{{.Tag}}` branch from `master` and make any further release-related changes to this branch.
   - For regular releases, use `origin/master` after confirming every {{.FirstReleaseTarget}} dependency above has landed.
   - Suggested commands:
      ```sh
      git fetch origin master --tags
      git push origin origin/master:refs/heads/release/miner/v{{.Tag}}
      git ls-remote --heads origin release/miner/v{{.Tag}}
      ```
<!--  {{end}}-->
<!--{{end}}-->
<!--{{if ne .Level "patch"}}-->
- `master` branch version string updates
   - [ ] Bump the version(s) in `build/version.go` to `v{{.NextTag}}-dev`.
<!--{{  if contains "Node" .Type}}-->
      - Ensure to update `NodeBuildVersion`
<!--{{  end}}-->
<!--{{  if contains "Miner" .Type}}-->
      - Ensure to update `MinerBuildVersion`
<!--{{  end}}-->
   - [ ] Run `make gen && make docsgen-cli` before committing changes.
   - [ ] `master` branch CHANGELOG updates
     - [ ] Change the `UNRELEASED` section header to `UNRELEASED v{{.Tag}}`
     - [ ] Set the `UNRELEASED v{{.Tag}}` section's content to be "_See https://github.com/filecoin-project/lotus/blob/release/v{{.Tag}}/CHANGELOG.md_"
     - [ ] Add a new `UNRELEASED` header to top.
   - [ ] Create a PR with title `build: update Lotus {{.Type}} version to v{{.NextTag}}-dev in master`
     - Link to PR:
   - [ ] Merge PR
<!--{{end}}-->
</details>

## RCs
<!--{{if .NoRCRelease}}-->
<details open>
  <summary>Section</summary>

- Skipped. This release issue uses the no-RC flow for a release with no related network upgrade.
- If release-owner review finds risk that needs soak time, regenerate or edit this issue with `--release-flow=rc`.

</details>
<!--{{end}}-->
<!--{{range $target := .ReleaseTargets}}-->
<!--  {{$stable := eq $target "Stable Release"}}-->
<!--  {{$tagSuffix := ""}}-->
<!--  {{if not $stable}}{{$tagSuffix = printf "-%s" $target}}{{end}}-->
<!--  {{if $stable}}-->
## Stable Release
<details{{if $.NoRCRelease}} open{{end}}>
  <summary>Section</summary>
<!--  {{else}}-->
### {{$target}}
<details>
  <summary>Section</summary>
<!--  {{end}}-->

> [!IMPORTANT]
> Make changes through PRs that target the release branch; never push to it directly.
<!--  {{if contains "Node" $.Type}}-->
> Node branch: `release/v{{$.Tag}}`
<!--  {{end}}-->
<!--  {{if contains "Miner" $.Type}}-->
> Miner branch: `release/miner/v{{$.Tag}}`
<!--  {{end}}-->

#### Release blockers and backports for {{$target}}
- [ ] All explicitly tracked items from `Dependencies for releases` have landed
<!--  {{if $stable}}-->
- [ ] Account for every unresolved `release/backport` item: included in this release, intentionally deferred and linked below, or no longer a blocker.
   - Deferred items:
<!--  {{end}}-->
<!--  {{if and $stable $.NoRCRelease}}-->
- [ ] No additional backport PR is needed because this no-RC release branch was created from `origin/master` after all included dependencies landed.
<!--  {{else if ne $target "rc1"}}-->
- [ ] Backported [everything with the "backport" label](https://github.com/filecoin-project/lotus/issues?q=label%3Arelease%2Fbackport+)
- [ ] Create a PR with title `build: backport changes for {{$.Type}} v{{$.Tag}}{{$tagSuffix}}`
   - Link to PR:
- [ ] Rebase-merge the backport PR.
   <!-- agent:
   - Rebase, not squash: each backported commit should stay traceable to its master PR.
   - If the release PR already exists, rebase it onto the updated release branch.
   - Land later fixes as another small backport PR, not in the release PR.
   -->
- [ ] Remove the "backport" label from all backported PRs (no ["backport" issues](https://github.com/filecoin-project/lotus/issues?q=label%3Arelease%2Fbackport+))
<!--  {{end}}-->

#### Release PR for {{$target}}
- [ ] Update the version string(s) in `build/version.go` to `{{$.Tag}}{{$tagSuffix}}` (without a leading `v`).
<!--  {{if contains "Node" $.Type}}-->
    - Change `NodeBuildVersion` to `{{$.Tag}}{{$tagSuffix}}`
<!--  {{end}}-->
<!--  {{if contains "Miner" $.Type}}-->
    - Change `MinerBuildVersion` to `{{$.Tag}}{{$tagSuffix}}`
<!--  {{end}}-->
    - The release tags include the leading `v`; the values in `build/version.go` do not.
<!--  {{if and (contains "Node" $.Type) (contains "Miner" $.Type)}}-->
    - If the release branches still point at the same commit, one PR that updates both version strings is expected. If the branches diverge later, add/link one PR per branch here.
<!--  {{end}}-->
- [ ] Run `make clean && make deps` so FFI is rebuilt for the pinned submodule commit.
   <!-- agent:
   - Stale make stamps can silently skip the FFI rebuild.
   - `git submodule status extern/filecoin-ffi` must show no leading `+`, `-` or `U`.
   - After committing, build each release binary and confirm `--version` reports the intended version with no `.dirty` suffix.
   -->
- [ ] Run `make gen && make docsgen-cli` to generate documentation
- [ ] Create a draft PR with title `build: release Lotus {{$.Type}} v{{$.Tag}}{{$tagSuffix}}`
   - Link to PR:
   - Opening a PR will trigger a CI run that will build assets, create a draft GitHub release, and attach the assets.
- [ ] Changelog prep
   - [ ] Add a dated `# {{$.Type}} v{{$.Tag}}{{$tagSuffix}} / {date}` entry below `# UNRELEASED` with a short summary, and move this release's entries into it.
      <!-- agent:
      - Keep the `# UNRELEASED` header and its empty subsection headers.
      - When promoting an RC, carry the RC entry's content forward plus later fixes; UNRELEASED is already empty by then.
      - Each change appears once. Check the previous stable release so already-released changes are not presented as new.
      - Never edit historical entries.
      - End with `## 📝 Changelog` (compare link `PREVIOUS_TAG...TARGET_TAG` using tags, not branches) and `## 👨‍👩‍👧‍👦 Contributors` (from `./scripts/mkreleaselog PREVIOUS_TAG HEAD`).
      - Keep changelog edits out of cherry-picked fix commits so the post-release copy to master is clean.
      -->
   - [ ] After pushing CHANGELOG edits, check the draft GitHub release body. Every Release workflow run regenerates it from `CHANGELOG.md`, so fix the text there, not on GitHub.
      <!-- agent:
      - TAG is `v{{$.Tag}}{{$tagSuffix}}` for node and `miner/v{{$.Tag}}{{$tagSuffix}}` for miner.
      - View with `gh release view TAG --repo filecoin-project/lotus --json body -q .body`.
      - The workflow uses the CHANGELOG section whose header has this project's name and the version, so keep the header format above.
      - Publishing fails, leaving a draft, if no section with content matches.
      -->
   - [ ] Perform editorial review (e.g., callout breaking changes, new features, FIPs, actor bundles)
<!--  {{if ne $.NetworkUpgrade ""}}-->
      <!-- agent:
      - Take FIP titles and scope from https://github.com/filecoin-project/FIPs; do not infer them.
      - Take migration durations from the mainnet benchmark comment for nv{{$.NetworkUpgrade}} in https://github.com/filecoin-project/lotus/issues/12432, and link that comment directly.
      - Compare with the previous network upgrade's CHANGELOG entry for wording and context.
      - If there are no benchmark numbers yet, leave a clearly marked TODO; never invent durations.
      - Recompute the upgrade epoch's time-zone link (World Time Buddy) for this release; never copy an earlier one.
      - World Time Buddy URLs need `sln=H-H+1` in the query string, where H is the UTC hour containing the epoch's timestamp; the page returns 500 without it.
      -->
<!--    {{if $stable}}-->
   - [ ] (network upgrade) Ensure the Mainnet upgrade epoch is specified.
<!--    {{else}}-->
   - [ ] (network upgrade) Specify whether the Calibration or Mainnet upgrade epoch has been specified or not yet.
      - Example where these weren't specified yet: [PR #12169](https://github.com/filecoin-project/lotus/pull/12169)
<!--    {{end}}-->
<!--  {{end}}-->
   - [ ] Ensure no missing content when spot checking git history
      - Find the previous stable tag first:
<!--  {{if contains "Node" $.Type}}-->
         - Node: `git tag -l 'v*' | grep -v '-' | sort -V -r | head -n 1`
<!--  {{end}}-->
<!--  {{if contains "Miner" $.Type}}-->
         - Miner: `git tag -l 'miner/v*' | grep -v '-' | sort -V -r | head -n 1`
<!--  {{end}}-->
      - Example command looking at git commits: `git log --oneline --graph PREVIOUS_TAG..HEAD`
      - Example GitHub UI search looking at merged PRs into master, where `YYYY-MM-DD` is the previous stable release publish date: https://github.com/filecoin-project/lotus/pulls?q=is%3Apr+base%3Amaster+merged%3A%3EYYYY-MM-DD
      - Example `gh` cli command looking at merged PRs into master and sorted by title to group similar areas: `gh pr list --repo filecoin-project/lotus --search "base:master merged:>YYYY-MM-DD" --json number,mergedAt,author,title | jq -r '.[] | [.number, .mergedAt, .author.login, .title] | @tsv' | sort -k4`
   - [ ] Update the PR with the commit(s) made to the CHANGELOG
<!--  {{if $stable}}-->
- [ ] Confirm the release PR CI is green, including release asset generation.
- [ ] Confirm the release owner approves publishing this stable release.
- [ ] Confirm any security-advisory staging needed for this release has an owner and follows [policy](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#security-fix-policy).
<!--  {{end}}-->
- [ ] Mark the PR "ready for review" (non-draft)
- [ ] Squash-merge the PR
   - Merging the PR will trigger a CI run that will build assets, attach the assets to the GitHub release, publish the GitHub release, and create the corresponding git tag.
- [ ] Wait for the post-merge Release workflow to finish, then verify each published release.
   <!-- agent:
   - TAG is `v{{$.Tag}}{{$tagSuffix}}` for node and `miner/v{{$.Tag}}{{$tagSuffix}}` for miner; inspect with `gh release view TAG --repo filecoin-project/lotus --json isDraft,isPrerelease,body,assets,targetCommitish`.
   - Body: starts with the output of `go run cmd/release/main.go changelog-section --project PROJECT --tag TAG`, followed by the Release Log.
   - Flags: not a draft; prerelease only for RCs; the node stable release is marked latest.
   - Tag: `git rev-list -n 1 TAG` is the squash-merge commit on the release branch.
   - Assets: the Linux amd64 and macOS arm64 archives plus their checksum files are attached.
   - Checksums: download one archive and its checksum file with `gh release download` and confirm they match.
   -->
<!--  {{if and (contains "Node" $.Type) (contains "Miner" $.Type)}}-->
- [ ] Fast-forward `release/miner/v{{$.Tag}}` to the node release commit, then check both releases again.
   <!-- agent:
   - `git fetch origin release/v{{$.Tag}} release/miner/v{{$.Tag}}`
   - `git merge-base --is-ancestor origin/release/miner/v{{$.Tag}} origin/release/v{{$.Tag}} && git push origin origin/release/v{{$.Tag}}:refs/heads/release/miner/v{{$.Tag}}`
   - If the ancestry check fails, reconcile through a PR; never force-push.
   -->
<!--  {{end}}-->
- [ ] Update `Estimated shipping date` table
- [ ] Comment on this issue announcing the release:
   - Link to issue comment:

#### Testing for {{$target}}

> [!NOTE]
> Link to any special steps for testing releases beyond ensuring CI is green. Steps can be inlined here or tracked elsewhere.

</details>

<!--{{end}}-->
## Post-Release
<details>
  <summary>Section</summary>

- [ ] Open a PR against `master` that copies the final stable CHANGELOG entry from the release branch. Title it `chore(release): copy v{{.Tag}} changelog back to master`
   - Link to PR:
<!--{{if contains "Node" .Type}}-->
   - Node source branch: `release/v{{.Tag}}`
<!--{{end}}-->
<!--{{if contains "Miner" .Type}}-->
   - Miner source branch: `release/miner/v{{.Tag}}`
<!--{{end}}-->
   - Change only `CHANGELOG.md`, and copy only the final stable entry; RC entries stay on the release branch.
   <!-- agent:
   - Release commits also bump versions and generated files, so do not cherry-pick them.
   - Extract the diff with `git diff RELEASE_COMMIT^ RELEASE_COMMIT -- CHANGELOG.md` and apply it with `git apply --3way`.
   - Replace the `UNRELEASED v{{.Tag}}` placeholder with the final entry.
   - Keep newer master-only entries under `UNRELEASED`, and remove an `UNRELEASED` entry only if it shipped in this release.
   - Confirm the diff touches only `CHANGELOG.md`, with no version rollback and no duplicate release heading.
   -->
- [ ] Finish updating/merging the [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) PR from `Release Setup` with any improvements determined from this latest release iteration.
- [ ] Review and approve the auto-generated PR in [lotus-docs](https://github.com/filecoin-project/lotus-docs/pulls) that updates the latest Lotus version information.
   - The PR comes from a daily schedule; to get it sooner, run [Bump Lotus Version](https://github.com/filecoin-project/lotus-docs/actions/workflows/update-version.yml).
   <!-- agent:
   - `gh workflow run update-version.yml --repo filecoin-project/lotus-docs`
   -->
- [ ] Review and approve the auto-generated PR in [homebrew-lotus](https://github.com/filecoin-project/homebrew-lotus/pulls) that updates the homebrew to the latest Lotus version.
   - Check the asset URLs, and compute the SHA-256 checksums from the published archives yourself.
   - The PR comes from a daily schedule; to get it sooner, run [Bump Lotus Version](https://github.com/filecoin-project/homebrew-lotus/actions/workflows/update-version.yml).
   <!-- agent:
   - `gh workflow run update-version.yml --repo filecoin-project/homebrew-lotus`
   - Check each formula this release updates: `Formula/lotus.rb` for node and `Formula/lotus-miner.rb` for miner.
   - Download that project's archives: `gh release download v{{.Tag}} --repo filecoin-project/lotus --pattern '*.tar.gz'` for node, and `gh release download miner/v{{.Tag}} --repo filecoin-project/lotus --pattern '*.tar.gz'` for miner.
   - Run `shasum -a 256 *.tar.gz` and compare with the `url` and `sha256` lines in `gh pr diff PR_NUMBER --repo filecoin-project/homebrew-lotus`.
   -->
- [ ] Stage any security advisories for future publishing per [policy](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#security-fix-policy).
</details>

# Contributors

See the final release notes!

# Do you have questions?

Leave a comment in this ticket!
