> Release Issue Template

# Lotus X.Y.Z Release

[//]: # (Open this issue as [WIP] Lotus vX.Y.Z)
[//]: # (Apply the `tpm` label to it, and pin the issue on GitHub)

## 🚢 Estimated shipping date

<Date this release will ship on if everything goes to plan (week beginning...)>

## ✅ Release Checklist

**Note for whoever is owning the release:** please capture notes as comments in this issue for anything you noticed that could be improved for future releases.  There is a *Post Release* step below for incorporating changes back into the [RELEASE_ISSUE_TEMPLATE](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md), and this is easier done by collecting notes from along the way rather than just thinking about it at the end.

- [ ] Fork a new branch (`release/vX.Y.Z`) from `master` and make any further release related changes to this branch. If any "non-trivial" changes get added to the release, uncheck all the checkboxes and return to this stage.
- [ ] Bump the version in `build/version.go` in the `master` branch to `vX.Y.(Z+1)-dev` (bump from feature release) or `vX.(Y+1).0-dev` (bump from mandatory release). Run make gen and make docsgen-cli before committing changes
- [ ] *Optional:* If preparing a release for a network upgrade, make sure all [Lotus dependecies are updated to the correct versions for the network upgrade](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/Update_Dependencies_Lotus.md)

**Prepping an RC**:

- [ ] version string in `build/version.go` needs to be updated to end with '-rcX' (in the `release/vX.Y.Z` branch)
- [ ] run `make gen && make docsgen-cli`
- [ ] Generate changelog using the script at scripts/mkreleaselog
- [ ] Add contents of generated text to lotus/CHANGELOG.md in addition to other details
- [ ] Commit using PR
- [ ] tag commit with `vX.Y.Z-rcN`
- [ ] cut a pre-release [here](https://github.com/filecoin-project/lotus/releases/new?prerelease=true)

**Testing**

Test the release candidate thoroughly, including automated and manual tests to ensure stability and functionality across various environments and scenarios.

**Stable Release**
  - [ ] Final preparation
    - [ ] Verify that version string in [`version.go`](https://github.com/filecoin-project/lotus/blob/master/build/version.go) has been updated.
    - [ ] Verify that codegen is up to date (`make gen && make docsgen-cli`)
    - [ ] Ensure that [CHANGELOG.md](https://github.com/filecoin-project/lotus/blob/master/CHANGELOG.md) is up to date
    - [ ] Open a pull request against the `releases` branch with a merge of `release-vX.Y.Z`.
    - [ ] Cut the release [here](https://github.com/filecoin-project/lotus/releases/new?prerelease=false&target=releases).
      - The target should be the `releases` branch.
      - Either make the tag locally and push to the `releases` branch, or allow GitHub to create a new tag via the UI when the release is published.

**Post-Release**
  - [ ] Open a pull request against the `master` branch with a merge of the `releases` branch. Conflict resolution should ignore the changes to `version.go` (keep the `-dev` version from master). Do NOT delete the `releases` branch when doing so!
  - [ ] Update [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) with any improvements determined from this latest release iteration.
  - [ ] Create an issue using [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) for the _next_ release.

## ❤️ Contributors

See the final release notes!

## ⁉️ Do you have questions?

Leave a comment in this ticket!
