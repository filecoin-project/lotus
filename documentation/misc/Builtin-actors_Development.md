# builtin-actors Development with Lotus

This guide will walk through how to develop, update and test builtin-actors from within Lotus. The aim of this guide is to make it easier to contribute to builtin-actors by providing a clear path to follow through the somewhat complex web of dependencies and processes involved in integrating changes through to Lotus.

## Context

[builtin-actors](https://github.com/filecoin-project/builtin-actors) is a Rust codebase that compiles to a set of WASM binaries that are bundled into CAR files and executed by the [FVM](https://github.com/filecoin-project/ref-fvm/) as required by messages in the Filecoin network. These actors are used to manage the state transitions of the Filecoin network and are critical to the operation of the network. The builtin-actors bundles are not just used by Lotus, but are shared by all Filecoin implementations. The compiled bundles are referenced by their CID for each network version. That CID must match what is in the CAR file and this process is repeated by each Filecoin implementation. Differences in the compiled actors can lead to consensus failures.

The file [`builtin_actors_gen.go`](../../build/builtin_actors_gen.go) is generated (by the `actors-gen` Makefile target, i.e. `make actors-gen`) and contains a mapping of CID to the compiled actor code for each network version. [`build/actors/`](../../build/actors/) contains compressed tar files with CAR files for each network type (mainnet, calibnet, devnet, etc.) supported by Lotus. When executing Filecoin messages, the correct actors bundle for the network type and version for the particular epoch is loaded using this mapping and the WASM code is executed by the FVM to process the message.

Changes to builtin-actors code can have subtle impacts on message execution, primarily through gas accounting, which, among other things, accounts for the cost of executing individual WASM instructions. Executing a message for one network version using a different builtin-actors bundle can lead to different gas costs, which will lead to consensus failures because your local node will not agree with the rest of the network on the cost of executing a message and the state transition that results. Therefore, changes to builtin-actors code must either be executed on a separate network (e.g. a devnet), or be included in a coordinated network upgrade.

Testing integration of builtin-actors code typically involves bundling pre-release builds of the actors bundles into Lotus and executing an isolated devnet, primarily using the `itests` suite. In this way, you can simulate a version of the network that executes your custom bundle and exercise the changes in functionality you are developing. You can even simulate a network upgrade process where the network switches from one bundle to another. This is described in more detail below.

### Builtin-actors-only changes

#### Environment variable override

When developing builtin-actors, the `bundle-devnet` Makefile target can be used to compile a devenet ("2k") version of builtin-actors into a CAR file in the `output/` directory. This bundle is immediately usable by Lotus for testing using the `LOTUS_BUILTIN_ACTORS_VXX` environment variable. For a given actors version, `XX`, which corresponds to the network version you want to replace, you can execute itests using the `LOTUS_BUILTIN_ACTORS_VXX` environment variable to test the changes you have made. The file [`chain/consensus/filcns/upgrades.go`](../../chain/consensus/filcns/upgrades.go) contains the full list of mappings from network versions to actors versions. e.g. `LOTUS_BUILTIN_ACTORS_V15=/path/to/builtin-actors/output/builtin-actors-devnet.car go test ./itests/test_foo.go` will run `test_foo.go` using the `builtin-actors-devnet.car` bundle if the test expects to load actors version 15. By default, itests will run using the latest network / actors version. So if the current network version is NV15, then actors version 15 will be loaded by default and `LOTUS_BUILTIN_ACTORS_V15` will intercept the load and use the specified bundle instead of the devnet bundle packaged in [`build/actors/`](../../build/actors/). In this way you can run integration tests manually off a custom actors build.

This method does not extend to being able to run your Lotus changes in CI with a pull request as this override is not possible in that environment. This method is only useful for local development and testing, but can be used until a tagged and released version of builtin-actors containing your changes is checked into the lotus repository.

#### Replacing actors bundles

An alternative method to the environment variable override is to simply replace the current `vX.tar.zst` file in [`build/actors/`](../../build/actors/) with your custom bundle. It is important that you not attempt to check your replaced file in to the Lotus codebase **unless it is a tagged (and verified) builtin-actors bundle**. This method can be used to check in code for pull requests to run in CI, but it is important that custom bundles are not merged unless they are the tagged version—this process is typically handled separately during the network upgrade process, so be prepared to remove your custom bundle in your commit tree before having your pull request merged.

First you will need a complete set of builtin-actors bundles for all of the network types. The `all-bundles` Makefile target in builtin-actors (`make all-bundles`) will generate these for you and place them into the `output/` directory. You can then use the [`build/actors/pack.sh`](../../build/actors/pack.sh) script to compress these into the `vX.tar.zst` files that Lotus expects in that directory.

**If you want to bundle a tagged builtin-actors version**, e.g. `v15.0.0-rc1`:

```
cd build/actors/
./pack.sh v15 v15.0.0-rc1
```

This will fetch the CAR files from the [GitHub release page for `v15.0.0-rc1`](https://github.com/filecoin-project/builtin-actors/releases/tag/v15.0.0-rc1) and create a replacement for `build/actors/v15.tar.zst` and generate an update to `build/builtin_actors_gen.go`. If v15 is the correct actors version for the current network version (as per upgrades.go), then running itests, by default, will use your new bundle.

**If you want to bundle a custom build of builtin-actors from your local filesystem**:

```
cd build/actors/
./pack.sh v15 local/20240930 /path/to/builtin-actors/output/
```

Where `local/20240930` is some arbitrary identifying string that is used for the `BundleGitTag` in `build/builtin_actors_gen.go`. Instead of fetching from the network, it will expect the CAR files to exist in `/path/to/builtin-actors/output/` and will create a replacement for `build/actors/v15.tar.zst` and generate an update to `build/builtin_actors_gen.go`.

#### Writing tests

By default, the itests framework will use the current latest network version. Edits to the `build/builtin_actors_gen.go` file will result in an updated `latestNetworkVersion` in `gen/inlinegen-data.json`, which will result in a new `TestNetworkVersion` in [`build/buildconstants/params.go`](../../build/buildconstants/params.go) which is used in `DefaultEnsembleOpts` in [`itests/kit/ensemble_opts.go`](../../itests/kit/ensemble_opts.go) which will set the genesis network version of itests unless it is overridden.

...

### Builtin-actors and (go-state-types) state migration changes

*TODO*

### Builtin-actors, (go-state-types) state migration and FVM changes

*TODO*