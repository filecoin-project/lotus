# Network Upgrade Skeleton in Lotus

This guide will walk you through the process of creating a skeleton for a network upgrade in Lotus. The process involves making changes in multiple repositories in the following order:

1. [`ref-fvm`](#ref-fvm-checklist)
2. [`filecoin-ffi`](#filecoin-ffi-checklist)
3. [`go-state-types`](#go-state-types-skeleton-checklist)
4. [`lotus`](#lotus-checklist)

Each repository has its own set of steps that need to be followed. This guide will provide detailed instructions for each repository.

## Setup

1. Clone the [ref-fvm](https://github.com/filecoin-project/ref-fvm.git) repository.

1. Clone the [filecoin-ffi](https://github.com/filecoin-project/filecoin-ffi.git) repository.

1. Clone the [go-state-types](https://github.com/filecoin-project/go-state-types) repository.

2. In your Lotus repository, add `replace github.com/filecoin-project/go-state-types => ../go-state-types` to the very end of your Lotus `go.mod` file.
    - This ensures that your local clone copy of `go-state-types` is used. Any changes you make there will be reflected in your Lotus project.

## Ref-FVM Checklist

1. Add support for the new network version in Ref-FVM:

    - In `fvm/src/gas/price_list.rs` add the new network version to the `price_list_by_network_version` function.
    - In `fvm/src/machine/default.rs` in the new function of your machine context, you will find a `SUPPORTED_VERSIONS` constant that defines the range of network versions (for networks that are not Hyperspace) supported. Bump this range to support your new network version.
    - In `shared/src/version/mod.rs`, in the `NetworkVersion` implementation, you will find a series of constants representing different network versions. To add a new network version, you need to declare a new constant: `pub const (VXX+1): Self = Self(XX+1);` 

## Filecoin-FFI Checklist

## Go-State-Types Skeleton Checklist

1. Follow the [go-state-types actor version checklist](https://github.com/filecoin-project/go-state-types/blob/master/actors_version_checklist.md):

    - Copy `go-state-types/builtin/vX` to `go-state-types/builtin/v(X+1)`.
    - Change all references from vX to v(X+1) in the new files.
    - Add new network version to `network/version.go`.
    - Add new actors version to `actors/version.go`.
        - Add `Version(XX+1) Version = XX+1` as a constant.
        - In `func VersionForNetwork` add `case network.Version(XX+1): return Version(XX+1), nil`.
    - Add the new version to the gen step of the makefile.
        - Add `$(GO_BIN) run ./builtin/v(XX+1)/gen/gen.go`.
    
## Actor Version Lotus Integration Checklist

1. Import new actors:

    - Create a mock actor-bundle for the new network version.
    - In `/build/actors` run `./pack.sh vXX+1 vXX.0.0` where XX is the current actor bundle version.

2. Define upgrade heights in `build/params_`:

    - Update the following files:
        - `params_2k.go`
            - Set previous `UpgradeXxxxxHeight = abi.ChainEpoch(-xx-1)`
            - Add `var UpgradeXxxxxHeight = abi.ChainEpoch(200)`
            - Add `UpgradeXxxxxHeight = getUpgradeHeight("LOTUS_XXXXX_HEIGHT", UpgradeXXXXHeight)`
            - Set `const GenesisNetworkVersion = network.VersionXX` where XX is the network version you are upgrading from.
        - `params_butterfly.go`
            - Add comment with ?????? signaling that the new upgrade date is unkown
            - Add `const UpgradeXxxxxHeight = 999999999999999`
        - `params_calibnet.go`
            - Add comment with `??????` signaling that the new upgrade date is unkown
            - Add `const UpgradeXxxxxHeight = 999999999999999`
        - `params_interop.go`
            - set previous upgrade to `var UpgradeXxxxxHeigh = abi.ChainEpoch(-xx-1)`
            - Add `const UpgradeXxxxxHeight = 50`
        - `params_mainnet.go`
            - Set previous upgrade to `const UpgradeXxxxxHeight = XX`
            - Add comment with ???? signaling that the new upgrade date is unkown
            - Add `var UpgradeXxxxxxHeight = abi.ChainEpoch(9999999999)`
            - Change the `LOTUS_DISABLE_XXXX` env variable to the new network name
        - `params_testground.go`
            - Add `UpgradeXxxxxHeight     abi.ChainEpoch = (-xx-1)`

3. Generate adapters:

    - Update `gen/inlinegen-data.json`.
        - Add `XX+1` to "actorVersions" and set "latestActorsVersion" to `XX+1`.
        - Add `XX+1` to "networkVersions" and set "latestNetworkVersion" to `XX+1`.

    - Run `make actors-gen`. This generates the `/chain/actors/builtin/*` code, `/chain/actors/policy/policy.go` code, `/chain/actors/version.go`, and `/itest/kit/ensemble_opts_nv.go`.

4. Update `chain/consensus/filcns/upgrades.go`.
    - Import `nv(XX+1) "github.com/filecoin-project/go-state-types/builtin/v(XX+1)/migration`.
    - Add Schedule. [^1]
    - Add Migration. [^2]

5. Add actorstype to the NewActorRegistry in `/chain/consensus/computestate.go`.
    - Add `inv.Register(actorstypes.Version(XX+1), vm.ActorsVersionPredicate(actorstypes.Version(XX+1)), builtin.MakeRegistry(actorstypes.Version(XX+1))`.

6. Add upgrade field to `api/types.go/ForkUpgradeParams`.
    - Add `UpgradeXxxxxHeight      abi.ChainEpoch` to `ForkUpgradeParams` struct.

7. Add upgrade to `node/impl/full/state.go`.
    - Add `UpgradeXxxxxHeight:      build.UpgradeXxxxxHeight,`.

8. Add network version to `chain/state/statetree.go`.
    - Add `network.VersionXX+1` to `VersionForNetwork` function.

9. Run `make gen`.

10. Run `make docsgen-cli`.

And you're done! This should create a network upgrade skeleton that you are able to run locally with your local go-state-types clones, and a mock Actors-bundle. This will allow you to:

- Have a local developer network that starts at the current network version.
- Be able to see the Actor CIDs/Actor version for the mock v12-bundle through `lotus state actors-cids --network-version XX+1`
- Have a successful pre-migration.
- Complete Migration at upgrade epoch, but fail immidiately after the upgrade.

At this point you are blocked on FVM/Actors work landing.

// TODO: Create a video-tutorial going through all the steps

[^1]: Here is an example of how you can add a schedule:

    ```go
    {
        Height:    build.UpgradeXxxxHeight,
        Network:   network.Version(XX+1),
        Migration: UpgradeActorsV(XX+1),
        PreMigrations: []stmgr.PreMigration{{
            PreMigration:    PreUpgradeActors(VXX+1),
            StartWithin:     120,
            DontStartWithin: 15,
            StopWithin:      10,
        }},
        Expensive: true,
    },
    ```

    This schedule should be added to the `DefaultUpgradeSchedule` function, specifically within the `updates` array.

[^2]: Here is an example of how you can add a migration:

    ```go
    func PreUpgradeActorsV(XX+1)(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
        // Use half the CPUs for pre-migration, but leave at least 3.
        workerCount := MigrationMaxWorkerCount
        if workerCount <= 4 {
            workerCount = 1
        } else {
            workerCount /= 2
        }

        lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
        if err != nil {
            return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
        }

        config := migration.Config{
            MaxWorkers:        uint(workerCount),
            ProgressLogPeriod: time.Minute * 5,
        }

        _, err = upgradeActorsV(XX+1)Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
        return err
    }

    func UpgradeActorsV(XX+1)(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
        root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
        // Use all the CPUs except 2.
        workerCount := MigrationMaxWorkerCount - 3
        if workerCount <= 0 {
            workerCount = 1
        }
        config := migration.Config{
            MaxWorkers:        uint(workerCount),
            JobQueueSize:      1000,
            ResultQueueSize:   100,
            ProgressLogPeriod: 10 * time.Second,
        }
        newRoot, err := upgradeActorsV(XX+1)Common(ctx, sm, cache, root, epoch, ts, config)
        if err != nil {
            return cid.Undef, xerrors.Errorf("migrating actors v11 state: %w", err)
        }
        return newRoot, nil
    }

    func upgradeActorsV(XX+1)Common(
        ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
        root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
        config migration.Config,
    ) (cid.Cid, error) {
        writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
        adtStore := store.ActorStore(ctx, writeStore)
        // ensure that the manifest is loaded in the blockstore
        if err := bundle.LoadBundles(ctx, writeStore, actorstypes.Version(XX+1)); err != nil {
            return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
        }

        // Load the state root.
        var stateRoot types.StateRoot
        if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
            return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
        }

        if stateRoot.Version != types.StateTreeVersion5 {
            return cid.Undef, xerrors.Errorf(
                "expected state root version 5 for actors v(XX+1) upgrade, got %d",
                stateRoot.Version,
            )
        }

        manifest, ok := actors.GetManifest(actorstypes.Version(XX+1))
        if !ok {
            return cid.Undef, xerrors.Errorf("no manifest CID for v(XX+1) upgrade")
        }

        // Perform the migration
        newHamtRoot, err := nv(XX+1).MigrateStateTree(ctx, adtStore, manifest, stateRoot.Actors, epoch, config,
            migrationLogger{}, cache)
        if err != nil {
            return cid.Undef, xerrors.Errorf("upgrading to actors v11: %w", err)
        }

        // Persist the result.
        newRoot, err := adtStore.Put(ctx, &types.StateRoot{
            Version: types.StateTreeVersion5,
            Actors:  newHamtRoot,
            Info:    stateRoot.Info,
        })
        if err != nil {
            return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
        }

        // Persists the new tree and shuts down the flush worker
        if err := writeStore.Flush(ctx); err != nil {
            return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
        }

        if err := writeStore.Shutdown(ctx); err != nil {
            return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
        }

        return newRoot, nil
    }
    ```