
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

This document aims to describe how the Lotus maintainers plan to ship releases of Lotus. Interested parties can expect new releases to come out as described in this document.

## High-level Summary

- Lotus uses semantic versioning (`MAJOR`.`MINOR`.`PATCH`).
- **`MAJOR` releases** (e.g., 2.0.0) are reserved for significant architectural changes to Lotus.
- **`MINOR` releases** (e.g., 1.29.0) are shipped for network upgrades, API breaking changes, or non-backwards-compatible feature enhancements.
- **`PATCH` releases** (e.g., 1.28.1) contain backwards-compatible bug fixes or feature enhancements.
- Releases are branched from the master branch, regardless of whether they include a network upgrade.
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

Bumps to the Lotus major version number (1.0.0, 2.0.0, etc.) are reserved for significant architectural changes to Lotus. These releases are expected to take considerable time to develop and will be rare.

### Minor Releases

Bumps to the Lotus minor version number (1.28.0, 1.29.0, etc.) are used for:

- Shipping Filecoin network upgrades
- API breaking changes
- Non-backwards-compatible feature enhancements

Users **must** upgrade to these releases that include network upgrades before a certain time to keep in sync with the Filecoin network, and we recommend everyone to subscribe to status.filecoin.io for updates when these are happening, as well checking the release notes of a minor version if 

### Patch Releases

Bumps to the Lotus patch version number (1.28.1, 1.28.2, etc.) are used for:

- Backwards-compatible bug fixes
- Backwards-compatible feature enhancements

These releases are not mandatory but are highly recommended, as they may contain critical security fixes.

## Release Cycle

We aim to ship a new release of the Lotus software approximately every 4 weeks. However, releases that include network upgrades may have longer development and testing periods.

## Release Process

1. Releases are branched from the master branch, regardless of whether they include a network upgrade or not.
2. All PRs should target the master branch, and if they need to be backported to a release candidate, they should be marked with a `backport` label.
3. The `releases` branch are no longer used.

## Release Candidates (RCs)

- For regular releases, the RC period is typically around 1 week.
- For releases accompanying network upgrades, the release candiadte period is a lot longer to allow for more extensive testing, usually around 5 to 6 weeks.

## Security Fix Policy

Any release may contain security fixes. Unless the fix addresses a bug being exploited in the wild, the fix will not be called out in the release notes. Please make sure to update ASAP.

By policy, the team will usually wait until about 3 weeks after the final release to announce any fixed security issues. However, depending on the impact and ease of discovery of the issue, the team may wait more or less time.

Unless a security issue is actively being exploited or a significant number of users are unable to update to the latest version, security fixes will not be backported to previous releases.

## Branch and Tag Strategy
* Releases branches have the name `release/vX.Y.Z`
* By the end of the release process, a `release/vX.Y.Z` branch will have an associated `vX.Y.Z` tag and likely also `vX.Y.Z-rcN` tags.
* `master` branch is almost always where `release/vX.Y.Z` branches are created from.  The exception is we need to do an emergency rushed patch release and can't risk pulling in other changes that have already landed in `master`.  In that case, `release/vX.Y.Z+1` will be created from `release/vX.Y.Z`.

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

1. Release Issue template
2. Lotus Release Flow Discussion: Leave a comment if you have any questions or feedback regarding the lotus release flow.