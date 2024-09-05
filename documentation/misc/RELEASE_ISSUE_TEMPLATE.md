> Release Issue Template

# Lotus Node|Miner X.Y.Z Release

[//]: # (Below are non-visible steps intended for the issue creator)
[//]: # (❗️ Complete the steps below as part of creating a release issue and mark them complete with an X or ✅ when done.)
[//]: # ([ ] Start an issue with title "[WIP] Lotus Node|Miner vX.Y.Z Release" and adjust the title for whether it's a Node or Miner release.)
[//]: # ([ ] Copy in the content of https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md)
[//]: # ([ ] Find/Replace "X.Y.Z+1" with the actual values. This is intentionally done before the find/replace X.Y.Z step because that will also match.)
[//]: # ([ ] Find/Replace "X.Y.Z" with the actual values.)
[//]: # ([ ] If this isn't a release tied to a network upgrade, remove all items with "\(network upgrade\)")
[//]: # ([ ] If this is a patch release, remove all items with "\(minor release\)")
[//]: # ([ ] If this is a minor release, remove all items with "\(patch release\)")
[//]: # ([ ] Copy/paste the "Release Checklist > RCX" section to "Release Checklist > Stable \(non-RC\) Release" and apply the "diff" called out there.)
[//]: # ([ ] Find/Replace "RCX" with "RC1".)
[//]: # ([ ] Adjust the "Meta" section values.)
[//]: # ([ ] Apply the `tpm` label to the issue)
[//]: # ([ ] Create the issue)
[//]: # ([ ] Pin the issue on GitHub)

## 😶‍🌫 Meta
* Scope: Node|Miner MINOR|PATCH
* Is this linked with a network upgrade, and thus mandatory? Yes|No
* Related network upgrade version: nvXX|n/a

## 🚢 Estimated shipping date

[//]: # (If/when we know an exact date, remove the "week of".)
[//]: # (If a date week is an estimate, annotate with "estimate".)

| Candidate | Date | Release URL |
|-----------|------|-------------|
| RC1 | Week of YYYY-MM-DD | |
| Stable (non-RC) | Week of YYYY-MM-DD (estimate) | |

## 🪢 Dependencies for releases
> [!NOTE]
> 1. This is the set of changes that need to make it in for a given RC.  This is effectively the set of changes to cherry-pick from master.  
> 2. They can be checked as done once they land in `master`.  
> 3. They are presented here for quick reference, but backporting is tracked in each `Release Checklist`.

[//]: # (Copy/paste this for each RC, and increment "X")
### RCX
- [ ] To Be Added

### Stable (non-RC)
- [ ] To Be Added

## ✅ Release Checklist

### Before RC1
- [ ] (network upgrade) Make sure all [Lotus dependencies are updated to the correct versions for the network upgrade](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/Update_Dependencies_Lotus.md)
   - Link to Lotus PR:
- [ ] Open PR against [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) with title `docs(release): vX.Y.Z release template improvements` for improving future releases.
   - Link to PR:
   - There likely aren't any changes at this point, but this can be opened with a small whitespace change so the PR is open and we can more easily hold the standard of making improvements incrementally since improvements are usually better done by collecting changes/notes along the way rather than just thinking about it at the end.
   - This will get merged in a `Post Release` step.
- [ ] (patch release) Fork a new branch (`release/vX.Y.Z` or `release/miner/vX.Y.Z`) from the last stable `release/vX.Y.Z` or `release/miner/vX.Y.Z` and make any further release-related changes to this branch.
- [ ] (minor release) Fork a new branch (`release/vX.Y.Z` or `release/miner/vX.Y.Z`) from `master` and make any further release-related changes to this branch.
- (minor release) `master` branch Version string updates
   - [ ] bump the version(s) in `build/version.go` to `vX.Y.(Z+1)-dev`. 
      - Ensure to update the appropriate version string based on whether you are creating a node release (`NodeBuildVersion`), a miner release (`MinerBuildVersion`), or both.
   - [ ] Run `make gen && make docsgen-cli` before committing changes.
   - [ ] Remove the contents in the `Unreleased Changelog` section.
   - [ ] Create a PR with title `build: update Lotus Node|Miner version to vX.Y.Z+1-dev in master`
     - Link to PR: 
   - [ ] Merge PR

### RCs

[//]: # (Copy/paste this "RCX" section for each additional RC, and increment "X")
#### RCX
> [!IMPORTANT]
> These PRs should be done in and target the `release/vX.Y.Z` or `release/miner/vX.Y.Z` branch.

**Backport PR**

[//]: # (For RC1 there likely isn't any backporting to do and thus no PR which reduces the steps.)
[//]: # (We do need all these steps for RC2 onwards though.)
[//]: # (If steps are removed for the RC1 checklist, they need to be preserved for future RCs/stable.)
[//]: # (For RC1 we still need to make sure the tracked items land though.)
- [ ] All explicitly tracked items from `Dependencies for releases` have landed
- [ ] Backported [everything with the "backport" label](https://github.com/filecoin-project/lotus/issues?q=label%3Arelease%2Fbackport+) 
- [ ] Removed the "backport" label from all backported PRs (no ["backport" issues](https://github.com/filecoin-project/lotus/issues?q=label%3Arelease%2Fbackport+))
- [ ] Create a PR with title `build: backport changes for node|miner vX.Y.Z-rcX`
   - Link to PR: 
- [ ] Merge PR 

**Release PR**

- [ ] Update the version string(s) in `build/version.go` to one ending with '-rcX'. 
    - Ensure to update the appropriate version string based on whether you are creating a node release (`NodeBuildVersion`), a miner release (`MinerBuildVersion`), or both.
- [ ] Run `make gen && make docsgen-cli` to generate documentation
- [ ] Create a PR with title `build: release Lotus node|miner vX.Y.Z-rcX`
   - Link to PR: 
   - Opening a PR will trigger a CI run that will build assets, create a draft GitHub release, and attach the assets.
- [ ] Changelog prep
   - [ ] Go to the [releases page](https://github.com/filecoin-project/lotus/releases) and copy the auto generated changelog into your release PR and adjust the changelog accordingly.
   - [ ] Editorial review (e.g., callout breaking changes, new features, FIPs, actor bundles)
   - [ ] (network upgrade) Specify whether the Calibration or Mainnet upgrade epoch has been specified or not yet.
      - Example where these weren't specified yet: [PR #12169](https://github.com/filecoin-project/lotus/pull/12169)
   - [ ] Ensure no missing content when spot checking git history
      - Example command looking at git commits: `git log --oneline --graph vA.B.C..`, where A.B.C correspond to the previous release. 
      - Example GitHub UI search looking at merged PRs into master: https://github.com/filecoin-project/lotus/pulls?q=is%3Apr+base%3Amaster+merged%3A%3EYYYY-MM-DD
      - Example `gh` cli command looking at merged PRs into master and sorted by title to group similar areas (where `YYYY-MM-DD` is the start search date): `gh pr list --repo filecoin-project/lotus --search "base:master merged:>YYYY-MM-DD" --json number,mergedAt,author,title | jq -r '.[] | [.number, .mergedAt, .author.login, .title] | @tsv' | sort -k4`
- [ ] Merge the PR
   - Opening the PR will trigger a CI run that will build assets, attach the assets to the GitHub release, publish the GitHub release, and create the corresponding git tag.
 - [ ] Update `🚢 Estimated shipping date` table
 - [ ] Comment on this issue announcing the RC
    - Link to issue comment:   

### Testing
> [!NOTE]
> Link to any special steps for testing releases beyond ensuring CI is green.  Steps can be inlined here or tracked elsewhere.

### Stable (non-RC) Release

[//]: # (This "NOTE" below with the "diff" to apply to the "RC copy/pasted content" is here to avoid the duplication in the template itself.)
[//]: # (This is done as a visible NOTE rather than a comment to make sure it's clear what needs to be added to this section.)
[//]: # (These comments ^^^ can be removed once the NOTE steps below are completed.)
> [!NOTE]
> Copy/paste in the `RCX` section above and then make these changes:
> 1. Under "Release PR > Update the version string...", edit:
> 
> Update the version string in `build/version.go` to one **NOT** ending with '-rcX'
>
> 2. Under "Release PR > Changelog prep...", add:
> 
> (network upgrade) Ensure the Mainnet upgrade epoch is specified.
>
> 3. Under "Release PR > Update the version string...", edit:
>
> Create a PR with title `build: release vX.Y.Z`
>
> 4. Remove this `[!Note]` and the related invisible comments.

### Post-Release

- [ ] Open a pull request against `master` cherry-picking the CHANGELOG commits from the `release/vX.Y.Z` branch. 
   - Link to PR: 
   - Assuming we followed [the process of merging changes into `master` first before backporting to the release branch](https://github.com/filecoin-project/lotus/blob/master/LOTUS_RELEASE_FLOW.md#branch-and-tag-strategy), the only changes should be CHANGELOG updates.  
- [ ] Finish updating/merging the [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) PR from `Before RC1` with any improvements determined from this latest release iteration.

## ❤️ Contributors

See the final release notes!

## ⁉️ Do you have questions?

Leave a comment in this ticket!
