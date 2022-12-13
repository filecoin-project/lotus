> Release Issue Template

# Lotus X.Y.Z Release


## What will be in the release


## 🚢 Estimated shipping date

<Date this release will ship on if everything goes to plan (week beginning...)>

## 🔦 Highlights

< See Changelog>

## ✅ Release Checklist

**Note for whomever is owning the release:** please capture notes as comments in this issue for anything you noticed that could be improved for future releases.  There is a *Post Release* step below for incorporating changes back into the [RELEASE_ISSUE_TEMPLATE](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md), and this is easier done by collecting notes from along the way rather than just thinking about it at the end.

First steps:

  - [ ] Fork a new branch (`release/vX.Y.Z`) from `master` and make any further release related changes to this branch. If any "non-trivial" changes get added to the release, uncheck all the checkboxes and return to this stage.
  - [ ] Bump the version in `build/version.go` in the `master` branch to `vX.Y.(Z+1)-dev` (bump from feature release) or `vX.(Y+1).0-dev` (bump from mandatory release). Run make gen and make docsgen-cli before committing changes
    
Prepping an RC:

- [ ] version string in `build/version.go` has been updated (in the `release/vX.Y.Z` branch)
- [ ] run `make gen && make docsgen-cli`
- [ ] Generate changelog using the script at scripts/mkreleaselog
- [ ] Add contents of generated text to lotus/CHANGELOG.md in addition to other details
- [ ] tag commit with `vX.Y.Z-rcN`
- [ ] cut a pre-release [here](https://github.com/filecoin-project/lotus/releases/new?prerelease=true)

Testing an RC:

- [ ] **Stage 0 - Automated Testing**
  - Automated Testing
    - [ ] CI: Ensure that all tests are passing.
    - [ ] Testground tests

- [ ] **Stage 1 - Internal Testing**
  - Binaries
    - [ ] Ensure the RC release has downloadable binaries
  - Upgrade our testnet infra
    - [ ] Wait 24 hours, confirm nodes stay in sync
  -  Upgrade our mainnet infra
    - [ ] Subset of development full archival nodes
    - [ ] Subset of bootstrappers (1 per region)
    - [ ] Confirm nodes stay in sync
    - Metrics report
        - Block validation time
        - Memory / CPU usage
        - Number of goroutines
        - IPLD block read latency
        - Bandwidth usage
    - [ ] If anything has worsened significantly, investigate + fix
  - Confirm the following work (some combination of Testground / Calibnet / Mainnet / beta users)
    - [ ] Seal a sector
    - [ ] make a deal
    - [ ] Submit a PoSt
    - [ ] (optional) let a sector go faulty, and see it be recovered
    
- [ ] **Stage 2 - Community Testing**
  - [ ] Test with [SPX](https://github.com/filecoin-project/lotus/discussions/7461) fellows
  - [ ] Work on documentations for new features, configuration changes and so on.

- [ ] **Stage 3 - Community Prod Testing**
  - [ ] Update the [CHANGELOG.md](https://github.com/filecoin-project/lotus/blob/master/CHANGELOG.md) to the state that can be used as release note.
  - [ ] Invite the wider community through (link to the release issue)
    
- [ ] **Stage 4 - Stable Release**
  - [ ] Final preparation
    - [ ] Verify that version string in [`version.go`](https://github.com/filecoin-project/lotus/blob/master/build/version.go) has been updated.
    - [ ] Verify that codegen is up to date (`make gen && make docsgen-cli`)
    - [ ] Ensure that [CHANGELOG.md](https://github.com/filecoin-project/lotus/blob/master/CHANGELOG.md) is up to date
    - [ ] Merge `release-vX.Y.Z` into the `releases` branch.
    - [ ] Tag this merge commit (on the `releases` branch) with `vX.Y.Z`
    - [ ] Cut the release [here](https://github.com/filecoin-project/lotus/releases/new?prerelease=false&target=releases).


- [ ] **Post-Release**
  - [ ] Merge the `releases` branch back into `master`, ignoring the changes to `version.go` (keep the `-dev` version from master). Do NOT delete the `releases` branch when doing so!
  - [ ] Update [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) with any improvements determined from this latest release iteration.
  - [ ] Create an issue using [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) for the _next_ release.

## ❤️ Contributors

See the final release notes!

## ⁉️ Do you have questions?

Leave a comment in this ticket!
