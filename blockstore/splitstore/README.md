# SplitStore: An actively scalable blockstore for the Filecoin chain

The SplitStore was first introduced in lotus v1.5.1, as an experiment
in reducing the performance impact of large blockstores.

With lotus v1.19.0, we introduce the next iteration in design and
implementation, which we call SplitStore v2.

The new design (see [#9056](https://github.com/filecoin-project/lotus/pull/9056) and [#9128](https://github.com/filecoin-project/lotus/discussions/9128))
evolves the splitstore to be a freestanding compacting blockstore that allows you to keep a small 60 GiB to 275 GiB working set in a hot blockstore and reliably archive out-of-scope objects which are historical chain objects that are no onger needed for the vast majority of Lotus use cases into a coldstore. The coldstore can also be a `discard` store, whereby out-of-scope objects are discarded, a `universal` store, which will store all chain data or a `messages` store which will only store on-chain messages and message receipts. The `messages` badger blockstore is the default storage type.

```
CompactionThreshold is the number of epochs that need to have elapsed
from the previously compacted epoch to trigger a new compaction.

       |················· CompactionThreshold ··················|
       |                                             |
=======‖≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡≡‖------------------------»
       |                    |  chain -->             ↑__ current epoch
       | archived epochs ___↑
                            ↑________ CompactionBoundary

=== :: cold (already archived)
≡≡≡ :: to be archived in this compaction
--- :: hot
```

To enable the splitstore, edit `.lotus/config.toml` and add the following:
```
[Chainstore]
  # type: bool
  # env var: LOTUS_CHAINSTORE_ENABLESPLITSTORE
  EnableSplitstore = true
```

If you intend to use the `discard` coldstore, you also need to add the following:
```
[Chainstore.Splitstore]
  # ColdStoreType specifies the type of the coldstore.
  # It can be "messages" (default) to store only messages, "universal" to store all chain state or "discard" for discarding cold blocks.
  #
  # type: string
  # env var: LOTUS_CHAINSTORE_SPLITSTORE_COLDSTORETYPE
  ColdStoreType = "discard"
```
For Storage Providers who have no need to perform historical chain quereis, the discard ColdStoreType can be utilised without issue. For users who do need to perform historical chain quesries both messages and universal options can be set depending on your specific use case. Further details can be found in the [Lotus docs](https://lotus.filecoin.io/lotus/configure/splitstore/).

## Configuration Options

```toml
[Chainstore]
  # type: bool
  # env var: LOTUS_CHAINSTORE_ENABLESPLITSTORE
  EnableSplitstore = false

  [Chainstore.Splitstore]
    # ColdStoreType specifies the type of the coldstore.
    # It can be "messages" (default) to store only messages, "universal" to store all chain state or "discard" for discarding cold blocks.
    #
    # type: string
    # env var: LOTUS_CHAINSTORE_SPLITSTORE_COLDSTORETYPE
    ColdStoreType = "messages"

    # HotStoreType specifies the type of the hotstore.
    # Only currently supported value is "badger".
    #
    # type: string
    # env var: LOTUS_CHAINSTORE_SPLITSTORE_HOTSTORETYPE
    HotStoreType = "badger"

    # MarkSetType specifies the type of the markset.
    # It can be "map" for in memory marking or "badger" (default) for on-disk marking.
    #
    # type: string
    # env var: LOTUS_CHAINSTORE_SPLITSTORE_MARKSETTYPE
    MarkSetType = "badger"

    # HotStoreMessageRetention specifies the retention policy for messages, in finalities beyond
    # the compaction boundary; default is 0.
    #
    # type: uint64
    # env var: LOTUS_CHAINSTORE_SPLITSTORE_HOTSTOREMESSAGERETENTION
    HotStoreMessageRetention = 0

    # HotStoreFullGCFrequency specifies how often to perform a full (moving) GC on the hotstore.
    # A value of 0 disables, while a value 1 will do full GC in every compaction.
    # Default is 20 (about once a week).
    #
    # type: uint64
    # env var: LOTUS_CHAINSTORE_SPLITSTORE_HOTSTOREFULLGCFREQUENCY
    HotStoreFullGCFrequency = 20
```
The `HotStoreFullGCFrequency` value referes to finalities

## Operation

When the splitstore is first enabled, the existing blockstore becomes the coldstore and a fresh hotstore is initialized.

The hotstore is warmed up on the first startup to load all chain headers and state roots in the current head. This process allows us to immediately gain the performance benefits of a smaller blockstore, which can be substantial for full archival nodes.

All new writes are directed to the hotstore, while reads first hit the hotstore with fallback to the coldstore.

Once five finalities (4500 epochs) have elapsed and every subsequent finality, the blockstore _compacts_. Compaction is the process of moving all unreachable objects within the last four finalities from the hotstore to the coldstore. These objects are discarded if the system is configured with a discard coldstore. Chain headers are considered reachable all the way to the genesis block. Stateroots and messages are considered reachable only within the last four finalities unless there is a live reference to them.

## Compaction

Compaction works transactionally with the following algorithm:

- We prepare a transaction whereby all i/o referenced objects through the API are tracked.
- We walk the chain and mark reachable objects, keeping four finalities of state roots and messages and all headers all the way to genesis.
- Once the chain walk is complete, we begin full transaction protection with concurrent marking; we walk and mark all references created during the chain walk. At the same time, all I/O through the API concurrently marks objects as live references.
- We collect cold objects by iterating through the hotstore and checking the mark set; if an object is not marked, then it is a candidate for purge.
- When running with a coldstore, we next copy all cold objects to the coldstore.
- At this point, we are ready to begin purging
- We then end the transaction and compact/garbage collect the hotstore.
- We delete in small batches taking a lock; each batch is checked again for marks, from the concurrent transactional mark, so as to never delete anything live

## Cold Store Garbage Collection

Garbage collection can be performed manually by running the `lotus chain prune <flags>` command.

## Relocating the Coldstore

Following successful configuration and activation of the SplitStore it is now also possible to further optimise daemon chain storage by relocating the coldstore data to slower and potentially less critical standard spinning disks. This can be accomplished by simply symlinking the current `/<lotus-repo>/datastore/chain` folder to a new folder located in your standard storage path.

```shell
mkdir /<standard-storage-path>/chain
ln -s /<lotus-repo>/datastore/chain /<standard-storage-path>/chain
```

## Utilities

`lotus-shed` has a `splitstore` command which provides some utilities:

- `rollback` -- rolls back a splitstore installation. This command copies the hotstore on top of the coldstore, and then deletes the splitstore directory and associated metadata keys. It can also optionally compact/gc the coldstore after the copy (with the `--gc-coldstore` flag) and automatically rewrite the lotus config to disable splitstore (with the `--rewrite-config` flag). The node *must be stopped* before running this command.
- `clear` -- clears a splitstore installation for restart from snapshot.
- `check` -- asynchronously runs a basic healthcheck on the splitstore.
  The results are appended to `<lotus-repo>/datastore/splitstore/check.txt`.
- `info` -- prints some basic information about the splitstore.
