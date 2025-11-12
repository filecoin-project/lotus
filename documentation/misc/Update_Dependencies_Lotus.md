# Updating Lotus Dependencies

This guide will walk through how to update the most common dependencies in Lotus. These are the dependencies this guide currently covers:

<!-- no toc -->
- [Updating Ref-FVM](#updating-ref-fvm)
- [Updating Filecoin-FFI](#updating-filecoin-ffi)
- [Updating Go-State-Types](#updating-go-state-types)
- [Updating Builtin-Actors](#updating-builtin-actors)

## Context

Updating these dependencies in Lotus is usually related to network upgrades.  See [building a network upgrade skeleton in Lotus context](./Building_a_network_skeleton.md#context) for information on the versions and relationships at play.

## Updating Ref-FVM

1. The Ref-FVM dependency is updated through Filecoin-FFI. So, if you need to update Ref-FVM, you would need create a Filecoin-FFI PR similar to this: [PR updating Ref-FVM in Filecoin-FFI](https://github.com/filecoin-project/filecoin-ffi/pull/447)

2. After the PR has been merged you would need to create a [new Filecoin-FFI release](https://github.com/filecoin-project/filecoin-ffi?tab=readme-ov-file#release-process).

3. After the Filecoin-FFI release is out, you can follow the process outlined in [Filecoin-FFI](#updating-filecoin-ffi).

## Updating Filecoin-FFI

0. In Lotus´s [go.mod file](https://github.com/filecoin-project/lotus/blob/master/go.mod), search for `filecoin-ffi` and update the version to your wanted version.

0. Run `go mod tidy`, and commit your changes.

1. In your `lotus` directory, `cd extern/filecoin-ffi`.

2. `git fetch` to ensure you have the latest changes for *filecoin-ffi*.

3. `git checkout vX.Y.Z` to checkout the version you want to update to.

4. Then commit the update to your Lotus branch and open a PR for updating Filecoin-FFI.

👉 Example of a [PR updating Filecoin-FFI](https://github.com/filecoin-project/lotus/pull/11431)

👉 If you need to create a Filecoin-FFI release, you can follow [the release process](https://github.com/filecoin-project/filecoin-ffi?tab=readme-ov-file#release-process).

## Updating Go-State-Types

1. In Lotus´s [go.mod file](https://github.com/filecoin-project/lotus/blob/master/go.mod), search for `go-state-types` and update the version to your wanted version.

2. Run `go mod tidy`, and commit your changes.

👉 Example of a [PR updating Go-State-Types](https://github.com/filecoin-project/lotus/pull/11732)

👉 If you need to create a Go-State-Types release, you can follow the steps to create a [new Go-State-Types release](https://github.com/filecoin-project/go-state-types?tab=readme-ov-file#release-process).

## Updating Builtin-Actors

1. In your `lotus` directory, `cd build/actors`.

2. Run this script `./pack.sh vXX vX.Y.Z-rcX` to pull in the builtin-actors bundle into your Lotus repo. 

- `vXX` is the network version you are bundling this builtin-actors for.
- `vX.Y.Z-rcX` is the builtin-actors release you are bundling.

👉 Example of a [PR updating Builtin-Actors bundle](https://github.com/filecoin-project/lotus/pull/11682/)

👉 If you need to create a Builtin-Actors release, you can follow the steps to create a [new Builtin-Actors release](https://github.com/filecoin-project/builtin-actors/?tab=readme-ov-file#releasing).