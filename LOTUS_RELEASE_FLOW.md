
<!-- TOC -->

- [`lotus` Release Flow](#lotus-release-flow)
  - [Purpose](#purpose)
  - [High-level Summary](#high-level-summary)
  - [Motivation and Requirements](#motivation-and-requirements)
  - [Adopted Conventions](#adopted-conventions)
    - [Major Releases](#major-releases)
    - [Mandatory Releases](#mandatory-releases)
    - [Feature Releases](#feature-releases)
    - [Examples Scenarios](#examples-scenarios)
  - [Release Cycle](#release-cycle)
    - [Patch Releases](#patch-releases)
    - [Performing a Release](#performing-a-release)
    - [Security Fix Policy](#security-fix-policy)
  - [FAQ](#faq)
    - [Why aren't Go major versions used more?](#why-arent-go-major-versions-used-more)
  - [Related Items](#related-items)

<!-- /TOC -->

# `lotus` Release Flow

## Purpose

This document aims to describe how the Lotus maintainers ship releases of Lotus. Interested parties can expect new releases to be delivered as described in this document.

## High-level Summary

- Lotus uses semantic versioning (`MAJOR`.`MINOR`.`PATCH`).
- **`MAJOR` releases** are reserved for significant architectural changes to Lotus. 
- **`MINOR` releases** are shipped for network upgrades, API breaking changes, or non-backwards-compatible feature enhancements.
- **`PATCH` releases** contain backwards-compatible bug fixes or feature enhancements.
- Releases are almost always branched from the `master` branch, even if they include a network upgrade.  The main exception is if there is a critical security patch we need to rush out.  In that case, we would patch an existing release to increase release speed and reduce barrier to adoption.
- We aim to ship a new release of the Lotus software approximately every 4 weeks, except during network upgrade periods which may have longer release cycles.

## Motivation and Requirements

Our primary motivation is for users of the Lotus software (node operators, RPC providers, storage providers, developers, etc.) to have a clear idea about when they can expect Lotus releases, and what they can expect in a release.

In order to achieve this, we need the following from our release process and conventions:

- Lotus version conventions make it clear what kind of changes are included in a release.
- The ability to ship critical fixes quickly when needed.
- A regular cadence of releases, so that users can know when a new Lotus release will be available.
- A clear description of the various stages of testing that a Lotus Release Candidate (RC) goes through.
- Lotus Release issues will present a single source of truth for what may be contained in Lotus releases, including security fixes, and how they will be disclosed.

## Adopted Conventions

### Major Releases

Bumps to the Lotus major version number (e.g., 2.0.0, 3.0.0) are reserved for significant architectural changes to Lotus. These releases are expected to take considerable time to develop and will be rare.  At least of 202408, there is nothing on the horizon that we're aware of that warrants this.   See also [What aren't go major versions used more?](#why-arent-go-major-versions-used-more)

### Minor Releases

Bumps to the Lotus minor version number (e.g., 1.28.0, 1.29.0) are used for:

- Shipping Filecoin network upgrades
- API breaking changes
- Non-backwards-compatible feature enhancements

Users MUST upgrade to minor releases that include a network upgrade before a certain time to keep in sync with the Filecoin network.  We recommend everyone to subscribe to status.filecoin.io for updates when these are happening, as well checking the release notes of a minor version. 
Users can decide whether to upgrade to minor version releases that don't include a network upgrade.  They are still encouraged to upgrade so they get the latest functionality and improvements and deploy a smaller delta of new code when there is a subsequent minor release they must adopt as part of a network upgrade later.  

### Patch Releases

Bumps to the Lotus patch version number (e.g., 1.28.1, 1.28.2) are used for:

- Backwards-compatible bug fixes
- Backwards-compatible feature enhancements

These releases are not mandatory but are highly recommended, as they may contain critical security fixes.

## Release Cycle

We aim to ship a new release of the Lotus software approximately every 4 weeks. However, releases that include network upgrades usually have longer development and testing periods.

## Release Process

1. Releases are branched from the master branch, regardless of whether they include a network upgrade or not.
2. All PRs should target the master branch, and if they need to be backported to a release candidate, they should be marked with a `backport` label.
3. As of 202408, the `releases` branch is no longer used, and no longer tracks the latest release.  One can still programmatically get the latest release with `git tag -l 'v*' | sort -V -r | head -n 1` 

## Release Candidates (RCs)

- For regular (i.e., no critical security patch) releases with no accompanying network upgrade, the RC period is typically around 1 week.
- For releases accompanying network upgrades, the release candiadte period is a lot longer to allow for more extensive testing, usually around 5 to 6 weeks.
- Releases rushing out a critical security patch will likely have an RC period on the order of hours or days, or may even forgo the RC phase.  To compensate for the release speed, these releases will include the minimum delta necessary, meaning they'll be a patch on top of an existing release rather than taking the latest changes in the `master` branch.

## Security Fix Policy

Any release may contain security fixes. Unless the fix addresses a bug being exploited in the wild, the fix will not be called out in the release notes to avoid shining a spotlight on the problem and increasing the chances of it being exploited.  As a result, this is one of the reasons we encourage users to upgrade in all cases, even when the release isn't associated with a network upgrade.

By policy, the team will usually wait until about 3 weeks after the final release to announce any fixed security issues. However, depending on the impact and ease of discovery of the issue, the team may wait more or less time.

Unless a security issue is actively being exploited or a significant number of users are unable to update to the latest version, security fixes will not be backported to previous releases.

## Branch and Tag Strategy

> [!NOTE]
> - <span style="color:blue">Blue text</span> indicates node-related information.
> - <span style="color:orange">Orange text</span> indicates miner-related information.
> - System default colored text applies to both node and miner releases.

* <span style="color:blue">Node release branches are named `release/vX.Y.Z`</span>
* <span style="color:orange">Miner release branches are named `release/miner/vX.Y.Z`</span>
* By the end of the release process:
  * <span style="color:blue">A `release/vX.Y.Z` branch (node) will have an associated `vX.Y.Z` tag</span>
  * <span style="color:orange">A `release/miner/vX.Y.Z` branch (miner) will have an associated `miner/vX.Y.Z` tag</span>
* Both node and miner releases may have additional `vX.Y.Z-rcN` or `miner/vX.Y.Z-rcN` tags for release candidates
* The `master` branch is typically the source for creating release branches
* For emergency patch releases where we can't risk including recent `master` changes:
  * <span style="color:blue">Node: `release/vX.Y.Z+1` will be created from `release/vX.Y.Z`</span>
  * <span style="color:orange">Miner: `release/miner/vX.Y.Z+1` will be created from `release/miner/vX.Y.Z`</span>
  
## FAQ

### Why aren't Go major versions used more?

Golang tightly couples source code with versioning (major versions beyond v1 leak into import paths). This poses logistical difficulties to using major versions here. Using major versions for every network upgrade would disrupt every downstream library/application that consumes the native Lotus API, even if it brought zero expectation of breakage for the Golang APIs they depend on.

### Do more frequent Lotus releases mean a change to network upgrade schedules?

No.  The starting-in-2024Q3 goal of more frequent (every 4 weeks) Lotus releases does not mean that there will be changes in the network upgrade schedule.  At least as of 202408, the current cadence of Filecoin network upgrades is ~3 per year.  We expect to usually uphold a 2 weeks upgrade time between a Lotus release candidate and a network upgrade on the Calibration network, and a 3 week upgrade time for a network upgrade on the Mainnet.

### How often exchanges and key stakeholders need to upgrade?

It´s hard to say how often they have to upgrade! If they do not encounter any issues with the current release they are on, and there are no new releases with vulnerability patches or an associated network upgrade, then upgrading is unnecessary. The goal for faster releases (and also having client and miner releases seperated) is to be able to bring bug-fixes and features faster to end-users that need them.  Per discussion above, users are still encouraged to consider upgrading more frequently than the ~3 network upgrades per year to reap the benefits of improved software and to have a smaller batch of changes to vet before a network upgrade.

### How much new code will a release with an associated network upgrade include?

Releases for a network upgrade will have "last production release + minimum commits necessary for network upgrade + any other commits that have made it into master since the last production release".  This means a release accompanying a network upgrade may have commits that aren't essential and haven't been deployed to production previously. This is a simplifier for Lotus maintainer, and we think the risk is acceptable because we'll be doing releases more frequently (thus the amount of commits that haven't made it to a production release will be smaller) and our testing quality has improved since years past.

## Related Items

1. [Release Issue template](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md)
2. [Network Upgrade process and tracking documents for nv23 onwards](https://drive.google.com/drive/folders/1LEfIQKsp0un3RxHdSbkBG7ON3H7HRuA7)
3. [Lotus release issues](https://github.com/filecoin-project/lotus/issues?q=is%3Aissue+release+in%3Atitle)
4. [202405 discussion](https://github.com/filecoin-project/lotus/discussions/12020) that triggered updating this document.
5. [Original 202108 Lotus release flow discussion](https://github.com/filecoin-project/lotus/discussions/7053).