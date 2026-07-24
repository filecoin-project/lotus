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

[//]: # (If/when we know an exact date, remove the "week of".)
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
<!--{{else}}-->
<!--{{range $rc := .RCCandidates}}-->
<!--  {{$tagSuffix := printf "-%s" $rc}}-->
### {{$rc}}
<details>
  <summary>Section</summary>

> [!IMPORTANT]
> These PRs should be done in and target the relevant release branch for this issue.
<!--  {{if contains "Node" $.Type}}-->
> Node branch: `release/v{{$.Tag}}`
<!--  {{end}}-->
<!--  {{if contains "Miner" $.Type}}-->
> Miner branch: `release/miner/v{{$.Tag}}`
<!--  {{end}}-->

#### Backport PR for {{$rc}}
- [ ] All explicitly tracked items from `Dependencies for releases` have landed
<!--  {{if ne $rc "rc1"}}-->
- [ ] Backported [everything with the "backport" label](https://github.com/filecoin-project/lotus/issues?q=label%3Arelease%2Fbackport+)
- [ ] Create a PR with title `build: backport changes for {{$.Type}} v{{$.Tag}}{{$tagSuffix}}`
   - Link to PR:
- [ ] Merge PR
- [ ] Remove the "backport" label from all backported PRs (no ["backport" issues](https://github.com/filecoin-project/lotus/issues?q=label%3Arelease%2Fbackport+))
<!--  {{end}}-->

#### Release PR for {{$rc}}
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
- [ ] Run `make gen && make docsgen-cli` to generate documentation
- [ ] Create a draft PR with title `build: release Lotus {{$.Type}} v{{$.Tag}}{{$tagSuffix}}`
   - Link to PR:
   - Opening a PR will trigger a CI run that will build assets, create a draft GitHub release, and attach the assets.
- [ ] Changelog prep
   - [ ] After the draft release exists, copy the auto-generated release notes into the CHANGELOG.
      - Note: after a draft release exists, rerunning the [release workflow](https://github.com/filecoin-project/lotus/blob/master/.github/workflows/release.yml#L220-L229) preserves the existing draft release body. If editorial review changes release-note content in CHANGELOG, update the draft GitHub release body too before merge; the [push-triggered publish step](https://github.com/filecoin-project/lotus/blob/master/.github/workflows/release.yml#L307-L308) publishes that draft body.
<!--  {{if contains "Node" $.Type}}-->
      - Node release body: `gh release view v{{$.Tag}}{{$tagSuffix}} --repo filecoin-project/lotus --json body -q .body`
<!--  {{end}}-->
<!--  {{if contains "Miner" $.Type}}-->
      - Miner release body: `gh release view miner/v{{$.Tag}}{{$tagSuffix}} --repo filecoin-project/lotus --json body -q .body`
<!--  {{end}}-->
   - [ ] Perform editorial review (e.g., callout breaking changes, new features, FIPs, actor bundles)
<!--  {{if ne $.NetworkUpgrade ""}}-->
   - [ ] (network upgrade) Specify whether the Calibration or Mainnet upgrade epoch has been specified or not yet.
      - Example where these weren't specified yet: [PR #12169](https://github.com/filecoin-project/lotus/pull/12169)
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
- [ ] Mark the PR "ready for review" (non-draft)
- [ ] Merge the PR
   - Merging the PR will trigger a CI run that will build assets, attach the assets to the GitHub release, publish the GitHub release, and create the corresponding git tag.
- [ ] Update `Estimated shipping date` table
- [ ] Comment on this issue announcing the release:
   - Link to issue comment:

#### Testing for {{$rc}}

> [!NOTE]
> Link to any special steps for testing releases beyond ensuring CI is green. Steps can be inlined here or tracked elsewhere.

</details>
<!--{{end}}-->
<!--{{end}}-->

## Stable Release
<details{{if .NoRCRelease}} open{{end}}>
  <summary>Section</summary>

> [!IMPORTANT]
> These PRs should be done in and target the relevant release branch for this issue.
<!--{{if contains "Node" .Type}}-->
> Node branch: `release/v{{.Tag}}`
<!--{{end}}-->
<!--{{if contains "Miner" .Type}}-->
> Miner branch: `release/miner/v{{.Tag}}`
<!--{{end}}-->

#### Backport PR for Stable Release
- [ ] All explicitly tracked items from `Dependencies for releases` have landed
- [ ] Confirm there are no unresolved `release/backport` blockers unless they are intentionally deferred and linked here:
   - Deferred items:
<!--{{if .NoRCRelease}}-->
- [ ] No additional backport PR is needed because this no-RC release branch was created from `origin/master` after dependency resolution.
<!--{{else}}-->
- [ ] Backported [everything with the "backport" label](https://github.com/filecoin-project/lotus/issues?q=label%3Arelease%2Fbackport+)
- [ ] Create a PR with title `build: backport changes for {{.Type}} v{{.Tag}}`
   - Link to PR:
- [ ] Merge PR
- [ ] Remove the "backport" label from all backported PRs (no ["backport" issues](https://github.com/filecoin-project/lotus/issues?q=label%3Arelease%2Fbackport+))
<!--{{end}}-->

#### Release PR for Stable Release
- [ ] Update the version string(s) in `build/version.go` to `{{.Tag}}` (without a leading `v`).
<!--{{if contains "Node" .Type}}-->
    - Change `NodeBuildVersion` to `{{.Tag}}`
<!--{{end}}-->
<!--{{if contains "Miner" .Type}}-->
    - Change `MinerBuildVersion` to `{{.Tag}}`
<!--{{end}}-->
    - The release tags include the leading `v`; the values in `build/version.go` do not.
<!--{{if and (contains "Node" .Type) (contains "Miner" .Type)}}-->
    - If the release branches still point at the same commit, one PR that updates both version strings is expected. If the branches diverge later, add/link one PR per branch here.
<!--{{end}}-->
- [ ] Run `make gen && make docsgen-cli` to generate documentation
- [ ] Create a draft PR with title `build: release Lotus {{.Type}} v{{.Tag}}`
   - Link to PR:
   - Opening a PR will trigger a CI run that will build assets, create a draft GitHub release, and attach the assets.
- [ ] Changelog prep
   - [ ] After the draft release exists, copy the auto-generated release notes into the CHANGELOG.
      - Note: after a draft release exists, rerunning the [release workflow](https://github.com/filecoin-project/lotus/blob/master/.github/workflows/release.yml#L220-L229) preserves the existing draft release body. If editorial review changes release-note content in CHANGELOG, update the draft GitHub release body too before merge; the [push-triggered publish step](https://github.com/filecoin-project/lotus/blob/master/.github/workflows/release.yml#L307-L308) publishes that draft body.
<!--{{if contains "Node" .Type}}-->
      - Node release body: `gh release view v{{.Tag}} --repo filecoin-project/lotus --json body -q .body`
<!--{{end}}-->
<!--{{if contains "Miner" .Type}}-->
      - Miner release body: `gh release view miner/v{{.Tag}} --repo filecoin-project/lotus --json body -q .body`
<!--{{end}}-->
   - [ ] Perform editorial review (e.g., callout breaking changes, new features, FIPs, actor bundles)
<!--{{if ne .NetworkUpgrade ""}}-->
   - [ ] (network upgrade) Ensure the Mainnet upgrade epoch is specified.
<!--{{end}}-->
   - [ ] Ensure no missing content when spot checking git history
      - Find the previous stable tag first:
<!--{{if contains "Node" .Type}}-->
         - Node: `git tag -l 'v*' | grep -v '-' | sort -V -r | head -n 1`
<!--{{end}}-->
<!--{{if contains "Miner" .Type}}-->
         - Miner: `git tag -l 'miner/v*' | grep -v '-' | sort -V -r | head -n 1`
<!--{{end}}-->
      - Example command looking at git commits: `git log --oneline --graph PREVIOUS_TAG..HEAD`
      - Example GitHub UI search looking at merged PRs into master, where `YYYY-MM-DD` is the previous stable release publish date: https://github.com/filecoin-project/lotus/pulls?q=is%3Apr+base%3Amaster+merged%3A%3EYYYY-MM-DD
      - Example `gh` cli command looking at merged PRs into master and sorted by title to group similar areas: `gh pr list --repo filecoin-project/lotus --search "base:master merged:>YYYY-MM-DD" --json number,mergedAt,author,title | jq -r '.[] | [.number, .mergedAt, .author.login, .title] | @tsv' | sort -k4`
   - [ ] Review and update the draft GitHub release body so it matches the CHANGELOG.
   - [ ] Update the PR with the commit(s) made to the CHANGELOG
- [ ] Confirm the release PR CI is green, including release asset generation.
- [ ] Confirm the release owner approves publishing this stable release.
- [ ] Confirm any security-advisory staging needed for this release has an owner and follows [policy](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#security-fix-policy).
- [ ] Mark the PR "ready for review" (non-draft)
- [ ] Merge the PR
   - Merging the PR will trigger a CI run that will build assets, attach the assets to the GitHub release, publish the GitHub release, and create the corresponding git tag.
- [ ] Update `Estimated shipping date` table
- [ ] Comment on this issue announcing the release:
   - Link to issue comment:

#### Testing for Stable Release

> [!NOTE]
> Link to any special steps for testing releases beyond ensuring CI is green. Steps can be inlined here or tracked elsewhere.

</details>

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
   - Assuming we followed [the process of merging changes into `master` first before backporting to the release branch](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#branch-and-tag-strategy), the only changes should be CHANGELOG updates.
- [ ] Finish updating/merging the [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) PR from `Release Setup` with any improvements determined from this latest release iteration.
- [ ] Review and approve the auto-generated PR in [lotus-docs](https://github.com/filecoin-project/lotus-docs/pulls) that updates the latest Lotus version information.
- [ ] Review and approve the auto-generated PR in [homebrew-lotus](https://github.com/filecoin-project/homebrew-lotus/pulls) that updates the homebrew to the latest Lotus version.
- [ ] Stage any security advisories for future publishing per [policy](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#security-fix-policy).
</details>

# Contributors

See the final release notes!

# Do you have questions?

Leave a comment in this ticket!
