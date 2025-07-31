[//]: # (Below are non-visible steps intended for the issue creator)
<!--{{if .ContentGeneratedWithLotusReleaseCli}}-->
[//]: # (This content was generated using `{{.LotusReleaseCliString}}`.)
[//]: # (Learn more at https://github.com/filecoin-project/lotus/tree/master/cmd/release#readme.)
<!--{{end}}-->
[//]: # (❗️ Complete the steps below as part of creating a release issue and mark them complete with an X or ✅ when done.)
<!--{{if not .ContentGeneratedWithLotusReleaseCli}}-->
[//]: # ([ ] Start an issue with title "Lotus {{.Type}} v{{.Tag}} Release" and adjust the title for whether it's a Node or Miner release.)
[//]: # ([ ] Copy in the content of https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md)
[//]: # ([ ] Find all the "go templating" "control" logic that is in \{\{ \}\} blocks and mimic the logic manually.)
[//]: # ([ ] Adjust the "Meta" section values)
[//]: # ([ ] Apply the `tpm` label to the issue)
[//]: # ([ ] Create the issue)
<!--{{end}}-->
<!-- At least as of 2025-03-20, it isn't possible to programmatically pin issues. -->
[//]: # ([ ] Pin the issue on GitHub)

# 😶‍🌫 Meta
* Type: {{.Type}} 
* Level: {{.Level}}
* Related network upgrade version: <!--{{if not .NetworkUpgrade}}-->n/a<!--{{else}}-->nv{{.NetworkUpgrade}}
   * Scope, dates, and epochs: {{.NetworkUpgradeDiscussionLink}}
   * Lotus changelog with Lotus specifics: {{.NetworkUpgradeChangelogEntryLink}}
<!--{{end}}-->

# 🚢 Estimated shipping date

[//]: # (If/when we know an exact date, remove the "week of".)
[//]: # (If a date week is an estimate, annotate with "estimate".)

| Candidate | Expected Release Date | Release URL |
|-----------|-----------------------|-------------|
| RC1 | {{.RC1DateString}} | |
| Stable (non-RC) | {{.StableDateString}} | |

# 🪢 Dependencies for releases
> [!NOTE]
> 1. This is the set of changes that need to make it in for a given RC.  This is effectively the set of changes to cherry-pick from master.
> 2. They can be checked as done once they land in `master`.
> 3. They are presented here for quick reference, but backporting is tracked in each `Release Checklist`.

<!--{{/* Sprig is used for defining a list per https://stackoverflow.com/a/57959333 */}}-->
<!--{{$rcVersions := list "rc1" "rcX" "Stable Release (non-RC)"}}-->
<!--{{range $rc := $rcVersions}}-->
## {{$rc}}
- [ ] To Be Added

<!--{{end}}-->
# ✅ Release Checklist

## ⬅️  Before RC1
<details open>
  <summary>Section</summary>

<!--{{if ne .NetworkUpgrade ""}}-->
- [ ] Make sure all [Lotus dependencies are updated to the correct versions for the network upgrade](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/Update_Dependencies_Lotus.md)
   - Link to Lotus PR:
<!--{{end}}-->
- [ ] Open PR against [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) with title `docs(release): v{{.Tag}} release template improvements` for improving future releases.
   - Link to PR:
   - There likely aren't any changes at this point, but this can be opened with a small whitespace change so the PR is open and we can more easily hold the standard of making improvements incrementally since improvements are usually better done by collecting changes/notes along the way rather than just thinking about it at the end. 
   - This will get merged in a `Post Release` step.
<!--{{if eq .Level "patch"}})-->
<!--  {{if contains "Node" .Type}}-->
- [ ] Fork a new `release/v{{.Tag}}` branch from the `master` branch and make any further release-related changes to this branch.
   - Note: For critical security patches, fork a new branch from the last stable `release/vX.Y.x` to expedite the release process.
<!--  {{end}}-->
<!--  {{if contains "Miner" .Type}}-->
- [ ] Fork a new `release/miner/v{{.Tag}}` branch from the `master` branch and make any further release-related changes to this branch.
   - Note: For critical security patches, fork a new branch from the last stable `release/vX.Y.x` to expedite the release process.
<!--  {{end}}-->
<!--{{end}}-->
<!--{{if eq .Level "minor"}}-->
<!--  {{if contains "Node" .Type}}-->
- [ ] Fork a new `release/v{{.Tag}}` branch from `master` and make any further release-related changes to this branch.
<!--  {{end}}-->
<!--  {{if contains "Miner" .Type}}-->
- [ ] Fork a new `release/miner/v{{.Tag}}` branch from `master` and make any further release-related changes to this branch.
<!--  {{end}}-->
<!--{{end}}-->
<!--{{if ne .Level "patch"}}-->
- `master` branch Version string updates
   - [ ] bump the version(s) in `build/version.go` to `v{{.NextTag}}-dev`.
<!--{{  if contains "Node" .Type}}-->
      - Ensure to update `NodeBuildVersion`
<!--{{  end}}-->
<!--{{  if contains "Miner" .Type}}-->
      - Ensure to update `MinerBuildVersion`
<!--{{  end}}-->
   - [ ] Run `make gen && make docsgen-cli` before committing changes.
   - [ ] Update the CHANGELOG
     - [ ] Change the `UNRELEASED` section header to `UNRELEASED v{{.Tag}}`
     - [ ] Set the `UNRELEASED v{{.Tag}}` section's content to be "_See https://github.com/filecoin-project/lotus/blob/release/v{{.Tag}}/CHANGELOG.md_"
     - [ ] Add a new `UNRELEASED` header to top.
   - [ ] Create a PR with title `build: update Lotus {{.Type}} version to v{{.NextTag}}-dev in master`
     - Link to PR:
   - [ ] Merge PR
<!--{{end}}-->
</details>

## 🏎️  RCs

<!--{{range $rc := $rcVersions}}-->
<!--  {{$tagSuffix := ""}}-->
<!--  {{if contains "rc" $rc}}-->
<!--    {{$tagSuffix = printf "-%s" $rc}}-->
<!--  {{end}}-->
### {{$rc}}
<details>
  <summary>Section</summary>

> [!IMPORTANT]
> These PRs should be done in and target the `release/v{{$.Tag}}` or `release/miner/v{{$.Tag}}` branch.

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
- [ ] Update the version string(s) in `build/version.go` to one {{if contains "rc" $rc}}ending with '-{{$rc}}'{{else}}**NOT** ending with 'rcX'{{end}}.
<!--  {{if contains "Node" $.Type}}-->
    - Ensure to update `NodeBuildVersion`
<!--  {{end}}-->
<!--  {{if contains "Miner" $.Type}}-->
    - Ensure to update `MinerBuildVersion`
<!--  {{end}}-->
- [ ] Run `make gen && make docsgen-cli` to generate documentation
- [ ] Create a draft PR with title `build: release Lotus {{$.Type}} v{{$.Tag}}{{$tagSuffix}}`
   - Link to PR:
   - Opening a PR will trigger a CI run that will build assets, create a draft GitHub release, and attach the assets.
- [ ] Changelog prep
   - [ ] Go to the [releases page](https://github.com/filecoin-project/lotus/releases) and copy the auto-generated release notes into the CHANGELOG
   - [ ] Perform editorial review (e.g., callout breaking changes, new features, FIPs, actor bundles)
<!--  {{if ne $.NetworkUpgrade ""}}-->
<!--    {{if contains "rc" $rc}}-->
   - [ ] (network upgrade) Specify whether the Calibration or Mainnet upgrade epoch has been specified or not yet.
      - Example where these weren't specified yet: [PR #12169](https://github.com/filecoin-project/lotus/pull/12169)
<!--    {{else}}-->
   - [ ] (network upgrade) Ensure the Mainnet upgrade epoch is specified.
<!--    {{end}}-->
<!--  {{end}}-->
   - [ ] Ensure no missing content when spot checking git history
      - Example command looking at git commits: `git log --oneline --graph vA.B.C..`, where A.B.C correspond to the previous release.
      - Example GitHub UI search looking at merged PRs into master: https://github.com/filecoin-project/lotus/pulls?q=is%3Apr+base%3Amaster+merged%3A%3EYYYY-MM-DD
      - Example `gh` cli command looking at merged PRs into master and sorted by title to group similar areas (where `YYYY-MM-DD` is the start search date): `gh pr list --repo filecoin-project/lotus --search "base:master merged:>YYYY-MM-DD" --json number,mergedAt,author,title | jq -r '.[] | [.number, .mergedAt, .author.login, .title] | @tsv' | sort -k4`
    - [ ] Update the PR with the commit(s) made to the CHANGELOG
- [ ] Mark the PR "ready for review" (non-draft)
- [ ] Merge the PR
   - Merging the PR will trigger a CI run that will build assets, attach the assets to the GitHub release, publish the GitHub release, and create the corresponding git tag.
 - [ ] Update `🚢 Estimated shipping date` table
 - [ ] Comment on this issue announcing the release:
    - Link to issue comment:

#### Testing for {{$rc}}

> [!NOTE]
> Link to any special steps for testing releases beyond ensuring CI is green.  Steps can be inlined here or tracked elsewhere.

</details>
<!--{{end}}-->

## ➡ Post-Release
<details>
  <summary>Section</summary>

- [ ] Open a PR against `master` cherry-picking the CHANGELOG commits from the `release/v{{.Tag}}` branch. Title it `chore(release): cherry-pick v{{.Tag}} changelog back to master`
   - Link to PR:
   - Assuming we followed [the process of merging changes into `master` first before backporting to the release branch](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#branch-and-tag-strategy), the only changes should be CHANGELOG updates.
- [ ] Finish updating/merging the [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) PR from `Before RC1` with any improvements determined from this latest release iteration.
- [ ] Review and approve the auto-generated PR in [lotus-docs](https://github.com/filecoin-project/lotus-docs/pulls) that updates the latest Lotus version information.
- [ ] Review and approve the auto-generated PR in [homebrew-lotus](https://github.com/filecoin-project/homebrew-lotus/pulls) that updates the homebrew to the latest Lotus version.
- [ ] Stage any security advisories for future publishing per [policy](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#security-fix-policy).
</details>

# ❤️ Contributors

See the final release notes!

# ⁉️ Do you have questions?

Leave a comment in this ticket!
