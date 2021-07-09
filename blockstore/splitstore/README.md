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

If you intend to use the discard coldstore, your also need to add the following:
```
  [Chainstore.Splitstore]
    ColdStoreType = "discard"
```
In general you _should not_ have to use the discard store, unless you
are running a network booster or have very constrained hardware with
not enough disk space to maintain a coldstore, even with garbage
collection.


## Operation

When the splitstore is first enabled, the existing blockstore becomes
the coldstore and a fresh hotstore is initialized.

The hotstore is warmed up on first startup so as to load all chain
headers and state roots in the current head.  This allows us to
immediately gain the performance benefits of a smallerblockstore which
can be substantial for full archival nodes.

All new writes are directed to the hotstore, while reads first hit the
hotstore, with fallback to the coldstore.

Once 5 finalities have ellapsed, and every finality henceforth, the
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
  - We sort cold objects heaviest first, so as to never delete the consituents of a DAG before the DAG itself (which would leave dangling references)
  - We delete in small batches taking a lock; each batch is checked again for marks, from the concurrent transactional mark, so as to never delete anything live
- We then end the transaction and compact/gc the hotstore.

## Coldstore Garbage Collection

TBD -- see [#6577](https://github.com/filecoin-project/lotus/issues/6577)
