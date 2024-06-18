# Updating Lotus Dependencies

This guide will walk through how to update the most common dependencies in Lotus. These are the dependencies this guide currently covers:

- [Filecoin-FFI](#updating-filecoin-ffi)
- [Ref-FVM](#updating-ref-fvm)
- [Go-State-Types](#updating-go-state-types)
- [Builtin-Actors](#updating-builtin-actors)

## Updating Filecoin-FFI

1. In your `/lotus` folder, run `cd extern/filecoin-ffi`.

2. `git pull` to ensure you have the latests changes for *filecoin-ffi*.

3. `git checkout vX.XX.X` to checkout the version you want to update to.

4. Then commit the update to your Lotus branch and open a PR for updating Filecoin-FFI.

👉 Example of a [PR updating Filecoin-FFI](https://github.com/filecoin-project/lotus/pull/11431) 

## Updating Ref-FVM

## Updating Go-State-Types

1. In Lotus´s [go.mod file](https://github.com/filecoin-project/lotus/blob/master/go.mod), search for `go-state-types` and update the version to your wanted version.

2. Run `go mod tidy`, and commit your changes.

👉 Example of a [PR updating Go-State-Types](https://github.com/filecoin-project/lotus/pull/11732) 

## Updating Builtin-Actors
