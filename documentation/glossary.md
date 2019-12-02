# Glossary

**Chain: Types**

Implementation of data structures used by Filecoin and their serializations.

**Chain: Store**

The chainstore manages all local chain state, including block headers, messages, and state.

**Chain: State**

A package for dealing with the Filecoin state tree. Wraps the [HAMT](https://github.com/ipfs/go-hamt-ipld).

**Chain: Actors**

Implementations of the builtin Filecoin network actors.

**Chain: VM**

The Filecoin state machine 'vm'. Implemented here are utilities to invoke Filecoin actor methods.

**Miner**

The block producer logic. This package interfaces with the full node through the API, despite currently being implemented in the same process (very likely to be extracted as its own separate process in the near future).

**Storage**

The storage miner logic. This package also interfaces with the full node through a subset of the api. This code is used to implement the `lotus-storage-miner` process.

**Pond**

[Pond](https://docs.lotu.sh/join-devnet-gui) is a graphical testbed for lotus. It can be used to spin up nodes, connect them in a given topology, start them mining, and observe how they function over time.
