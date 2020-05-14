# Genesis block

Seems a good way to start exploring the VM state though the instantiation of its different actors like the storage power.

Explain where do we load the genesis block, the CAR entries, and we set the root of the state. Follow the daemon command option, `chain.LoadGenesis()` saves all the blocks of the CAR file into the store provided by `ChainBlockstore` (this should already be explained in the previous section). The CAR root (MT root?) of those blocks is decoded into the `BlockHeader` that will be the Filecoin (genesis) block of the chain, but most of the information was stored in the raw data (non-Filecoin, what's the correct term?) blocks forwarded directly to the chain, the block header just has a pointer to it.

`SetGenesis` block with name 0. `(ChainStore).SetGenesis()` stores it there.

`MakeInitialStateTree` (`chain/gen/genesis/genesis.go`, used to construct the genesis block (`MakeGenesisBlock()`), constructs the state tree (`NewStateTree`) which is just a "pointer" (root node in the HAMT) to the different actors. It will be continuously used in `(*StateTree).SetActor()` an `types.Actor` structure under a certain `Address` (in the HAMT). (How does the `stateSnaps` work? It has no comments.)

From this point we can follow different setup function like:

* `SetupInitActor()`: see the `AddressMap`.

* `SetupStoragePowerActor`: initial (zero) power state of the chain, most important attributes.

* Account actors in the `template.Accounts`: `SetActor`.

Which other actor type could be helpful at this point?

# Basic concepts

What should be clear at this point either from this document or the spec.

## Addresses

## Accounts

# Sync Topics PubSub

Gossip sub spec and some introduction.

# Look at the constructor of a miner

Follow the `lotus-storage-miner` command to see how a miner is created, from the command to the message to the storage power logic.

# Directory structure so far, main structures seen, their relation

List what are the main directories we should be looking at (e.g., `chain/`) and the most important structures (e.g., `StateTree`, `Runtime`, etc.)

# Tests

Run a few messages and observe state changes. What is the easiest test that also let's us "interact" with it (modify something and observe the difference).
