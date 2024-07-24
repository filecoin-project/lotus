> Release Issue Template

# Lotus X.Y.Z Release

[//]: # (Open this issue as [WIP] Lotus vX.Y.Z)
[//]: # (Apply the `tpm` label to it, and pin the issue on GitHub)

## 🚢 Estimated shipping date

<Date this release will ship on if everything goes to plan (week beginning...)>

## ✅ Release Checklist

**Note for whoever is owning the release:** please capture notes as comments in this issue for anything you noticed that could be improved for future releases.  There is a *Post Release* step below for incorporating changes back into the [RELEASE_ISSUE_TEMPLATE](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md), and this is easier done by collecting notes from along the way rather than just thinking about it at the end.

- [ ] Fork a new branch (`release/vX.Y.Z` or `release/miner/vX.Y.Z`) from `master` and make any further release related changes to this branch. If any "non-trivial" changes get added to the release, uncheck all the checkboxes and return to this stage.
- [ ] Bump the version in `build/version.go` in the `master` branch to `vX.Y.(Z+1)-dev` (bump from feature release) or `vX.(Y+1).0-dev` (bump from mandatory release). Run make gen and make docsgen-cli before committing changes

**Prepping an RC**:

Perform the following changes to the `release/vX.Y.Z` or `release/miner/vX.Y.Z` branch through a PR:
- [ ] update the version string in `build/version.go` to one ending with '-rcX'
- [ ] run `make gen && make docsgen-cli` to generate documentation
- [ ] create a **PR** targetting `release/vX.Y.Z` or `release/miner/vX.Y.Z` branch
  - Opening a PR will trigger a CI run that will build the release and publish it to GitHub as a draft
  - Merging the PR will trigger a CI run that will publish the GitHub release (it will also create a git tag)

**Testing**

Test the release candidate thoroughly, including automated and manual tests to ensure stability and functionality across various environments and scenarios.

**Stable Release**

Perform the following changes to the `release/vX.Y.Z` branch (optionally, through a PR):
- [ ] update the version string in `build/version.go` to one **NOT** ending with '-rcX'
- [ ] run `make gen && make docsgen-cli` to generate documentation
- [ ] either commit the changes directly or open a PR against the `release/vX.Y.Z` or `release/miner/vX.Y.Z` branch

Perform the following changes to the `releases` branch through a PR:
- [ ] create a **PR** targetting `releases` branch (base) from `release/vX.Y.Z` or `release/miner/vX.Y.Z` branch (head)
  - Opening a PR will trigger a CI run that will build the release and publish it to GitHub as a draft
  - Merging the PR will trigger a CI run that will publish the GitHub release (it will also create a git tag)

**Post-Release**

- [ ] Open a pull request against the `master` branch with a merge of the `releases` branch. Conflict resolution should ignore the changes to `version.go` (keep the `-dev` version from master). Do NOT delete the `releases` branch when doing so!
- [ ] Update [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) with any improvements determined from this latest release iteration.
- [ ] Create an issue using [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) for the _next_ release.

## ❤️ Contributors

See the final release notes!

## ⁉️ Do you have questions?

Leave a comment in this ticket!
