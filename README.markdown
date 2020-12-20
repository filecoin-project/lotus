# 瘪莲 (ltsh)

A leaner variant of [莲](https://github.com/filecoin-project/lotus) powering all `riba.cloud` nodes.

## What

This repository contains the **PRODUCTION VERSION** of nodes used at and around `riba.cloud`. The code is the result of a series of patches on top of the [*reasonably-earliest* version of `lotus`][11] compatible with the current filecoin mainnet. The changes are either backports or modifications of various aspects of upstream, aimed at leaner and/or more correct operations.

## How

The repository never adds tags: named branches are used as source of truth. As upstream is tracked relatively closely, all branches, including [`master`][1] are **force-pushed to quite frequently**. Always use `git pull --rebase` when fetching new changes.

The main named-branches of interest are as follows:

- [`upstream_base`][11]: the base point matching a recent-ish-stable release of [`lotus`][2]

- [`backports`][12]: the portion of [`master`][1] that consists solely of backports from [`upstream-lotus:master`][2] on top of the [`upstream_base`][11].

- [`maybe_for_upstreaming`][13]: the portion of [`master`][1] that could reasonably be considered for inclusion upstream. Note that author **does not plan to raise PRs** against upstream. The marker is provided for those who would like to attempt upstreaming the changes on their own.

- [`master`][1]: code powering the production nodes which author uses for various chain-syncing/-analysis tasks. Drifts between daemon-reported commitish and this branch are generally rare. You are welcome to run this version, as long as you are doing it **AT YOUR OWN RISK** and agree to keep all the pieces if something breaks.

## Lead Maintainer

[Peter 'ribasushi' Rabbitson](https://github.com/ribasushi)

## License

SPDX-License-Identifier: Apache-2.0 OR MIT

[1]: https://github.com/ribasushi/ltsh/commits/master
[2]: https://github.com/filecoin-project/lotus
[11]: https://github.com/ribasushi/ltsh/commits/upstream_base
[12]: https://github.com/ribasushi/ltsh/compare/upstream_base...backports
[13]: https://github.com/ribasushi/ltsh/compare/backports...maybe_for_upstreaming

