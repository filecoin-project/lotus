# Lotus

* [Components](#components)
* [Preliminaries](#preliminaries)
	* [Tipsets](#tipsets)
	* [Actors and Messages](#actors-and-messages)
* [Sync](#sync)
	* [Sync setup](#sync-setup)
	* [Fetching and Persisting Block Headers](#fetching-and-persisting-block-headers)
	* [Fetching and Validating Blocks](#fetching-and-validating-blocks)
	* [Setting the head](#setting-the-head)
	* [Keeping up with the chain](#keeping-up-with-the-chain)
	* [Network Message Flow and Bitswap Integration](#network-message-flow-and-bitswap-integration)
* [State](#state)
	* [Calculating a Tipset State](#calculating-a-tipset-state)
* [Virtual Machine](#virtual-machine)
	* [Applying a Message](#applying-a-message)
* [Building a Lotus node](#building-a-lotus-node)
	* [The Repository](#the-repository)
	* [Online](#online)

Lotus is an implementation of the [Filecoin Distributed Storage Network](https://filecoin.io/).
A Lotus node syncs blockchains that follow the
Filecoin protocol, validating the blocks and state transitions.
The specification for the Filecoin protocol can be found [here](https://filecoin-project.github.io/specs/).

For information on how to setup and operate a Lotus node,
please follow the instructions [here](en+getting-started).

## Components

At a high level, a Lotus node comprises the following components:

FIXME: No mention of block production here, cross-reference with schomatis's miner doc
- The Syncer, which manages the process of syncing the blockchain
- The State Manager, which can compute the state at any given point in the chain
- The Virtual Machine (VM), which executes messages
- The Repository, where all data is stored
- P2P stuff (FIXME missing libp2p listed under other PL dependencies)? allows hello, blocksync, retrieval, storage
- API / CLI (FIXME missing, in scratchpad)
- Other Filecoin dependencies (specs actors, proofs, storage, etc., FIXME missing)
- Is the Builder worth its own component?
- Other PL dependencies (IPFS, libp2p, IPLD? FIXME, missing)
- External libraries used by Lotus and other deps (FIXME, missing)

## Preliminaries

We discuss some key Filecoin concepts here, aiming to explain them by contrasting them with analogous concepts
in other well-known blockchains like Ethereum. We only provide brief descriptions here; elaboration
can be found in the [spec](https://filecoin-project.github.io/specs/).

### Tipsets

Unlike in Ethereum, a block can have multiple parents in Filecoin. We thus refer to the parent set of a block,
instead of a single parent.
A [tipset](https://filecoin-project.github.io/specs/#systems__filecoin_blockchain__struct__tipset)
is any set of blocks that share the same parent set.

There is no concept of "block difficulty" in Filecoin. Instead,
the weight of a tipset is simply the number of blocks in the chain that ends in that tipset. Note that a longer chain
can have less weight than a shorter chain with more blocks per tipset.

We also allow for "null" tipsets, which include zero blocks. This allows miners to "skip" a round, and build on top
of an imaginary empty tipset if they want to.

We call the heaviest tipset in a chain the "head" of the chain.

### Actors and Messages

An [Actor](https://filecoin-project.github.io/specs/#systems__filecoin_vm__actor)
 is analogous to a smart contract in Ethereum. Filecoin does not allow users to define their own
actors, but comes with several [builtin actors](https://github.com/filecoin-project/specs-actors),
which can be thought of as pre-compiled contracts.

A [Message](https://filecoin-project.github.io/specs/#systems__filecoin_vm__message)
is analogous to transactions in Ethereum.

## Sync

Sync refers to the process by which a Lotus node synchronizes to the heaviest chain being advertised by its peers.
At a high-level, Lotus syncs in a manner similar to most other blockchains; a Lotus node listens to the various
chains its peers claim to be at, picks the heaviest one, requests the blocks in the chosen chain,
and validates each block in that chain, running all state transitions along the way.

The majority of the sync functionality happens in the [`Syncer`](https://github.com/filecoin-project/lotus/blob/master/chain/sync.go),
internally managed by a [`SyncManager`](https://github.com/filecoin-project/lotus/blob/master/chain/sync_manager.go).

We now discuss the various stages of the sync process.

### Sync setup

When a Lotus node connects to a new peer, we exchange the head of our chain
with the new peer through [the `hello` protocol](https://github.com/filecoin-project/lotus/blob/master/node/hello/hello.go).
If the peer's head is heavier than ours, we try to sync to it. Note
that we do NOT update our chain head at this stage.

### Fetching and Persisting Block Headers

Note: The API refers to these stages as `StageHeaders` and `StagePersistHeaders`.

We proceed in the sync process by requesting block headers from the peer,
moving back from their head, until we reach a tipset that we have in common
(such a common tipset must exist, though it may simply be the genesis block).
The functionality can be found in `Syncer::collectHeaders()`.

If the common tipset is our head, we treat the sync as a "fast-forward", else we must
drop part of our chain to connect to the peer's head (referred to as "forking").

FIXME: This next para might be best replaced with a link to the validation doc
Some of the possible causes of failure in this stage include:

- The chain is linked to a block that we have previously marked as bad,
and stored in a [`BadBlockCache`](https://github.com/filecoin-project/lotus/blob/master/chain/badtscache.go).
- The beacon entries in a block are inconsistent (FIXME: more details about what is validated here wouldn't be bad).
- Switching to this new chain would involve a chain reorganization beyond the allowed threshold (SPECK-CHECK).

### Fetching and Validating Blocks

Note: The API refers to this stage as `StageMessages`.

Having acquired the headers and found a common tipset, we then move forward, requesting the full blocks, including the messages.

For each block, we  first confirm the syntactic validity of the block (SPECK-CHECK),
which includes the syntactic validity of messages included
in the block.
We then apply the messages, running all the state transitions, and compare the state root we calculate with the provided state root.


FIXME: This next para might be best replaced with a link to the validation doc
Some of the possible causes of failure in this stage include:

- a block is syntactically invalid (including potentially containing syntactically invalid messages)
- the computed state root after applying the block doesn't match the block's state root
- FIXME: Check what's covered by syntactic validity, and add anything important that isn't (like proof validity, future checks, etc.)

The core functionality can be found in `Syncer::ValidateTipset()`, with `Syncer::checkBlockMessages()` performing
syntactic validation of messages.

### Setting the head

Note: The API refers to this stage as `StageSyncComplete`.

If all validations pass we will now set that head as our heaviest tipset in
[`ChainStore`](https://github.com/filecoin-project/lotus/blob/master/chain/store/store.go).
We already have the full state, since we calculated
it during the sync process.

FIXME (aayush) I don't fully understand the next 2 paragraphs, but it seems important. Confirm and polish.
Relevant issue in IPFS: https://github.com/ipfs/ipfs-docs/issues/264

It is important to note at this point that similar to the IPFS architecture of addressing by content and not by location/address (FIXME: check and link to IPFS docs) the "actual" chain stored in the node repo is *relative* to which CID we look for. We always have stored a series of Filecoin blocks pointing to other blocks, each a potential chain in itself by following its parent's reference, and its parent's parent, and so on up to the genesis block. (FIXME: We need a diagram here, one of the Filecoin blog entries might have something similar to what we are describing here.) It only depends on *where* (location) do we start to look for. The *only* address/location reference we hold of the chain, a relative reference, is the `heaviest` pointer. This is reflected by the fact that we don't store it in the `Blockstore` by a fixed, *absolute*, CID that reflects its contents, as this will change each time we sync to a new head (FIXME: link to the immutability IPFS doc that I need to write).

FIXME: Create a further reading appendix, move this next para to it, along with other
extraneous content
This is one of the few items we store in `Datastore` by key, location, allowing its contents to change on every sync. This is reflected in the `(*ChainStore) writeHead()` function (called by `takeHeaviestTipSet()` above) where we reference the pointer by the explicit `chainHeadKey` address (the string `"head"`, not a hash embedded in a CID), and similarly in `(*ChainStore).Load()` when we start the node and create the `ChainStore`. Compare this to a Filecoin block or message which are immutable, stored in the `Blockstore` by CID, once created they never change.

### Keeping up with the chain

A Lotus node also listens for new blocks broadcast by its peers over the `gossipsub` channel (see Network Message Flow section).
If we have validated such a block's parent tipset, and adding it to our tipset at its height would lead to a heavier
head, then we validate and add this block. The validation described is identical to that invoked during the sync
process (indeed, it's the same codepath).

When processing incoming blocks, the node must fetch all referenced messages before validation can complete.
This message fetching process relies on messages typically being available in the local blockstore,
having been received earlier via the message pool from the `/fil/msgs` pubsub topic.

### Network Message Flow and Bitswap Integration

Lotus uses a pubsub-based architecture for real-time blockchain synchronization, combined with bitswap for reliable block and message retrieval. Understanding this flow is crucial for comprehending Lotus' networking performance characteristics.

#### PubSub Topics

Lotus connects to two primary pubsub topics:

- **`/fil/blocks/{network}`**: Used for broadcasting and receiving new block headers
- **`/fil/msgs/{network}`**: Used for broadcasting and receiving individual messages

These topics are generated dynamically based on the network name (e.g., mainnet, calibnet) to prevent cross-network message propagation.

#### Message Flow Architecture

The typical flow for a well-connected node follows this pattern:

**1. Message Propagation (`/fil/msgs`)**

1. **Message Creation**: When transactions are submitted, they're added to the local message pool
2. **Message Publishing**: `MessagePool.Add()` publishes new messages to `/fil/msgs` via `PubSubPublish()`
3. **Network Reception**: Other nodes receive messages through `HandleIncomingMessages()`
4. **Validation**: `MessageValidator.Validate()` performs signature verification and basic validation
5. **Storage**: Valid messages are stored via `MessagePool.Add()` → `ChainStore.PutMessage()` → `chainBlockstore`

**2. Block Propagation (`/fil/blocks`)**

1. **Block Creation**: Miners create blocks containing message CIDs from their message pool
2. **Block Publishing**: New blocks are published to `/fil/blocks` via `SyncSubmitBlock()`
3. **Network Reception**: Nodes receive block headers through `HandleIncomingBlocks()`
4. **Message Fetching**: For each block, the node must fetch the actual message content using `FetchMessagesByCids()`

**3. The Critical Design: Local vs Network Fetching**

**The Happy Path** (No Bitswap Needed):
- Messages arrive via `/fil/msgs` before their containing blocks
- Messages are stored in the local `chainBlockstore`
- When blocks arrive, `FetchMessagesByCids()` finds messages locally
- Result: No network requests required during block processing

**When Bitswap is Used**:
- Node missed messages from `/fil/msgs` (network partition, late join, etc.)
- Block references message CIDs not in local storage
- `FetchMessagesByCids()` triggers bitswap session to fetch from network
- Retrieved messages are cached locally for future use

#### Two Bitswap Mechanisms

Lotus employs two distinct bitswap mechanisms for different scenarios:

**1. Session-Based Bitswap (Always Active)**

**Purpose**: Real-time message fetching during block processing
**Location**: `HandleIncomingBlocks()` in `chain/sub/incoming.go`

```go
// Creates a new bitswap session for each incoming block
ses := bserv.NewSession(ctx, bs)

// Efficiently fetches missing messages
bmsgs, err := FetchMessagesByCids(ctx, ses, blk.BlsMessages)
smsgs, err := FetchSignedMessagesByCids(ctx, ses, blk.SecpkMessages)
```

**Characteristics**:
- Always enabled (no configuration required)
- Uses `ChainBlockService` built from `ChainBitswap` + `ExposedBlockstore`
- Protocol: `/chain/ipfs/bitswap/1.0.0`
- Short-lived sessions per block processing
- Optimized for batched message fetching
- Temporary caching (2 block times)

**2. FallbackStore Pattern (Optional)**

**Purpose**: Transparent safety net for any blockstore operation
**Configuration**: Enabled with `LOTUS_ENABLE_CHAINSTORE_FALLBACK=1`

**Characteristics**:
- Wraps main chain and state blockstores
- Automatically fetches any missing blocks on `Get()` operations
- Uses standard bitswap protocol
- Permanent local storage of retrieved blocks
- 120-second timeout protection
- Applies to all blockstore access, not just message fetching

#### Performance Implications

**When Bitswap is Rarely Needed**
- Healthy pubsub connectivity to `/fil/msgs`
- Good network topology with diverse peers
- Up-to-date node with recent sync
- **Result**: Most message fetching happens from local storage

**When Bitswap Becomes Critical**
- Network partitions or connectivity issues
- New nodes syncing from scratch
- Message propagation delays (blocks arrive before messages)
- Miners including messages not widely propagated
- **Result**: Frequent network fetches, higher latency

**Network Protocol Details**

**ChainBitswap Configuration**:
- Dedicated `/chain` protocol prefix to avoid interference with IPFS bitswap
- Tiered blockstore: `ExposedBlockstore` + temporary cache
- Automatic peer discovery through DHT routing
- Session-level optimizations for related message fetching

**Gossipsub Integration**:
- Peer scoring prevents spam and rewards good behavior
- Message deduplication using Blake2b hashing
- Topic-specific validation before propagation
- Bootstrap nodes vs regular nodes have different subscription timing

#### Monitoring and Debugging

Key indicators of bitswap usage:
- `FetchMessagesByCids` timing logs (warns if >3 seconds)
- Bitswap block retrieval metrics
- Peer connection and scoring status
- Message pool health and propagation delays

This architecture ensures Lotus can efficiently process blocks in the common case (local message availability) while providing robust fallback mechanisms for network reliability.

## State

In Filecoin, the chain state at any given point is  a collection of data stored under a root CID
encapsulated in the [`StateTree`](https://github.com/filecoin-project/lotus/blob/master/chain/state/statetree.go),
and accessed through the
[`StateManager`](https://github.com/filecoin-project/lotus/blob/master/chain/stmgr/stmgr.go).
The state at the chain's head is thus easily tracked and updated in a state root CID.
(FIXME: Talk about CIDs somewhere,  we might want to explain some of the modify/flush/update-root mechanism here.)

### Calculating a Tipset State

Recall that a tipset is a set of blocks that have identical parents (that is, that are built on top of the same tipset).
The genesis tipset comprises the genesis block(s), and has some state corresponding to it.

The methods `TipSetState()` and `computeTipSetState()` in
[`StateManager`](https://github.com/filecoin-project/lotus/blob/master/chain/stmgr/stmgr.go)
 are responsible for computing
the state that results from applying a tipset. This involves applying all the messages included
in the tipset, and performing implicit operations like awarding block rewards.

Any valid block built on top of a tipset `ts` should have its Parent State Root equal to the result of
calculating the tipset state of `ts`. Note that this means that all blocks in a tipset must have the same Parent
State Root (which is to be expected, since they have the same parent tipset)

#### Preparing to apply a tipset

When `StateManager::computeTipsetState()` is called with a tipset, `ts`,
it retrieves the parent state root of the blocks in `ts`. It also creates a list of `BlockMessages`, which wraps the BLS
and SecP messages in a block along with the miner that produced the block.

Control then flows to `StateManager::ApplyBlocks()`, which builds a VM to apply the messages given to it. The VM
is initialized with the parent state root of the blocks in `ts`. We apply the blocks in `ts` in order (see FIXME for
ordering of blocks in a tipset).

#### Applying a block

For each block, we prepare to apply the ordered messages (first BLS, then SecP). Before applying a message, we check if
we have already applied a message with that CID within the scope of this method. If so, we simply skip that message;
this is how duplicate messages included in the same tipset are skipped (with only the miner of the "first" block to
include the message getting the reward). For the actual process of message application, see FIXME (need an
internal link here), for now we
simply assume that the outcome of the VM applying a message is either an error, or a
[`MessageReceipt`](https://github.com/filecoin-project/lotus/blob/master/chain/types/message_receipt.go)
 and some
other information.

We treat an error from the VM as a showstopper; there is no recovery, and no meaningful state can be computed for `ts`.
Given a successful receipt, we add the rewards and penalties to what the miner has earned so far. Once all the messages
included in a block have been applied (or skipped if they're a duplicate), we use an implicit message to call
the Reward Actor. This awards the miner their reward for having won a block, and also awards / penalizes them based
on the message rewards and penalties we tracked.

We then proceed to apply the next block in `ts`, using the same VM. This means that the state changes that result
from applying a message are visible when applying all subsequent messages, even if they are included in a different block.

#### Finishing up

Having applied all the blocks, we send one more implicit message, to the Cron Actor, which handles operations that
must be performed at the end of every epoch (see FIXME for more). The resulting state after calling the Cron Actor
is the computed state of the tipset.

## Virtual Machine

The Virtual Machine (VM) is responsible for executing messages.
The [Lotus Virtual Machine](https://github.com/filecoin-project/lotus/blob/master/chain/vm/vm.go)
invokes the appropriate methods in the builtin actors, and provides
a [`Runtime`](https://github.com/filecoin-project/specs-actors/blob/master/actors/runtime/runtime.go)
interface to the [builtin actors](https://github.com/filecoin-project/specs-actors)
that exposes their state, allows them to take certain actions, and meters
their gas usage. The VM also performs balance transfers, creates new account actors as needed, and tracks the gas reward,
penalty, return value, and exit code.

### Applying a Message

The primary entrypoint of the VM is the `ApplyMessage()` method. This method should not return an error
unless something goes unrecoverably wrong.

The first thing this method does is assess if the message provided meets any of the penalty criteria.
If so, a penalty is issued, and the method returns. Next, the entire gas cost of the message is transferred to
a temporary gas holder account. It is from this gas holder that gas will be deducted; if it runs out of gas, the message
fails. Any unused gas in this holder will be refunded to the message's sender at the end of message execution.

The VM then increments the sender's nonce, takes a snapshot of the state, and invokes `VM::send()`.

The `send()` method creates a [`Runtime`](https://github.com/filecoin-project/lotus/blob/master/chain/vm/runtime.go)
 for the subsequent message execution.
It then transfers the message's value to the recipient, creating a new account actor if needed.

#### Method Invocation

We use reflection to translate a Filecoin message for the VM to an actual Go function, relying on the VM's
[`invoker`](https://github.com/filecoin-project/lotus/blob/master/chain/vm/invoker.go) structure.
Each actor has its own set of codes defined in `specs-actors/actors/builtin/methods.go`.
The `invoker` structure maps the builtin actors' CIDs
 to a list of `invokeFunc` (one per exported method), which each take the `Runtime` (for state manipulation)
 and the serialized input parameters.

FIXME (aayush) Polish this next para.

The basic layout (without reflection details) of `(*invoker).transform()` is as follows. From each actor registered in `NewInvoker()` we take its `Exports()` methods converting them to `invokeFunc`s. The actual method is wrapped in another function that takes care of decoding the serialized parameters and the runtime, this function is passed to `shimCall()` that will encapsulate the actors code being run inside a `defer` function to `recover()` from panics (we fail in the actors code with panics to unwrap the stack). The return values will then be (CBOR) marshaled and returned to the VM.

#### Returning from the VM

Once method invocation is complete (including any subcalls), we return to `ApplyMessage()`, which receives
the serialized response and the [`ActorError`](https://github.com/filecoin-project/lotus/blob/master/chain/actors/aerrors/error.go).
The sender will be charged the appropriate amount of gas for the returned response, which gets put into the
[`MessageReceipt`](https://github.com/filecoin-project/lotus/blob/master/chain/types/message_receipt.go).

The method then refunds any unused gas to the sender, sets up the gas reward for the miner, and
wraps all of this into an `ApplyRet`, which is returned.

## Building a Lotus node

When we launch a Lotus node with the command `./lotus daemon`
(see [here](https://github.com/filecoin-project/lotus/blob/master/cli/lotus/daemon.go) for more),
the node is created through [dependency injection](https://godoc.org/go.uber.org/fx).
This relies on reflection, which makes some of the references hard to follow.
The node sets up all of the subsystems it needs to run, such as the repository, the network connections, the chain sync
service, etc.
This setup is orchestrated through calls to the `node.Override` function.
The structure of each call indicates the type of component it will set up
(many defined in [`node/modules/dtypes/`](https://github.com/filecoin-project/lotus/tree/master/node/modules/dtypes)),
and the function that will provide it.
The dependency is implicit in the argument of the provider function.

As an example, consider the `modules.ChainStore()` function that provides the
[`ChainStore`](https://github.com/filecoin-project/lotus/blob/master/chain/store/store.go) structure.
It takes as one of its parameters the [`ChainBlockstore`](https://github.com/filecoin-project/lotus/blob/master/node/modules/dtypes/storage.go)
type, which becomes one of its dependencies.
For the node to be built successfully the `ChainBlockstore` will need to be provided before `ChainStore`, a requirement
that is made explicit in another `Override()` call that sets the provider of that type as the `ChainBlockstore()` function.

### The Repository

The repo is the directory where all of a node's information is stored. The node is entirely defined by its repo, which
makes it easy to port to another location. This one-to-one relationship means we can speak
of the node as the repo it is associated with, instead of the daemon process that runs from that repo.

Only one daemon can run with an associated repo at a time.
A process signals that it is running a node associated with a particular repo, by creating and acquiring
a `repo.lock`.

```sh
lsof ~/.lotus/repo.lock
# COMMAND   PID
# lotus   52356
```
Trying to launch a second daemon hooked to the same repo leads to a `repo is already locked (lotus daemon already running)`
error.

The `node.Repo()` function (`node/builder.go`) contains most of the dependencies (specified as `Override()` calls)
needed to properly set up the node's repo. We list the most salient ones here.

#### Datastore

`Datastore` and `ChainBlockstore`: Data related to the node state is saved in the repo's `Datastore`,
an IPFS interface defined [here](https://github.com/ipfs/go-datastore/blob/master/datastore.go).
Lotus creates this interface from a [Badger DB](https://github.com/dgraph-io/badger) in
 [`FsRepo`](https://github.com/filecoin-project/lotus/blob/master/node/repo/fsrepo.go).
Every piece of data is fundamentally a key-value pair in the `datastore` directory of the repo.
There are several abstractions laid on top of it that appear through the code depending on *how* we access it,
but it is important to remember that we're always accessing it from the same place.

FIXME: Maybe mention the `Batching` interface as the developer will stumble upon it before reaching the `Datastore` one.

#### Blocks

FIXME: IPFS blocks vs Filecoin blocks ideally happens before this / here

The [`Blockstore` interface](`github.com/filecoin-project/lotus/blockstore/blockstore.go`) structures the key-value pair
into the CID format for the key and the [`Block` interface](`github.com/ipfs/go-block-format/blocks.go`) for the value.
The `Block` value is just a raw string of bytes addressed by its hash, which is included in the CID key.

`ChainBlockstore` creates a `Blockstore` in the repo under the `/blocks` namespace.
Every key stored there will have the `blocks` prefix so that it does not collide with other stores that use the same repo.

FIXME: Link to IPFS documentation about DAG, CID, and related, especially we need a diagram that shows how do we wrap each datastore inside the next layer (datastore, batching, block store, gc, etc).

#### Metadata

`modules.Datastore()` creates a `dtypes.MetadataDS`, which is an alias for the basic `Datastore` interface.
Metadata is stored here under the `/metadata` prefix.
(FIXME: Explain *what* is metadata in contrast with the block store, namely we store the pointer to the heaviest chain, we might just link to that unwritten section here later.)

FIXME: Explain the key store related calls (maybe remove, per Schomatis)

#### LockedRepo

`LockedRepo()`: This method doesn't create or initialize any new structures, but rather registers an
 `OnStop` [hook](https://godoc.org/go.uber.org/fx/internal/lifecycle#Hook)
 that will close the locked repository associated with it on shutdown.


#### Repo types / Node types

FIXME: This section needs to be clarified / corrected...I don't fully understand the config differences (what do they have in common, if anything?)

At the end of the `Repo()` function we see two mutually exclusive configuration calls based on the `RepoType` (`node/repo/fsrepo.go`).
```Go
			ApplyIf(isType(repo.FullNode), ConfigFullNode(c)),
			ApplyIf(isType(repo.StorageMiner), ConfigStorageMiner(c)),
```
As we said, the repo fully identifies the node so a repo type is also a *node* type, in this case a full node or a miner. (FIXME: What is the difference between the two, does *full* imply miner?) In this case the `daemon` command will create a `FullNode`, this is specified in the command logic itself in `main.DaemonCmd()`, the `FsRepo` created (and passed to `node.Repo()`) will be initiated with that type (see `(*FsRepo).Init(t RepoType)`).

### Online

FIXME: Much of this might need to be subsumed into the p2p section

The `node.Online()` configuration function (`node/builder.go`) initializes components that involve connecting to,
or interacting with, the Filecoin network. These connections are managed through the libp2p stack (FIXME link to this section when it exists).
We discuss some of the components found in the full node type (that is, included in the `ApplyIf(isType(repo.FullNode),` call).

#### Chainstore

`modules.ChainStore()` creates the [`store.ChainStore`](https://github.com/filecoin-project/lotus/blob/master/chain/store/store.go))
that wraps the stores
 previously instantiated in `Repo()`. It is the main point of entry for the node to all chain-related data
 (FIXME: this is incorrect, we sometimes access its underlying block store directly, and probably shouldn't).
 It also holds the crucial `heaviest` pointer, which indicates the current head of the chain.

#### ChainExchange and ChainBlockservice

`ChainExchange()` provides a custom Filecoin protocol (`/fil/chain/xchg/0.0.1`) for requesting block headers during chain synchronization. This is distinct from bitswap and optimized for bulk header retrieval.

`ChainBlockservice()` creates the critical bitswap-enabled service used for real-time message fetching. It combines:
- `ExposedBlockstore` (main local storage interface)
- `ChainBitswap` (network exchange with `/chain` protocol prefix)
- Tiered caching for performance optimization

This service is primarily used by `HandleIncomingBlocks()` to fetch message content when processing new blocks received via pubsub. See the [Network Message Flow](#network-message-flow-and-bitswap-integration) section for detailed explanation of when bitswap network requests occur versus when messages are available locally.

##### Monitoring Bitswap Usage

To monitor how often Lotus needs to use bitswap for fetching messages (as opposed to having them available locally from pubsub), you can enable message fetch instrumentation:

```bash
export LOTUS_ENABLE_MESSAGE_FETCH_INSTRUMENTATION=1
```

This enables metrics that track:
- `lotus_message_fetch_requested`: Total messages requested
- `lotus_message_fetch_local`: Messages found in local blockstore (from pubsub)
- `lotus_message_fetch_network`: Messages that required bitswap network fetch

#### Incoming handlers

`HandleIncomingBlocks()` and `HandleIncomingMessages()` start the services in charge of processing new Filecoin blocks
and messages from the network via the `/fil/blocks` and `/fil/msgs` pubsub topics respectively. These handlers implement the core real-time synchronization logic described in the [Network Message Flow](#network-message-flow-and-bitswap-integration) section.

#### Hello

`RunHello()`: starts the services to both send (`(*Service).SayHello()`) and receive (`(*Service).HandleStream()`, `node/hello/hello.go`)
`hello` messages. When nodes establish a new connection with each other, they exchange these messages
to share chain-related information (namely their genesis block and their heaviest tipset).

#### Syncer

`NewSyncer()` creates the `Syncer` structure and starts the services related to the chain sync process (FIXME link).

#### Ordering the dependencies

We can establish the dependency relations by looking at the parameters that each function needs and by understanding
the architecture of the node and how the different components relate to each other (the chief purpose of this document).

As an example, the sync mechanism depends on the node being able to exchange different IPFS blocks with the network,
so as to be able to request the "missing pieces" needed to construct the chain. This dependency is reflected by `NewSyncer()`
having a `blocksync.BlockSync` parameter, which in turn depends on `ChainBlockservice()` and `ChainExchange()`.
The chain exchange service further depends on the chain store to save and retrieve chain data, which is reflected
in `ChainExchange()` having `ChainGCBlockstore` as a parameter (which is just a wrapper around `ChainBlockstore` capable
 of garbage collection).

This block store is the same store underlying the chain store, which is an indirect dependency of `NewSyncer()` (through the `StateManager`).
(FIXME: This last line is flaky, we need to resolve the hierarchy better, we sometimes refer to the chain store and sometimes to its underlying block store. We need a diagram to visualize all the different components just mentioned otherwise it is too hard to follow. We probably even need to skip some of the connections mentioned.)
