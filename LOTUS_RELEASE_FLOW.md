
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

This document aims to describe how the Lotus team plans to ship releases of the Lotus implementation of Filecoin network. Interested parties can expect new releases to come out as described in this document.

## High-level Summary

- **Major releases** (1.0.0, 2.0.0, etc.) are reserved for significant changes to the Filecoin protocol that transform the network and its usecases. Such changes could include the addition of new Virtual Machines to the protocol, major upgrades to the Proofs of Replication used by Filecoin, etc.
- Even minor releases (1.2.0, 1.4.0, etc.) of the Lotus software correspond to **mandatory releases** as they ship Filecoin network upgrades. Users **must** upgrade to these releases before a certain time in order to keep in sync with the Filecoin network. We aim to ensure there is at least one week before the upgrade deadline.
- Patch versions of even minor releases (1.2.1, 1.4.2, etc.) correspond to **hotfix releases**. Such releases will only be shipped when a critical fix is needed to be applied on top of a mandatory release.
- Odd minor releases (1.3.0, 1.5.0, etc.), as well as patch releases in these series (1.3.1, 1.5.2, etc.) correspond to **feature releases** with new development and bugfixes. These releases are not mandatory, but still highly recommended, **as they may contain critical security fixes**.
- We aim to ship a new feature release of the Lotus software every 3 weeks, so users can expect a regular cadence of Lotus feature releases. Note that mandatory releases for network upgrades may disrupt this schedule.

## Motivation and Requirements

Our primary motivation is for users of the Lotus software (storage providers, storage clients, developers, etc.) to have a clear idea about when they can expect Lotus releases, and what they can expect in a release.

In order to achieve this, we need the following from our release process and conventions:

- Lotus version conventions make it immediately obvious whether a new Lotus release is mandatory or not. A release is mandatory if it ships a network upgrade to the Filecoin protocol.
- The ability to ship critical fixes on top of mandatory releases, so as to avoid forcing users to consume larger unrelated changes.
- A regular cadence of feature releases, so that users can know when a new Lotus release will be available for consumption.
- The ability to ship any number of feature releases between two mandatory releases.
- A clear description of the various stages of testing that a Lotus Release Candidate (RC) goes through.
- Lotus Release issues will present a single source of truth for what may be contained in Lotus releases, including security fixes, and how they will be disclosed.

## Adopted Conventions

This section describes the conventions we have adopted. Users of Lotus are encouraged to review this in detail so as to be informed when new Lotus releases are shipped.

### Major Releases

Bumps to the Lotus major version number (1.0.0, 2.0.0, etc.) are reserved for significant changes to the Filecoin protocol that dramatically transform the network itself. Such changes could include the addition of new Virtual Machines to the protocol, major upgrades to the Proofs of Replication used by Filecoin, etc. These releases are expected to take lots of time to develop and will be rare.  See also "Why aren't Go major versions used more?" below.

### Mandatory Releases

Even bumps to the Lotus minor version number (1.2.0, 1.4.0, etc.) are reserved for **mandatory releases** that ship Filecoin network upgrades. Users **must** upgrade to these releases before a certain time in order to keep in sync with the Filecoin network.

Depending on the scope of the upgrade, these releases may take up to several weeks to fully develop and test. We aim to ensure there are at least 2 weeks between the publication of the final Lotus release and the Filecoin network upgrade deadline.

These releases do not follow a regular cadence, as they are developed in lockstep with the other implementations of the Filecoin protocol. As of August 2021, the developers aim to ship 3-4 Filecoin network upgrades a year, though smaller security-critical upgrades may occur unpredictably.

Mandatory releases are somewhat sensitive since all Lotus users are forced to upgrade to them at the same time. As a result, they will be shipped on top of the most recent stable release of Lotus, which will generally be the latest Lotus release that has been in production for more than 2 weeks. (Note: given this rule, the basis of a mandatory release could be a mandatory release or a feature release depending on timing).  Mandatory releases will not include any new feature development or bugfixes that haven't already baked in production for 2+ weeks, except for the changes needed for the network upgrade itself. Further, any critical fixes that are needed after the network upgrade will be shipped as patch version bumps to the mandatory release (1.2.1, 1.2.2, etc.) This prevents users from being forced to quickly digest unnecessary changes.

Users should generally aim to always upgrade to a new even minor version release since they either introduce a mandatory network upgrade or a critical fix.

### Feature Releases

All releases under an odd minor version number indicate **feature releases**. These could include releases such as 1.3.0, 1.3.1, 1.5.2, etc. 

Feature releases include new development and bug fixes. They are not mandatory, but still highly recommended, **as they may contain critical security fixes**. Note that some of these releases may be very small patch releases that include critical hotfixes.  There is no way to distinguish between a bug fix release and a feature release on the "feature" version. Both cases will use the "patch" version number.

We aim to ship a new feature release of the Lotus software from our development (master) branch every 3 weeks, so users can expect a regular cadence of Lotus feature releases. Note that mandatory releases for network upgrades may disrupt this schedule. For more, see the Release Cycle section (TODO: Link).

### Examples Scenarios

- **Scenario 1**: **Lotus 1.12.0 shipped a network upgrade, and no network upgrades are needed for a long while.**

    In this case, the next feature release will be Lotus 1.13.0. In three-week intervals, we will ship Lotus 1.13.1, 1.13.2, and 1.13.3, all containing new features and bug fixes.

    Let us assume that after the release of 1.13.3, a critical issue is discovered and a hotfix quickly developed. This hotfix will then be shipped in **both** 1.12.1 and 1.13.4. Users who have already upgrade to the 1.13 series can simply upgrade to 1.13.4. Users who have chosen to still be on 1.12.0, however, can use 1.12.1 to patch the critical issue without being forced to consume all the changes in the 1.13 series.

- **Scenario 2**: **Lotus 1.12.0 shipped a network upgrade, but the need for an unexpected network upgrade soon arises**

    In this case, the Lotus 1.13 series will be dropped entirely, including any RCs that may have been undergoing testing. Instead, the network upgrade will be shipped as Lotus 1.14.0, built on top of Lotus 1.12.0. It will thus include no unnecessary changes, only the work needed to support the new network upgrade.

    Any changes that were being worked on in the 1.13.0 series will then get applied on top of Lotus 1.14.0 and get shipped as Lotus 1.15.0.

## Release Cycle

A mandatory release process should take about 3-6 weeks, depending on the amount and the overall complexity of new features being introduced to the network protocol. It may also be shorter if there is a network incident that requires an emergency upgrade.  A feature release process should take about 2-3 weeks. 

The start time of the mandatory release process is subject to the network upgrade timeline. We will start a new feature release process every 3 weeks on Tuesdays, regardless of when the previous release landed unless it's still ongoing.

### Patch Releases

**Mandatory Release**

If we encounter a serious bug in a mandatory release post a network upgrade, we will create a patch release based on this release. Strictly only the fix to the bug will be included in the patch, and the bug fix will be backported to the master (dev) branch, and any ongoing feature release branch if applicable.

Patch release process for the mandatory releases will follow a compressed release cycle from hours to days depending on the severity and the impact to the network of the bug. In a patch release:

1. Automated and internal testing (stage 0 and 1) will be compressed into a few hours.
2. Stage 2-3 will be skipped or shortened case by case.

Some patch releases, especially ones fixing one or more complex bugs that doesn't require a follow-up mandatory upgrade, may undergo the full release process.

**Feature Release**

Patch releases in odd minor releases (1.3.0, 1.5.0, etc.) like 1.3.1, 1.5.2 and etc are corresponding to another **feature releases** with new development and bugfixes. These releases are not mandatory, but still **highly** recommended, **as they may contain critical security fixes**.

---

### Performing a Release

At the beginning of each release cycle, we will generate our "Release tracking issue", which is populated with the content at [https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md) 

This template will be used to track major goals we have, a planned shipping date, and a complete release checklist tied to a specific release.

### Security Fix Policy

Any release may contain security fixes. Unless the fix addresses a bug being exploited in the wild, the fix will *not* be called out in the release notes. Please make sure to update ASAP.

By policy, the team will usually wait until about 3 weeks after the final release to announce any fixed security issues. However, depending on the impact and ease of discovery of the issue, the team may wait more or less time.

 It is important to always update to the latest version ASAP and file issues if you're unable to update for some reason.

Finally, unless a security issue is actively being exploited or a significant number of users are unable to update to the latest version (e.g., due to a difficult migration, breaking changes, etc.), security fixes will *not* be backported to previous releases.

## FAQ

### Why aren't Go major versions used more?

Golang tightly couples source code with versioning (major versions beyond v1 leak into import paths).  This poses logistical difficulties to using major versions here.  Concretely, if we were to pick a policy that bumped the major version on every network upgrade, we would disrupt every single downstream library/application that consumed the native Lotus API (e.g., libraries depending on the JSON-RPC client, testground tests). They would need to update their code every single time that we released a network breaking change, even if it brought on zero expectation of breakage for the Golang APIs that they depend on. In this scenario, we are signaling breakage on the wrong API surface! We're signaling breakage on the Go level, when what breaks is the network protocol.

## Related Items

1. [Release Issue template](https://github.com/filecoin-project/lotus/blob/master/documentation/misc/RELEASE_ISSUE_TEMPLATE.md)
2. [Lotus Release Flow Discussion](https://github.com/filecoin-project/lotus/discussions/7053): Leave a comment if you have any questions or feedbacks with regard to the lotus release flow.
