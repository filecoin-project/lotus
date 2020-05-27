#Lotus

Lotus is an implementation of the Filecoin Distributed Storage Network. A Lotus node syncs blockchains that follow the 
Filecoin protocol, validating the blocks and state transitions.

For information on how to setup and operate a Lotus node, 
please follow the instructions [here](https://lotu.sh/en+getting-started).

# Components

At a high level, a Lotus node comprises the following components:

- VM
- Sync
- API / CLI
- State Manager (used by sync)
- Database (badger)
- P2P stuff (libp2p listed under other PL dependencies)? allows hello, blocksync, retrieval, storage
- Other Filecoin dependencies (specs actors, proofs, storage, etc.)
- Other PL dependencies (IPFS, libp2p, IPLD?)
- External libraries used by Lotus and other deps?

# Sync

Sync broadly refers to the process by which a Lotus node synchronizes to the chain being advertised by its peers.
At a high-level, Lotus syncs in a manner similar to most other blockchains; a Lotus node listens to the various
chains its peers claim to be at, picks the heaviest one, requests the blocks in the chosen chain,
and validates each block in that chain, running all state transitions along the way.

The majority of the sync functionality happens in the `Sync` (LINK) structure, internally managed by a `SyncManager` (LINK).
In order to understand the Lotus sync process, it is important to be familiar with Filecoin's concepts of tipsets (SPECK-CHECK),
and how we evaluate the weight of a tipset. We call the heaviest tipset in a chain the "head" of the chain.

Sync occurs in the following stages 

## Sync setup

When a Lotus node connects to a new peer, we exchange the head of our chain with the new peer through the `hello` protocol.
See FIXME for more about the `hello` protocol. If our peer's head is heavier than ours, we try to sync to it. Note
that we do NOT update our chain head at this stage.

## Fetching Headers

Note: The API refers to this stage as `StageHeaders`.

We proceed in the sync process by requesting block headers from the peer, 
moving back from their head, until we reach a tipset that we have in common
(such a common tipset must exist, thought it may simply be the genesis block).

Some of the possible causes of failure in this stage include:

- The chain is linked to a block that we have previously marked as bad, and stored in a `BadBlockCache`.
- The beacon entries in a block are inconsistent (WHYMAGIK: more details about what is validated here wouldn't be bad).
- Switching to this new chain would invole a chain reorganization beyond the allowed threshold (SPECK-CHECK).

This functionality is in `Syncer::collectHeaders()` (LINK).

## Persisting Headers

Note: The API refers to this stage as `StagePersistHeaders`.

The next step is simply to store these block headers.

## Fetching and Validating Blocks

Note: The API refers to this stage as `StageMessages`.

Having acquired the headers and found a common tipset, we then move forward, requesting the full blocks, including the messages.

For each block, we  first confirm the syntactic validity of the block (SPECK-CHECK), 
which includes the syntactic validity of messages included
in the block.
We then apply the messages, running all the state transitions, and compare the state root we calculate with the provided state root.

Some of the possible causes of failure in this stage include:

- a block is syntactically invalid (including potentially containing syntactically invalid messages)
- the computed state root after applying the block doesn't match the block's state root
- FIXME: Check what's covered by syntactic validity, and add anything important that isn't (like proof validity, future checks, etc.)

The core functionality can be found in `Syncer::ValidateTipset()`, with `Syncer::checkBlockMessages()` performing
syntactic validation of messages (LINK).

## Setting the head

Note: The API refers to this stage as `StageSyncComplete`.

If all validations pass we will now set that head as our heaviest tipset in `ChainStore`.
We already have the full state, since we calculated
it during the sync process.
 
FIXME (aayush) I don't fuilly understand the next 2 paragraphs, but it seems important. Confirm and polish.

It is important to note at this point that similar to the IPFS architecture of addressing by content and not by location/address (FIXME: check and link to IPFS docs) the "actual" chain stored in the node repo is *relative* to which CID we look for. We always have stored a series of Filecoin blocks pointing to other blocks, each a potential chain in itself by following its parent's reference, and its parent's parent, and so on up to the genesis block. (FIXME: We need a diagram here, one of the Filecoin blog entries might have something similar to what we are describing here.) It only depends on *where* (location) do we start to look for. The *only* address/location reference we hold of the chain, a relative reference, is the `heaviest` pointer. This is reflected by the fact that we don't store it in the `Blockstore` by a fixed, *absolute*, CID that reflects its contents, as this will change each time we sync to a new head (FIXME: link to the immutability IPFS doc that I need to write).

This is one of the few items we store in `Datastore` by key, location, allowing its contents to change on every sync. This is reflected in the `(*ChainStore) writeHead()` function (called by `takeHeaviestTipSet()` above) where we reference the pointer by the explicit `chainHeadKey` address (the string `"head"`, not a hash embedded in a CID), and similarly in `(*ChainStore).Load()` when we start the node and create the `ChainStore`. Compare this to a Filecoin block or message which are immutable, stored in the `Blockstore` by CID, once created they never change.

## Keeping up with the chain 

A Lotus node also listens for new blocks broadcast by its peers over the `gossipsub` channel (see FIXME for more).
If we have validated such a block's parent tipset, and adding it to our tipset at its height would lead to a heavier
head, then we validate and add this block. The validation described is identical to that invoked during the sync
process (indeed, it's the same codepath).

## Some more sync notes (FIXME)

Alternatively to this path, we also receive new blocks through the `blocks` `gossipsub` topic which will also be checked to be a potential new head to our current chain. (FIXME: We either explain gossipsub in libp2p subsection above or discuss it in its own section later, either way refer to that). For simplicity in this analysis we follow the `hello` code path.

It is important to note that the sync service runs throughout the entire lifetime of the node, it cannot be stopped, as we are always in a "perpetual syncing state" since we can never be sure that we have *the* heaviest tipset in the entire network, there is always the possibility of a new incoming tipset to be "better" (heavier) than our current head. Even when the `lotus sync wait` command reports we are synced, it just means the timestamp of our current head is close to the current time (see `SyncWait()` in `cli/sync.go`), that we are "synced up to the present", but it does not guarantee having the heaviest tipset possible (a decentralized model does not have such a concept to begin with because there is no central authority to keep track of that, we just depend on the weight of a block based on an agreed upon set of rules from the protocol).

We already saw the `modules.RunHello()` dependency setup in the `node.Online()` configuration function, it starts the `(*Service).HandleStream()` service to process incoming `hello` messages (`HelloMessage`, `node/hello/hello.go`) and extracts the `HeaviestTipSet` from it, calling `(*Syncer).InformNewHead()` to notify the syncer process about a new potential head of the chain, that is, a new tipset with more weight than our current heaviest one (without still knowing if it is valid or not). A few notes of interest before moving on, the tipset in its entirety is not actually sent in the message but instead its `TipSetKey` (string of CIDs of the blocks that conform that tipset). The `hello` service fetches the actual tipset by using the `Syncer`'s `BlockSync` service `(*BlockSync).GetFullTipSet()`. What does this means in terms of dependencies? That `hello.Service` needs access to the `Syncer` which in turn uses its `BlockSync`, this is reflected in the dependency construction: the `RunHello()` provider has a `hello.Service` parameter, this in turn has the `Syncer` as a parameter (`NewHelloService`), and the `Syncer` (as we have already seen) needs a `BlockSync` (`NewSyncer()`).

Once the `Syncer` is informed of a new potential head it adds it to the queue to process it. Without going into the full details of `Syncer` internals, we should mention the it has a `SyncManager` structure in charge of coordinating the multiple syncing process (as we might be checking more than one potential heaviest tipset at a time, this is specially true during the first sync of the node where we start from the initial genesis block). As said, the `Syncer` runs throughout the life cycle of the node and this is reflected on the `OnStart`/`OnStop` application hooks in `NewSyncer` that start and stop the sync process in coordination with the node application. When the `SyncManager` starts it runs many sync worker routines and a central scheduler to coordinate them:

```Go
	go sm.syncScheduler()
	for i := 0; i < syncWorkerCount; i++ {
		go sm.syncWorker(i)
	}
```

The `syncWorkerCount` number (normally 3) is reflected in the output of the `lotus sync status` command that will show a separate status (`SyncerState`, `chain/syncstate.go`) for each of the sync workers. When a new tipset is informed, depending on the current state of the sync, the scheduler will assign a free worker to process it or will queue it (see `scheduleIncoming` in `chain/sync_manager.go`). (FIXME: There is much more to it but that should be in the code's comments, that currently doesn't have, but eventually we should point there.)

Each `syncWorker` once it receives a new head calls the `doSync` handler to sync to it, usually corresponding to the `(*Syncer).Sync()` function (set up in the `NewSyncer()` configuration.)

# CLI, API

Explain how do we communicate with the node, both in terms of the CLI and the programmatic way (to create our own tools).

## Client/server architecture

In terms of the Filecoin network the node is a peer on a distributed hierarchy, but in terms of how we interact with the node we have client/server architecture.

The node itself was initiated with the `daemon` command, it already started syncing to the chain by default. Along with that service it also started a [JSON-RPC](https://en.wikipedia.org/wiki/JSON-RPC) server to allow a client to interact with it. (FIXME: Check if this client is local or can be remote, link to external documentation of connection API.)

We can connect to this server through the Lotus CLI. Virtually any other command other than `daemon` will run a client that will connect (by default) to the address specified in the `api` file in the repo associated with the node (by default in `~/.lotus`), e.g.,

```sh
cat  ~/.lotus/api && echo
# /ip4/127.0.0.1/tcp/1234/http

# With `lotus daemon` running in another terminal.
nc -v -z 127.0.0.1 1234 

# Start daemon and turn off the logs to not clutter the command line.
bash -c "lotus daemon &" &&
  lotus wait-api &&
  lotus log set-level error # Or a env.var in the daemon command.

nc -v -z 127.0.0.1 1234
# Connection to 127.0.0.1 1234 port [tcp/*] succeeded!

killall lotus
# FIXME: We need a lotus stop command:
#  https://github.com/filecoin-project/lotus/issues/1827
```

FIXME: Link to more in-depth documentation of the CLI architecture, maybe some IPFS documentation (since they share some common logic).

## Node API

The JSON-RPC server exposes the node API, the `FullNode` interface (defined in `api/api_full.go`). When we issue a command like `lotus sync status` to query the progress of the node sync we don't access the node's internals, those are decoupled in a separate daemon process, we call the `SyncState` function (of the `FullNode` API interface) through the RPC client started by our own command (see `NewFullNodeRPC` in `api/client/client.go` for more details).

FIXME: Link to (and create) documentation about API fulfillment.

Because we rely heavily on reflection for this part of the code the call chain is not easily visible by just following the references through the symbolic analysis of the IDE. If we start by the `lotus sync` command definition (in `cli/sync.go`), we eventually end up in the method interface `SyncState`, and when we look for its implementation we will find two functions:

* `(*SyncAPI).SyncState()` (in `node/impl/full/sync.go`): this is the actual implementation of the API function that shows what the node (here acting as the RPC server) will execute when it receives the RPC request issued from the CLI acting as the client.

* `(*FullNodeStruct).SyncState()`: this is an "empty placeholder" structure that will get later connected to the JSON-RPC client logic (see `NewMergeClient` in `lib/jsonrpc/client.go`, which is called by `NewFullNodeRPC`). (FIXME: check if this is accurate). The CLI (JSON-RPC client) will actually execute this function which will connect to the server and send the corresponding JSON request that will trigger the call of `(*SyncAPI).SyncState()` with the node implementation.

This means that when we are tracking the logic of a CLI command we will eventually find this bifurcation and need to study the code of the server-side implementation in `node/impl/full` (mostly in the `common/` and `full/` directories). If we understand this architecture going directly to that part of the code abstracts away the JSON-RPC client/server logic and we can think that the CLI is actually running the node's logic.

FIXME: Explain that "*the* node" is actually an API structure like `impl.FullNodeAPI` with the different API subcomponents like `full.SyncAPI`. We won't see a *single* node structure, each API (full node, minder, etc) will gather the necessary subcomponents it needs to service its calls.

# Node build

When we start the daemon command (`cmd/lotus/daemon.go`) the node is created through [dependency injection](https://godoc.org/go.uber.org/fx) (again relying on reflection that might make some of the references hard to follow). The node sets up all of the subsystems it needs to run, e.g., the repository, the network connections, services like chain sync, etc. Each of those is ordered through successive calls to the `node.Override` function (see `fx`package linked above for more details). The structure of the call indicates first the type of component it will set up (many defined in `node/modules/dtypes/`) and the function that will provide it. The dependency is implicit in the argument of the provider function, for example, the `modules.ChainStore()` function that provides the `ChainStore` structure (as indicated by one of the `Override()` calls) takes as one of its parameters the `ChainBlockstore` type, which will be then one of its dependencies. For the node to be built successfully the `ChainBlockstore` will need to be provided before `ChainStore`, and this is indeed explicitly stated in another `Override()` call that sets the provider of that type as the `ChainBlockstore()` function. (Most of these references can be followed through the IDE.)

## Repo

The repo is the directory where all the node information is stored. The node is entirely defined by it making it easy to port to another location. This one-to-one relation means sometimes we will speak (or define structures in the code) of the node as just the repo it is associated to, instead of the daemon process that runs from that repo, both associations are correct and just depend on the context.

Only one daemon can run per node/repo at a time, if we try to start a second one (e.g., maybe because we have one running in the background without noticing it) we will get a `repo is already locked` error. This is the way for a process to signal it is running a node associated with that repo, a `repo.lock` will be created in it and seized by that process:

```sh
lsof ~/.lotus/repo.lock
# COMMAND   PID
# lotus   52356
```

FIXME: Replace the `repo is already locked` error with the actual explanation so we don't need to also translate error to reality here as well: https://github.com/filecoin-project/lotus/issues/1829.

The `node.Repo()` function (`node/builder.go`) contains most of the dependencies (specified as `Override()` calls) needed to properly set up the node's repo. We list the most salient ones here (defined in `node/modules/storage.go`).

`LockedRepo()`: a common pattern in the DI model is to not only provide actual structures the node will incorporate in itself but list any type of dependencies in terms of "things that need to be done" before the node starts, this is the case of this function that doesn't create and initialize any new structure but rather registers a `OnStop` [hook](https://godoc.org/go.uber.org/fx/internal/lifecycle#Hook) that will close the locked repository associated to it:

```Go
type LockedRepo interface {
	// Close closes repo and removes lock.
	Close() error
```

`Datastore` and `ChainBlockstore`: all data related to the node state is saved in the repo's `Datastore`, an IPFS interface defined in `github.com/ipfs/go-datastore/datastore.go`. (See `(*fsLockedRepo).Datastore()` in `node/repo/fsrepo.go` for how Lotus creates it from a [Badger DB](https://github.com/dgraph-io/badger).) At the core every piece of data is a key-value pair in the `datastore` directory of the repo, but there are several abstractions we lay on top of it that appear through the code depending on *how* do we access it, but it is important to remember that the *where* is always the same.

FIXME: Maybe mention the `Batching` interface as the developer will stumble upon it before reaching the `Datastore` one.

The first abstraction is the `Blockstore` interface (`github.com/ipfs/go-ipfs-blockstore/blockstore.go`) which structures the key-value pair into the CID format for the key and the `Block` interface (`github.com/ipfs/go-block-format/blocks.go`) for the value, which is just a raw string of bytes addressed by its hash (which is included in the CID key/identifier). `ChainBlockstore` will create a `Blockstore` in the repo under the `/blocks` namespace (basically every key stored there will have that prefix so it does not collide with other stores that use the same underlying repo).

FIXME: Link to IPFS documentation about DAG, CID, and related, especially we need a diagram that shows how do we wrap each datastore inside the next layer (datastore, batching, block store, gc, etc).

Similarly, `modules.Datastore()` creates a `dtypes.MetadataDS` (alias for the basic `Datastore` interface) to store metadata under the `/metadata`. (FIXME: Explain *what* is metadata in contrast with the block store, namely we store the pointer to the heaviest chain, we might just link to that unwritten section here later.)

FIXME: Explain the key store related calls.

At the end of the `Repo()` function we see two mutually exclusive configuration calls based on the `RepoType` (`node/repo/fsrepo.go`).
```Go
			ApplyIf(isType(repo.FullNode), ConfigFullNode(c)),
			ApplyIf(isType(repo.StorageMiner), ConfigStorageMiner(c)),
```
As we said, the repo fully identifies the node so a repo type is also a *node* type, in this case a full node or a storage miner. (FIXME: What is the difference between the two, does *full* imply miner?) In this case the `daemon` command will create a `FullNode`, this is specified in the command logic itself in `main.DaemonCmd()`, the `FsRepo` created (and passed to `node.Repo()`) will be initiated with that type (see `(*FsRepo).Init(t RepoType)`).

### Filecoin blocks vs IPFS blocks

The term *block* has different meanings depending on the context, many times both meanings coexist at once in the code and it is important to distinguish them. (FIXME: link to IPFS blocks and related doc throughout this explanation). In terms of the lower IPFS layer, in charge of storing and retrieving data, both present at the repo or accessible through the network (e.g., through the BitSwap protocol discussed later), a block is a string of raw bytes identified by its hash, embedded and fully qualified in a CID identifier. IPFS blocks are the "building blocks" of almost any other piece of (chain) data described in the Filecoin protocol.

In contrast, in the higher Filecoin (application) layer, a block is roughly (FIXME: link to spec definition, if we have any) a set of zero or more messages grouped together by a single miner which is itself grouped with other blocks (from other miners) in the same round to form a tipset. The Filecoin blockchain is a series of "chained" tipsets, each referencing its parent by its header's *CID*, that is, its header as seen as a single IPFS block, this is where both layers interact.

Using now the full Go package qualifiers to avoid any ambiguity, the Filecoin block, `github.com/filecoin-project/lotus/chain/types.FullBlock`, is defined as,

```Go
package types

import "github.com/ipfs/go-cid"

type FullBlock struct {
	Header        *BlockHeader
	BlsMessages   []*Message
	SecpkMessages []*SignedMessage
}

func (fb *FullBlock) Cid() cid.Cid {
	return fb.Header.Cid()
}
```

It has, besides the Filecoin messages, a header with protocol related information (e.g., its `Height`) which is (like virtually any other piece of data in the Filecoin protocol) stored, retrieved and shared as an IPFS block with its corresponding CID,

```Go
func (b *BlockHeader) Cid() cid.Cid {
	sb, err := b.ToStorageBlock()

	return sb.Cid()
}

func (b *BlockHeader) ToStorageBlock() (block.Block, error) {
	data, err := b.Serialize()

	return github.com/ipfs/go-block-format.block.NewBlockWithCid(data)
}
```

These edited extracts from the `BlockHeader` show how it's treated as an IPFS block, `github.com/ipfs/go-block-format.block.BasicBlock`, to be both stored and referenced by its block storage CID.

This duality permeates the code (and the Filecoin spec for that matter) but it is usually clear within the context to which block we are referring to. Normally the unqualified *block* is reserved for the Filecoin block and we won't usually refer to the IPFS one but only implicitly through the concept of its CID. With enough understanding of both stack's architecture the two definitions can coexist without much confusion as we will abstract away the IPFS layer and just use the CID as an identifier that we now its unique for two sequences of different *raw* byte strings.

(FIXME: We use to do this presentation when talking about `gossipsub` topics and incoming blocks, and had to deal with, besides the block ambiguity, a similar confusion with the *message* term, used in libp2p to name anything that comes through the network, needing to present the extremely confusing hierarchy of a libp2p message containing a Filecoin block, identified by a IPFS block CID, containing Filecoin messages.)

FIXME: Move the following tipset definition to sync or wherever is most needed, to avoid making this more confusing.

Messages from the same round are collected into a block set (`chain/store/fts.go`):

```Go
type FullTipSet struct {
	Blocks []*types.FullBlock
	tipset *types.TipSet
	cids   []cid.Cid
}
```

The "tipset" denomination might be a bit misleading as it doesn't refer *only* to the tip, the block set from the last round in the chain, but to *any* set of blocks, depending on the context the tipset is the actual tip or not. From its own perspective any block set is always the tip because it assumes nothing from following blocks.

## Online

The `node.Online()` configuration function (`node/builder.go`) sets up more than its name implies, the general theme though is that all of the components initialized here depend on the node connecting with the Filecoin network, being "online", through the libp2p stack (`libp2p()` discussed later). We list in this section the most relevant ones corresponding to the full node type (that is, included in the `ApplyIf(isType(repo.FullNode),` call).

`modules.ChainStore()`: creates the `store.ChainStore` structure (`chain/store/store.go`) that wraps the previous stores instantiated in `Repo()`. It is the main point of entry for the node to all chain-related data (FIXME: this is incorrect, we sometimes access its underlying block store directly, and probably shouldn't). Most important, it holds the pointer (`heaviest`) to the current head (see more details in the sync section later).

`ChainExchange()` and `ChainBlockservice()` establish a BitSwap connection (see libp2p() section below) to exchange chain information in the form of `blocks.Block`s stored in the repo. (See sync section for more details, the Filecoin blocks and messages are backed by these raw IPFS blocks that together form the different structures that define the state of the current/heaviest chain.)

`HandleIncomingBlocks()` and `HandleIncomingMessages()`: rather than create other structures they start the services in charge of processing new Filecoin blocks and messages from the network (see `<undefined>` for more information about the topics the node is subscribed to, FIXME: should that be part of the libp2p section or should we expand on gossipsub separately?).

`RunHello()`: starts the services to both send (`(*Service).SayHello()`) and receive (`(*Service).HandleStream()`, `node/hello/hello.go`) `hello` messages with which nodes that establish a new connection with each other exchange chain-related information (namely their genesis block and their chain head, heavies tipset).

`NewSyncer()`: creates the structure and starts the services related to the chain sync process (discussed in detailed in this document).

Although the previous list is not necessarily sorted by their dependency order, we can establish those relations by looking at the parameters each function needs, and most importantly, by understanding the architecture of the node and how the different components relate to each other, the main objective of this document. For example, the sync mechanism depends on the node being able to exchange different IPFS blocks in the network to request the "missing pieces" to reconstruct the chain, this is reflected by `NewSyncer()` having a `blocksync.BlockSync` parameter, and this in turn depends on the above mentioned `ChainBlockservice()` and `ChainExchange()`. Similarly, the chain exchange service depends on the chain store to save and retrieve chain data,  this is reflected by `ChainExchange()` having `ChainGCBlockstore` as a parameter, which is just a wrapper around the `ChainBlockstore` with garbage collection support.

This block store is the same store underlying the chain store which in turn is an indirect dependency (through the `StateManager`) of the `NewSyncer()`. (FIXME: This last line is flaky, we need to resolve the hierarchy better, we sometimes refer to the chain store and sometimes to its underlying block store. We need a diagram to visualize all the different components just mentioned otherwise it is too hard to follow. We probably even need to skip some of the connections mentioned.)

## libp2p

FIXME: This should be a brief section with the relevant links to `libp2p` documentation and how do we use its services.

# VM: chain state

We continue the sync process to give more details on the chain state and how the VM computes it. (FIXME: Is there anything accessible in the spec we can link to?)

The most important fact about the state is that it is just a collection of data stored under a root CID encapsulated in the `StateTree` structure (`chain/state/statetree.go`) and accessed through the `StateManager` (`chain/stmgr/stmgr.go`). (See, currently non-existing, Actors section later for the type of data that the state holds. FIXME: We can at least link to something in the spec for now.) Changing the state is then just changing the value of that CID pointer (`StateTree.root`). (FIXME: We could link to the immutability doc again here, but we might want to explain some of the modify/flush/update-root mechanism here.)

This data is generated by the VM when executing the messages of the blocks of a particular block set (FIXME: at this point the tipset/block set clarification should be made). That means there isn't a *single* state but instead it depends on the block set of the particular epoch we are looking in the chain (we might sometimes talk about an unqualified "chain state" to refer to the state of the current head of the chain). The genesis block starts with a hard-coded state and from then onwards the messages of the following blocks will modify this initial state.

Each tipset will hold a CID pointer to its own *base* state over which its messages are applied (technically each block in the tipset has that `ParentStateRoot` entry, which should be the same for all blocks in the same set, so we normally reference the first one). It is important to note that the state seen by that tipset, by those blocks in it, does *not* depend on the messages *they* contain but on their parent's (tipset) messages. The messages of tipset `N` are applied over the *base* state of tipset `N-1`, since each message is potentially generated by a different party, no message in the same set can rely on the contents of the (yet unknown) other messages that will be included in the same block of the same tipset. (FIXME: Not exactly correct, after the blocks and messages are sorted, the state seen by one message is what the previous one left behind, a sort of "partial" state being built on top of the base, but we may get away with this simplification here and clarify it in the next subsection.)

With this in mind, the sync process then progressively computes the states of each fetch tipset leading up to the new head, starting from the last state computed on the current chain up to, but not including, the new potential head. The messages of the new head will be checked (against its parent's state) but officially its state will not be fully determined until that round is over and we move to the new epoch.

## State computation

We continue the sync process from `syncMessagesAndCheckState()`, that is, we have already retrieved all blocks up to the new head and we need to execute their messages to check if the head is valid. We will mostly skip all validation checks (they will be detailed in a separate document) and focus on the state computation by the VM. The VM is nothing more than the part of the logic that knows how to interpret and execute the messages, it mostly resides in the `github.com/filecoin-project/specs-actors` repo with the "adapter" logic in Lotus to pass state information back and forth from it (see `Runtime` below).

In `ValidateBlock()` where the message validation happens we need then to first compute the state of the previous tipset (for every block up to the new head). For the block being validated then we call `(*StateManager).TipSetState(ts *types.TipSet)` (`chain/stmgr/stmgr.go`) with the block's parent tipset (which is the same for all blocks in a tipset since they share their parents). This is accessible through the block's header in `BlockHeader.Parents`, it holds the list of CIDs of all the parent blocks forming the previous tipset, as we can't reference a state for a single block but only for all blocks in the tipset.

Skipping the cache logic, we go straight to `(*StateManager).ApplyBlocks()` which applies all messages from all blocks together. For that we construct a `NewVM()` (`chain/vm/vm.go`), as although the logic is always the same the associated state inside the VM depends on the parent tipset we are applying the messages to. From this point on we should remember that messages and current state refer always to *different* tipsets (the messages to the current tipset we are computing the state for, and the *base* state on which the messages are applied to the parent tipset). `LoadStateTree(cbor.IpldStore, cid.Cid)` (`chain/state/statetree.go`) is called to load the parent's state, identified by the CID of the root IPFS block of a HAMT structure (see `github.com/ipfs/go-hamt-ipld` for more on HAMT management). The other argument of `LoadStateTree` is an `IpldStore` which is just another wrapper over the original chain store were we store *all* chain data. That means that besides storing Filecoin blocks and messages we also store there all state information generated by them (all this data is accessible through the CID of the underlying IPFS blocks that encode it).

`(*VM).ApplyMessage()` is then called separately for each message that will conform the new state. Since each message is independent of each other it is actually the VM that will consolidate the *aggregated* state from the succession of message executions. The state each message will see is the base state (from the parent tipset) plus all the state modifications the previous messages in the current tipset have executed, so when we talk about the base state, technically only the first message in the tipset will see it, all following messages will see a "partial" state from previous modification to the starting base. No messages is aware of this a priori though, since they were all generated by potentially independent parties that had no information about what everyone else was computing. This aggregated stated is retrieved at the end of `ApplyBlocks()` (after all `ApplyMessage()`s) when `(*VM).Flush()` is called. This function will return the new root CID reflecting the modified contents of its internal `StateTree`, which is ultimately the CID that will be stored in the new blocks being created for the next round under their `ParentStateRoot` attribute (see `MinerCreateBlock` in `chain/gen/mining.go` for more details).

Inside `ApplyMessage()` (ignoring verifications), `(*VM).send()` is called. Messages are deemed with that name because each targets a specific logic of the Filecoin protocol grouped into what are called "actors" (in the OOP model we can think of them as objects that receive their associated methods), *send* in this context means targeting the specific actor the messages should run for. The VM logic (and the actors logic it contains) is decoupled from the rest of the code and depends *only* in the chain state (this way the VM is portable across different implementations of the Filecoin protocol). The way we pass that state is through the `Runtime` structure (`chain/vm/runtime.go`), which most importantly contains the `StateTree` with the initial base state from the parent tipset, over which the VM will run the different messages and continuously update its value.

## Invoker

To translate a Filecoin message for the VM to an actual Go function we rely on reflection logic (which can make the code harder to follow), we outline the most important aspects here. The VM contains an `invoker` structure which is the responsible for making the translation, mapping a message code directed to an actor (each actor has its *own* set of codes defined in `specs-actors/actors/builtin/methods.go`) to an actual Go function (`(*invoker).Invoke()`) stored in itself (`invoker.builtInCode`) of type `invokeFunc`, which takes the `Runtime` (state communication) and the serialized parameters (for the actor's logic):

```
type invoker struct {
	builtInCode  map[cid.Cid]nativeCode
	builtInState map[cid.Cid]reflect.Type
}

type invokeFunc func(rt runtime.Runtime, params []byte) ([]byte, aerrors.ActorError)
type nativeCode []invokeFunc
```

`(*invoker).Register()` stores for each actor available its set of methods exported through its interface:

```
type Invokee interface {
	Exports() []interface{}
}
```

The basic layout (without reflection details) of `(*invoker).transform()` is as follows. From each actor registered in `NewInvoker()` we take its `Exports()` methods converting them to `invokeFunc`s. The actual method is wrapped in another function that takes care of decoding the serialized parameters and the runtime, this function is passed to `shimCall()` that will encapsulate the actors code being run inside a `defer` function to `recover()` from panics (we fail in the actors code with panics to unwrap the stack). The return values will then be (CBOR) marshaled and returned to the VM.

## Returning from the VM

`(*VM).ApplyMessage` will receive back from the invoker the serialized response and the `ActorError`. The returned message will be charged the corresponding gas to be stored in `rt.chargeGasSafe(rt.Pricelist().OnChainReturnValue(len(ret)))`, we don't process the return value any further than this (FIXME: is anything else done with it?).

Besides charging the gas for the returned response, in `(*VM).makeRuntime()` the block store is wrapped in another `gasChargingBlocks` store responsible for charging any other state information generated (through the runtime):

```
func (bs *gasChargingBlocks) Put(blk block.Block) error {
	bs.chargeGas(bs.pricelist.OnIpldPut(len(blk.RawData())))
```

(FIXME: What happens when there is an error? Is that just discarded?)
