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
> These PRs should be done in and target the relevant release branch for this issue.
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
- [ ] Merge PR
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
- [ ] Run `make clean` before generating and building, especially in a long-lived clone. `build/.update-modules` and `build/.filecoin-install` are stamp files that can be newer than the actual submodule commit, which makes `make deps` skip `git submodule update --init --recursive` and silently link against a stale `libfilcrypto.a`. `make clean` removes both stamps and also runs `filecoin-ffi`'s own clean target.
   - After `make deps`, `git submodule status` should show no leading `+` on `extern/filecoin-ffi`.
   - The built version string should have no `.dirty` suffix (check with the built binary's `--version`).
- [ ] Run `make gen && make docsgen-cli` to generate documentation
- [ ] Create a draft PR with title `build: release Lotus {{$.Type}} v{{$.Tag}}{{$tagSuffix}}`
   - Link to PR:
   - Opening a PR will trigger a CI run that will build assets, create a draft GitHub release, and attach the assets.
- [ ] Changelog prep
   - [ ] Convert the top of `CHANGELOG.md` from a plain `UNRELEASED` section into the dated release entry, **without renaming or removing the `# UNRELEASED` header**:
      1. Insert a new `# {{$.Type}} v{{$.Tag}}{{$tagSuffix}} / {date}` section directly below `# UNRELEASED`, with a one-sentence summary paragraph.
      2. Move each bullet from `# UNRELEASED`'s subsections (`Upgrade Warnings`, `New Features`, `Bug Fixes`, `Improvements`) down into the matching subsection under the new dated section. Do not duplicate content between the two, and do not add anything beyond what is already in `UNRELEASED` at this point (confirm any additions with the release owner first).
      3. Leave `# UNRELEASED` in place above the dated section, with its subsection headers present but now empty of bullets. See `release/v1.36.2`'s tip for a worked example of the expected shape.
      4. Add `## 📝 Changelog` (compare link against the previous stable tag found earlier, e.g. `https://github.com/filecoin-project/lotus/compare/release/vPREVIOUS...release/v{{$.Tag}}{{$tagSuffix}}`) and `## 👨‍👩‍👧‍👦 Contributors` (commit/lines/files-changed table) sections at the end of the dated entry.
      - **Why the `# UNRELEASED` header must stay:** the [release workflow](https://github.com/filecoin-project/lotus/blob/master/.github/workflows/release.yml#L220-L229)'s draft-release-body generation only parses `CHANGELOG.md` on the *first* CI run for this tag, before any GitHub release exists for it yet; every later run just reuses whatever draft body already exists instead of re-parsing the file (this is also why, per the note below, editorial changes made to `CHANGELOG.md` after that point must be copied into the draft release body by hand). That first-run parse splits the file on `^# ` boundaries and matches, working from the bottom of the file up, either the literal tag string `^# {{$.Tag}}{{$tagSuffix}} ` (note: this is the bare tag, e.g. `v1.36.3`, not `Node v1.36.3`) or `^# UNRELEASED`. In practice the dated header format used here never matches the tag pattern, so this always falls through to matching `^# UNRELEASED`. If that header is missing, renamed, or already emptied of content before a draft release exists for this tag, the generated body comes out empty. ([Caught by Copilot review on the v1.36.3 release PR](https://github.com/filecoin-project/lotus/pull/13782#discussion_r3983862233) after an agent renamed `UNRELEASED` away entirely.)
      - **If you ever need to delete an existing draft release** (e.g. because its content looks stale from an earlier attempt): do not assume the next CI run will regenerate it correctly. By that point `UNRELEASED` is usually already emptied, so the auto-generated body will come out empty or wrong. Recreate it yourself first with the real content, e.g. `gh release create v{{$.Tag}}{{$tagSuffix}} --draft --title v{{$.Tag}}{{$tagSuffix}} --notes-file <file-built-from-the-dated-CHANGELOG-section>`.
      - Note: after a draft release exists, rerunning the [release workflow](https://github.com/filecoin-project/lotus/blob/master/.github/workflows/release.yml#L220-L229) preserves the existing draft release body. If editorial review changes release-note content in CHANGELOG, update the draft GitHub release body too before merge; the [push-triggered publish step](https://github.com/filecoin-project/lotus/blob/master/.github/workflows/release.yml#L307-L308) publishes that draft body.
      - Commit structure: past releases folded this change into the same commit/PR as the version bump (e.g. `release/v1.36.1`'s [`d7491c12d`](https://github.com/filecoin-project/lotus/commit/d7491c12d3b67da4416192c079e1c1718fcf3db9), `release/v1.36.2`'s [`c6f4d0240`](https://github.com/filecoin-project/lotus/commit/c6f4d02400dba55ebc5ab3677ef2ae5a5f4d1aef)). A single dedicated commit titled `docs(release): finalize v{{$.Tag}}{{$tagSuffix}} release notes in CHANGELOG` also works and makes the Post-Release cherry-pick-back-to-master step cleaner by isolating exactly the CHANGELOG diff. Past releases used inconsistent titles for this step (`docs: prepare vX.Y.Z changelog`, `chore: prep changelog Lotus vX.Y.Z`, `chore: update changelog`) with no real convention; pick one of the two approaches above and stay consistent through Post-Release.
<!--  {{if contains "Node" $.Type}}-->
      - Node release body: `gh release view v{{$.Tag}}{{$tagSuffix}} --repo filecoin-project/lotus --json body -q .body`
<!--  {{end}}-->
<!--  {{if contains "Miner" $.Type}}-->
      - Miner release body: `gh release view miner/v{{$.Tag}}{{$tagSuffix}} --repo filecoin-project/lotus --json body -q .body`
<!--  {{end}}-->
   - [ ] Perform editorial review (e.g., callout breaking changes, new features, FIPs, actor bundles)
<!--  {{if ne $.NetworkUpgrade ""}}-->
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
<!--  {{if $stable}}-->
   - [ ] Review and update the draft GitHub release body so it matches the CHANGELOG.
<!--  {{end}}-->
   - [ ] Update the PR with the commit(s) made to the CHANGELOG
<!--  {{if $stable}}-->
- [ ] Confirm the release PR CI is green, including release asset generation.
- [ ] Confirm the release owner approves publishing this stable release.
- [ ] Confirm any security-advisory staging needed for this release has an owner and follows [policy](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#security-fix-policy).
<!--  {{end}}-->
- [ ] Mark the PR "ready for review" (non-draft)
- [ ] Merge the PR
   - Merging the PR will trigger a CI run that will build assets, attach the assets to the GitHub release, publish the GitHub release, and create the corresponding git tag.
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
