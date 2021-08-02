> Release Issue Template

# Lotus X.Y.Z Release

We're happy to announce Lotus X.Y.Z...

## 🗺 Must-dos for the release

## 🌟 Nice-to-haves for the release

<List of items with PRs and/or Issues to be considered for this release>

## 🚢 Estimated shipping date

<Date this release will ship on if everything goes to plan (week beginning...)>

## 🔦 Highlights

< top highlights for this release notes >

## ✅ Release Checklist

**Note for whomever is owning the release:** please capture notes as comments in this issue for anything you noticed that could be improved for future releases.  There is a *Post Release* step below for incorporating changes back into the [RELEASE_ISSUE_TEMPLATE](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md), and this is easier done by collecting notes from along the way rather than just thinking about it at the end.

First steps:

  - [ ] Fork a new branch (`release/vX.Y.Z`) from `master` and make any further release related changes to this branch. If any "non-trivial" changes get added to the release, uncheck all the checkboxes and return to this stage.
  - [ ] Bump the version in `version.go` in the `master` branch to `vX.(Y+1).0-dev`.
    
Prepping an RC:

- [ ] version string in `build/version.go` has been updated (in the `release/vX.Y.Z` branch).
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
  - [ ] Inform beta lotus users (@lotus-early-testers in Filecoin Slack #fil-lotus)


- [ ] **Stage 3 - Community Prod Testing**
  - [ ] Documentation
    - [ ] Ensure that [CHANGELOG.md](https://github.com/filecoin-project/lotus/blob/master/CHANGELOG.md) is up to date
    - [ ] Check if any [config](https://docs.filecoin.io/get-started/lotus/configuration-and-advanced-usage/#configuration) updates are needed
  - [ ] Invite the wider community through (link to the release issue):
    - [ ] Check `Create a discussion for this release` when tagging for the major/close-to-final rcs(new features, hot-fixes) release 
    - [ ] Link the disucssion in #fil-lotus on Filecoin slack
    
- [ ] **Stage 4 - Release**
  - [ ] Final preparation
    - [ ] Verify that version string in [`version.go`](https://github.com/ipfs/go-ipfs/tree/master/version.go) has been updated.
    - [ ] Ensure that [CHANGELOG.md](https://github.com/filecoin-project/lotus/blob/master/CHANGELOG.md) is up to date
    - [ ] Prep the changelog using `scripts/mkreleaselog`, and add it to `CHANGELOG.md`
    - [ ] Merge `release-vX.Y.Z` into the `releases` branch.
    - [ ] Tag this merge commit (on the `releases` branch) with `vX.Y.Z`
    - [ ] Cut the release [here](https://github.com/filecoin-project/lotus/releases/new?prerelease=true&target=releases).
    - [ ] Final announcements
        - [ ] Update network.filecoin.io for mainnet, calib and nerpa.
        - [ ] repost in #fil-lotus-announcement in filecoin slack
        - [ ] Inform node providers (Protofire, Digital Ocean..)

- [ ] **Post-Release**
  - [ ] Merge the `releases` branch back into `master`, ignoring the changes to `version.go` (keep the `-dev` version from master). Do NOT delete the `releases` branch when doing so!
  - [ ] Update [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) with any improvements determined from this latest release iteration.
  - [ ] Create an issue using [RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) for the _next_ release.

## ❤️ Contributors

See the final release notes!

## ⁉️ Do you have questions?

Leave a comment [here](<link to release discussion>) if you have any questions.
