
# `lotus` Release Flow <!-- omit in toc -->

- [Purpose](#purpose)
- [Terminology](#terminology)
- [High-level Summary](#high-level-summary)
- [Motivation and Requirements](#motivation-and-requirements)
- [Adopted Conventions](#adopted-conventions)
  - [Major Releases](#major-releases)
  - [Minor Releases](#minor-releases)
  - [Patch Releases](#patch-releases)
- [Release Cadence](#release-cadence)
- [Release Process](#release-process)
- [Release Candidates (RCs)](#release-candidates-rcs)
- [Security Fix Policy](#security-fix-policy)
- [Branch and Tag Strategy](#branch-and-tag-strategy)
- [FAQ](#faq)
  - [Why aren't Go major versions used more?](#why-arent-go-major-versions-used-more)
  - [Do more frequent Lotus releases mean a change to network upgrade schedules?](#do-more-frequent-lotus-releases-mean-a-change-to-network-upgrade-schedules)
  - [How often do exchanges and key stakeholders need to upgrade?](#how-often-do-exchanges-and-key-stakeholders-need-to-upgrade)
  - [How much new code will a release with an associated network upgrade include?](#how-much-new-code-will-a-release-with-an-associated-network-upgrade-include)
  - [Why do we call it "Lotus Node"?](#why-do-we-call-it-lotus-node)
  - [Why isn't Lotus Miner released more frequently?](#why-isnt-lotus-miner-released-more-frequently)
  - [Why is the `releases` branch deprecated and what are alternatives?](#why-is-the-releases-branch-deprecated-and-what-are-alternatives)
  - [Why does Lotus still use a `master` branch instead of `main`?](#why-does-lotus-still-use-a-master-branch-instead-of-main)
- [Related Items](#related-items)

## Purpose

This document aims to describe how the Lotus maintainers ship releases of Lotus. Interested parties can expect new releases to be delivered as described in this document.

## Terminology
* Lotus Node - This is all the functionality that lives in the `lotus` binary. Branches, tags, etc. that don't have a prefix correspond with Lotus Node. (See [Why do we call it "Lotus Node"?](#why-do-we-call-it-lotus-node))
* Lotus Miner - This is all the functionality that lives in the `lotus-miner` binary. Corresponding branches, tags, etc. are prefixed with `miner/`.
* Lotus software - This refers to the full collection of Lotus software that lives in [filecoin-project/lotus](https://github.com/filecoin-project/curio), both Lotus Node and Lotus Miner.
* Lotus (no suffix) - This usually means "Lotus Node" (which is why `lotus` maps to "Lotus Node"). That said, we strive to avoid this unqualified term because of the potential ambiguity. 

## High-level Summary

- Lotus software use semantic versioning (`MAJOR`.`MINOR`.`PATCH`).
- **`MAJOR` releases** are reserved for significant architectural changes to Lotus. 
- **`MINOR` releases** are shipped for [network upgrades](./documentation/misc/Building_a_network_skeleton.md#context), API breaking changes, or non-backwards-compatible feature enhancements.
- **`PATCH` releases** contain backwards-compatible bug fixes or feature enhancements.
- Releases are almost always branched from the `master` branch, even if they include a network upgrade. The main exception is if there is a critical security patch we need to rush out. In that case, we would patch an existing release to increase release speed and reduce barrier to adoption.
- We aim to ship a new release of the Lotus Node software approximately every 4 weeks, except during network upgrade periods which may have longer release cycles.
- Lotus Miner releases ship on an as-needed basis with no specified cadence. (See [Why isn't Lotus Miner released more frequently?](#why-isnt-lotus-miner-released-more-frequently))


## Motivation and Requirements

Our primary motivation is for users of the Lotus software (node operators, RPC providers, storage providers, developers, etc.) to have a clear idea about when they can expect Lotus releases, and what they can expect in a release.

In order to achieve this, we need the following from our release process and conventions:

- Lotus version conventions make it clear what kind of changes are included in a release.
- The ability to ship critical fixes quickly when needed.
- A regular cadence of releases, so that users can know when a new Lotus Node release will be available.
- A clear description of the various stages of testing that a Lotus Release Candidate (RC) goes through.
- Lotus Release issues will present a single source of truth for what may be contained in Lotus software releases, including security fixes, and how they will be disclosed.

## Adopted Conventions

### Major Releases

Bumps to the Lotus software major version number (e.g., 2.0.0, 3.0.0) are reserved for significant architectural changes to Lotus. These releases are expected to take considerable time to develop and will be rare. At least of 202408, there is nothing on the horizon that we're aware of that warrants this. See also [What aren't go major versions used more?](#why-arent-go-major-versions-used-more)

### Minor Releases

Bumps to the Lotus software minor version number (e.g., 1.28.0, 1.29.0) are used for:

- Shipping Filecoin network upgrades
- API breaking changes
- Non-backwards-compatible feature enhancements

Users MUST upgrade to minor releases that include a network upgrade before a certain time to keep in sync with the Filecoin network. We recommend everyone subscribe to status.filecoin.io for updates when these are happening, as well as checking the release notes of a minor version. ([Learn more about how network upgrades relate to Lotus and its key dependencies.](./documentation/misc/Building_a_network_skeleton.md#context))

Users can decide whether to upgrade to minor version releases that don't include a network upgrade. They are still encouraged to upgrade so they get the latest functionality and improvements and deploy a smaller delta of new code when there is a subsequent minor release they must adopt as part of a network upgrade later. 

### Patch Releases

Bumps to the Lotus software patch version number (e.g., 1.28.1, 1.28.2) are used for:

- Backwards-compatible bug fixes
- Backwards-compatible feature enhancements

These releases are not mandatory but are highly recommended, as they may contain critical security fixes.

## Release Cadence

* Lotus Node: we aim to ship a new release every 4 weeks. However, releases that include network upgrades usually have longer development and testing periods.
* Lotus Miner: releases ship on an as-needed basis with no specified cadence. (See [Why isn't Lotus Miner released more frequently?](#why-isnt-lotus-miner-released-more-frequently))

## Release Process
The specific steps executed for Lotus software releases are captured in the [Release Issue template](./documentation/misc/RELEASE_ISSUE_TEMPLATE.md).  `[cmd/release](./cmd/release/README.md) is used to help populate the template.

## Release Candidates (RCs)

- For regular (i.e., no critical security patch) releases with no accompanying network upgrade, the RC period is typically around 1 week.
- For releases accompanying network upgrades, the release candidate period is a lot longer to allow for more extensive testing, usually around 5 to 6 weeks.
- Releases rushing out a critical security patch will likely have an RC period on the order of hours or days, or may even forgo the RC phase. To compensate for the release speed, these releases will include the minimum delta necessary, meaning they'll be a patch on top of an existing release rather than taking the latest changes in the `master` branch.

## Security Fix Policy

Any release may contain security fixes. Unless the fix addresses a bug being exploited in the wild, the fix will not be called out in the release notes to avoid shining a spotlight on the problem and increasing the chances of it being exploited. As a result, this is one of the reasons we encourage users to upgrade in all cases, even when the release isn't associated with a network upgrade.

By policy, the team will usually wait until about 3 weeks after the final release to announce any fixed security issues. However, depending on the impact and ease of discovery of the issue, the team may wait more or less time.

Unless a security issue is actively being exploited or a significant number of users are unable to update to the latest version, security fixes will not be backported to previous releases.

## Branch and Tag Strategy

* Releases are usually branched from the `master` branch, regardless of whether they include a network upgrade or not.
  * For certain patch releases where we can't risk including recent `master` changes (such as for security or emergency bug-fix releases):
    * Node: `release/vX.Y.Z+1` will be created from `release/vX.Y.Z`
    * Miner: `release/miner/vX.Y.Z+1` will be created from `release/miner/vX.Y.Z`
* PRs usually target the `master` branch, even if they need to be backported to a release branch. 
  * The primary exception is CHANGELOG editorializing and callouts.  As part of the [release process](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md), those changes happen directly in a release branch and are cherry-picked back to `master` at the end of a release. 
* PRs that need to be backported should be marked with a `backport` label.
* Node release branches are named `release/vX.Y.Z`
* Miner release branches are named `release/miner/vX.Y.Z`
* By the end of the release process:
  * A `release/vX.Y.Z` branch (node) will have an associated `vX.Y.Z` tag
  * A `release/miner/vX.Y.Z` branch (miner) will have an associated `miner/vX.Y.Z` tag
* Both node and miner releases may have additional `vX.Y.Z-rcN` or `miner/vX.Y.Z-rcN` tags for release candidates.
* As of 202408, the `releases` branch is no longer used and no longer tracks the latest release.  See [Why is the `releases` branch deprecated and what are alternatives?](#why-is-the-releases-branch-deprecated-and-what-are-alternatives).

## FAQ

### Why aren't Go major versions used more?

Golang tightly couples source code with versioning (major versions beyond v1 leak into import paths). This poses logistical difficulties to using major versions here. Using major versions for every network upgrade would disrupt every downstream library/application that consumes the native Lotus API, even if it brought zero expectation of breakage for the Golang APIs they depend on.

### Do more frequent Lotus releases mean a change to network upgrade schedules?

No. The starting-in-2024Q3 goal of more frequent (every 4 weeks) Lotus releases does not mean that there will be changes in the network upgrade schedule. At least as of 202408, the current cadence of Filecoin network upgrades is ~3 per year. We expect to usually uphold a 2 weeks upgrade time between a Lotus release candidate and a network upgrade on the Calibration network, and a 3 week upgrade time for a network upgrade on the Mainnet.

### How often do exchanges and key stakeholders need to upgrade?

It´s hard to say how often they have to upgrade! If they do not encounter any issues with the current release they are on, and there are no new releases with vulnerability patches or an associated network upgrade, then upgrading is unnecessary. The goal for faster releases (and also having client and miner releases separated) is to be able to bring bug-fixes and features faster to end-users that need them. Per discussion above, users are still encouraged to consider upgrading more frequently than the ~3 network upgrades per year to reap the benefits of improved software and to have a smaller batch of changes to vet before a network upgrade.

### How much new code will a release with an associated network upgrade include?

Releases for a network upgrade will have "last production release + minimum commits necessary for network upgrade + any other commits that have made it into master since the last production release". This means a release accompanying a network upgrade may have commits that aren't essential and haven't been deployed to production previously. This is a simplifier for Lotus maintainer, and we think the risk is acceptable because we'll be doing releases more frequently (thus the amount of commits that haven't made it to a production release will be smaller) and our testing quality has improved since years past.

### Why do we call it "Lotus Node"?
There are other names that could have used instead of "Lotus Node", and some of those have existed historically (e.g., "Lotus Client", "Lotus Daemon"). We couldn't find a perfect name. "Lotus Node" is more than just a "client", even though it doesn't participate in consensus. "Lotus Daemon" is confusing as `lotus-miner` is also a daemon. Even "Node" isn't ideal, because it tends to imply full participation in the network, including consensus. We figured the most important thing was to pick a name and then consistently apply it.

### Why isn't Lotus Miner released more frequently?
Given Lotus Miner is being actively replaced by [Curio](https://github.com/filecoin-project/curio), Lotus Miner is not under active development. As a result, new releases happen reactively either to support new consensus-critical network functionality or patch critical security or performance issues.

### Why is the `releases` branch deprecated and what are alternatives?
`releases` goal was to point to the latest stable tagged release of Lotus software for convenience and script.  This worked when Lotus Node and Miner were released together, but with the [2024Q3 split of releasing Lotus Node and Miner separately](https://github.com/filecoin-project/lotus/issues/12010), there isn't necessarily a single commit to track for the latest released software of both. Rather than having ambiguity by tracking Lotus Node or Lotus Miner releases, we [decided it was clearer to deprecate the branch](https://github.com/filecoin-project/lotus/issues/12374). 

That said, one can still programmatically get the latest release based on the [Branch and Tag Strategy](#branch-and-tag-strategy) with:
* Lotus Node: `git tag -l 'v*' | grep -v "-" | sort -V -r | head -n 1` 
* Lotus Miner: `git tag -l 'miner/v*' | grep -v "-" | sort -V -r | head -n 1` 

### Why does Lotus still use a `master` branch instead of `main`?
There was a [push in 202109](https://github.com/filecoin-project/lotus/issues/7356) on changing the default branch to `main` from `master` for good reason. 3 years later though, the migration was never completed and `master` has ossified 😔.  The effort's failure was acknowledged and commented on in the issue above in 202409.  

## Related Items

1. [Release Issue template](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md)
2. [Network Upgrade process and tracking documents for nv23 onwards](https://drive.google.com/drive/folders/1LEfIQKsp0un3RxHdSbkBG7ON3H7HRuA7)
3. [Lotus release issues](https://github.com/filecoin-project/lotus/issues?q=is%3Aissue+release+in%3Atitle)
4. [202405 discussion](https://github.com/filecoin-project/lotus/discussions/12020) that triggered updating this document.
5. [Original 202108 Lotus release flow discussion](https://github.com/filecoin-project/lotus/discussions/7053).