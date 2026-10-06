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

- [ ] Use the exact Go version from `go.mod` for generation and builds: `export GOTOOLCHAIN="go$(awk '$1 == "go" {print $2}' go.mod)"`. Recompute this after switching branches; a newer installed Go does not automatically downgrade to the version used by CI.

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
> Work on a feature branch and open PRs targeting the relevant release branch. Do not push release changes directly to a release branch, even if branch protection can be bypassed.
<!--  {{if and (contains "Node" $.Type) (contains "Miner" $.Type)}}-->
> The verified fast-forward for a combined release below is the exception.
<!--  {{end}}-->
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
- [ ] Squash-merge the backport PR, then base the release PR on the updated release branch.
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
- [ ] Run `make clean && make deps` before generating and building to remove stale dependency stamps and rebuild FFI for the pinned submodule commit.
   - Check `git submodule status extern/filecoin-ffi`: it must have no leading `+`, `-`, or `U`.
   - After committing the release changes, build each release binary and verify its `--version` reports the intended version without a `.dirty` suffix.
- [ ] Run `make gen && make docsgen-cli` to generate documentation
- [ ] Create a draft PR with title `build: release Lotus {{$.Type}} v{{$.Tag}}{{$tagSuffix}}`
   - Link to PR:
   - Opening a PR will trigger a CI run that will build assets, create a draft GitHub release, and attach the assets.
- [ ] Changelog prep
   - [ ] Prepare `# {{$.Type}} v{{$.Tag}}{{$tagSuffix}} / {date}` below `# UNRELEASED`, with a short release summary and the relevant warnings, features, fixes, and improvements.
      - Move this release's entries out of `UNRELEASED`, keeping that header and its empty subsection headers. When promoting an RC, carry its release content forward into the new entry along with subsequent fixes; do not lose it by copying only the now-empty `UNRELEASED` section.
      - Include each change once in the new entry. Check against the previous stable release and git history so already-released changes are not presented as new; preserve historical release entries.
      - Add `## 📝 Changelog` and `## 👨‍👩‍👧‍👦 Contributors` sections using the actual previous stable tag for each project and the intended release tag. Use tags in compare links (`PREVIOUS_TAG...TARGET_TAG`), not RC-named release branches. Generate contributor statistics with `./scripts/mkreleaselog PREVIOUS_TAG HEAD`.
      - Keep changelog edits separate from unrelated fixes where practical, so the final release diff is easy to review and copy back to `master`.
   - [ ] Copy the reviewed dated entry into each draft GitHub release body, and repeat after editorial changes: `gh release edit TAG --repo filecoin-project/lotus --notes-file RELEASE_NOTES_FILE`.
      - Check the workflow on the release branch: existing nonempty draft bodies are currently reused even during publication, and a combined `Node and Miner` heading is not matched by the current parser. Do not assume a rerun, a recreated draft, or the publish step will recover the dated entry. [#13816](https://github.com/filecoin-project/lotus/pull/13816) proposes fixes but must be merged and included in the release branch before relying on them.
      - If a draft must be recreated, supply the reviewed notes explicitly with `gh release create TAG --repo filecoin-project/lotus --draft --title TAG --notes-file RELEASE_NOTES_FILE`, then verify its body and assets before publishing.
<!--  {{if contains "Node" $.Type}}-->
      - Node release body: `gh release view v{{$.Tag}}{{$tagSuffix}} --repo filecoin-project/lotus --json body -q .body`
<!--  {{end}}-->
<!--  {{if contains "Miner" $.Type}}-->
      - Miner release body: `gh release view miner/v{{$.Tag}}{{$tagSuffix}} --repo filecoin-project/lotus --json body -q .body`
<!--  {{end}}-->
   - [ ] Perform editorial review (e.g., callout breaking changes, new features, FIPs, actor bundles)
<!--  {{if ne $.NetworkUpgrade ""}}-->
      - Read the linked [FIP documents](https://github.com/filecoin-project/FIPs) for the exact titles and scope; do not infer titles from the change descriptions.
      - Cite actual migration benchmarks and their hardware/context. If measurements are missing, leave a clearly marked TODO for the release owner instead of inventing durations.
      - Verify the upgrade epoch's UTC timestamp and open the local-time link to check its date/time. For World Time Buddy, include and recompute `sln` for this upgrade; do not copy an earlier release's hour range.
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
- [ ] Merge the PR
   - Merging the PR will trigger a CI run that will build assets, attach the assets to the GitHub release, publish the GitHub release, and create the corresponding git tag.
- [ ] Wait for the post-merge Release workflow to finish, then verify each published release's tag commit, binaries, checksums, and complete notes against the reviewed CHANGELOG entry.
<!--  {{if and (contains "Node" $.Type) (contains "Miner" $.Type)}}-->
- [ ] If one PR released both projects from the node branch, fast-forward the miner branch only after that Release workflow completes successfully. Both branch pushes can otherwise try to update the same releases concurrently.
   - Fetch both branches and confirm the miner branch is an ancestor of the node release commit before pushing:
      ```sh
      git fetch origin release/v{{$.Tag}} release/miner/v{{$.Tag}}
      git merge-base --is-ancestor origin/release/miner/v{{$.Tag}} origin/release/v{{$.Tag}} && git push origin origin/release/v{{$.Tag}}:refs/heads/release/miner/v{{$.Tag}}
      ```
   - If ancestry verification fails, reconcile the branches through a PR; do not force-push. Wait for any second Release workflow and verify both branch heads, release tags, assets, and bodies again.
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

- [ ] Open a PR against `master` cherry-picking the CHANGELOG commits from the release branch. Title it `chore(release): cherry-pick v{{.Tag}} changelog back to master`
   - Link to PR:
<!--{{if contains "Node" .Type}}-->
   - Node source branch: `release/v{{.Tag}}`
<!--{{end}}-->
<!--{{if contains "Miner" .Type}}-->
   - Miner source branch: `release/miner/v{{.Tag}}`
<!--{{end}}-->
   - Bring back only `CHANGELOG.md` changes. If a release commit also changes versions or generated files, extract its changelog diff (for example, `git diff RELEASE_COMMIT^ RELEASE_COMMIT -- CHANGELOG.md`) and apply that diff with `git apply --3way` instead of cherry-picking the whole commit.
   - Replace the `UNRELEASED v{{.Tag}}` placeholder (if present) with the final dated release entry, preserving newer `master`-only entries under `UNRELEASED` and all historical releases. Remove entries from `UNRELEASED` only when they are included in the released entry; resolve conflicts by checking both histories.
   - Confirm the PR changes only `CHANGELOG.md`, with each released change appearing once in the final entry and no version rollback or duplicate release heading.
- [ ] Finish updating/merging the [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) PR from `Release Setup` with any improvements determined from this latest release iteration.
- If a version-update PR is missing, trigger that repository's version-bump workflow manually, then review the generated diff.
- [ ] Review and approve the auto-generated PR in [lotus-docs](https://github.com/filecoin-project/lotus-docs/pulls) that updates the latest Lotus version information.
- [ ] Review and approve the auto-generated PR in [homebrew-lotus](https://github.com/filecoin-project/homebrew-lotus/pulls) that updates the homebrew to the latest Lotus version.
   - Verify the formula's asset URLs and independently calculate SHA-256 checksums from the published archives before approving.
- [ ] Stage any security advisories for future publishing per [policy](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#security-fix-policy).
</details>

# Contributors

See the final release notes!

# Do you have questions?

Leave a comment in this ticket!
