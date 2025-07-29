# SplitStore: An actively scalable blockstore for the Filecoin chain

The SplitStore was first introduced in lotus v1.5.1, as an experiment
in reducing the performance impact of large blockstores.

With lotus v1.11.1, we introduce the next iteration in design and
implementation, which we call SplitStore v1.

The new design (see [#6474](https://github.com/filecoin-project/lotus/pull/6474)
evolves the splitstore to be a freestanding compacting blockstore that
allows us to keep a small (60-100GB) working set in a hot blockstore
and reliably archive out of scope objects in a coldstore.  The
coldstore can also be a discard store, whereby out of scope objects
are discarded or a regular badger blockstore (the default), which can
be periodically garbage collected according to configurable user
retention policies.

To enable the splitstore, edit `.lotus/config.toml` and add the following:
```
[Chainstore]
  EnableSplitstore = true
```

If you intend to use the discard coldstore, you also need to add the following:
```
  [Chainstore.Splitstore]
    ColdStoreType = "discard"
```
In general you _should not_ have to use the discard store, unless you
are running a network assistive node (like a bootstrapper or booster)
or have very constrained hardware with not enough disk space to
maintain a coldstore, even with garbage collection. It is also appropriate
for small nodes that are simply watching the chain.

*Warning:* Using the discard store for a general purpose node is discouraged, unless
you really know what you are doing. Use it at your own risk.

## Configuration Options

These are options in the `[Chainstore.Splitstore]` section of the configuration:

- `HotStoreType` -- specifies the type of hotstore to use.
  The only currently supported option is `"badger"`.
- `ColdStoreType` -- specifies the type of coldstore to use.
  The default value is `"universal"`, which will use the initial monolith blockstore
  as the coldstore.
  The other possible value is `"discard"`, as outlined above, which is specialized for
  running without a coldstore. Note that the discard store wraps the initial monolith
  blockstore and discards writes; this is necessary to support syncing from a snapshot.
- `MarkSetType` -- specifies the type of markset to use during compaction.
  The markset is the data structure used by compaction/gc to track live objects.
  The default value is "badger", which will use a disk backed markset using badger.
  If you have a lot of memory (48G or more) you can also use "map", which will use
  an in memory markset, speeding up compaction at the cost of higher memory usage.
  Note: If you are using a VPS with a network volume, you need to provision at least
  3000 IOPs with the badger markset.
- `HotStoreMessageRetention` -- specifies how many finalities, beyond the 4
  finalities maintained by default, to maintain messages and message receipts in the
  hotstore. This is useful for assistive nodes that want to support syncing for other
  nodes beyond 4 finalities, while running with the discard coldstore option.
  It is also useful for miners who accept deals and need to lookback messages beyond
  the 4 finalities, which would otherwise hit the coldstore.
- `HotStoreFullGCFrequency` -- specifies how frequently to garbage collect the hotstore
  using full (moving) GC.
  The default value is 20, which uses full GC every 20 compactions (about once a week);
  set to 0 to disable full GC altogether.
  Rationale: badger supports online GC, and this is used by default. However it has proven to
  be ineffective in practice with the hotstore size slowly creeping up. In order to address this,
  we have added moving GC support in our badger wrapper, which can effectively reclaim all space.
  The downside is that it takes a bit longer to perform a moving GC and you also need enough
  space to house the new hotstore while the old one is still live.


## Operation

When the splitstore is first enabled, the existing blockstore becomes
the coldstore and a fresh hotstore is initialized.

The hotstore is warmed up on first startup so as to load all chain
headers and state roots in the current head.  This allows us to
immediately gain the performance benefits of a smallerblockstore which
can be substantial for full archival nodes.

All new writes are directed to the hotstore, while reads first hit the
hotstore, with fallback to the coldstore.

Once 5 finalities have elapsed, and every finality henceforth, the
blockstore _compacts_.  Compaction is the process of moving all
unreachable objects within the last 4 finalities from the hotstore to
the coldstore. If the system is configured with a discard coldstore,
these objects are discarded. Note that chain headers, all the way to
genesis, are considered reachable. Stateroots and messages are
considered reachable only within the last 4 finalities, unless there
is a live reference to them.

## Compaction

Compaction works transactionally with the following algorithm:
- We prepare a transaction, whereby all i/o referenced objects through the API are tracked.
- We walk the chain and mark reachable objects, keeping 4 finalities of state roots and messages and all headers all the way to genesis.
- Once the chain walk is complete, we begin full transaction protection with concurrent marking; we walk and mark all references created during the chain walk. On the same time, all I/O through the API concurrently marks objects as live references.
- We collect cold objects by iterating through the hotstore and checking the mark set; if an object is not marked, then it is candidate for purge.
- When running with a coldstore, we next copy all cold objects to the coldstore.
- At this point we are ready to begin purging:
  - We sort cold objects heaviest first, so as to never delete the constituents of a DAG before the DAG itself (which would leave dangling references)
  - We delete in small batches taking a lock; each batch is checked again for marks, from the concurrent transactional mark, so as to never delete anything live
- We then end the transaction and compact/gc the hotstore.

As of [#8008](https://github.com/filecoin-project/lotus/pull/8008) the compaction algorithm has been
modified to eliminate sorting and maintain the cold object set on disk. This drastically reduces
memory usage; in fact, when using badger as the markset compaction uses very little memory, and
it should be now possible to run splitstore with 32GB of RAM or less without danger of running out of
memory during compaction.

## Garbage Collection

TBD -- see [#6577](https://github.com/filecoin-project/lotus/issues/6577)

## Utilities

`lotus-shed` has a `splitstore` command which provides some utilities:

- `rollback` -- rolls back a splitstore installation.
  This command copies the hotstore on top of the coldstore, and then deletes the splitstore
  directory and associated metadata keys.
  It can also optionally compact/gc the coldstore after the copy (with the `--gc-coldstore` flag)
  and automatically rewrite the lotus config to disable splitstore (with the `--rewrite-config` flag).
  Note: the node *must be stopped* before running this command.
- `clear` -- clears a splitstore installation for restart from snapshot.
- `check` -- asynchronously runs a basic healthcheck on the splitstore.
  The results are appended to `<lotus-repo>/datastore/splitstore/check.txt`.
- `info` -- prints some basic information about the splitstore.
