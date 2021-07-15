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

## Chain Pruning and Garbage Collection

The coldstore can be pruned and garbage collection using the `lotus chain prune` command.
Note that the command initiates pruning, but runs asynchronously as it can take a long time to
complete.

By default, pruning keeps all chain reachable object; the user however has the option to specify
a retention policy for old state roots and message receipts with the `--retention` option.
The value is an integer with the following semantics:
- If it is -1 then all state objects reachable from the chain will be retained in the coldstore.
  This is the (safe) default.
- If it is 0 then no state objects that are unreachable within the compaction boundary will
  be retained in the coldstore.
  This effectively throws away all old state roots and it is maximally effective at reclaiming space.
- If it is a positive integer, then it's the number of finalities past the compaction boundary
  for which chain-reachable state objects are retained.
  This allows you to keep some older state roots in case you need to reset your head outside
  the compaction boundary or perform historical queries.

During pruning, unreachable objects are deleted from the coldstore. In order to reclaim space,
you also need to specify a gargage collection policy, with two possible options:
- The `--online-gc` flag performs online garbage collection; this is fast but does not reclaim all
  space possible. This is the default.
- The `--moving-gc` flag performs moving garbage collection, where the coldstore is moved,
  copying only live objects. If your coldstore lives outside the `.lotus` directory, e.g. with
  a symlink to a different file system comprising of cheaper disks, then you can specify the
  directory to move to with the `--move-to` option.
  This reclaims all possible space, but it is slow and also requires disk space to house the new
  coldstore together with the old coldstore during the move.
