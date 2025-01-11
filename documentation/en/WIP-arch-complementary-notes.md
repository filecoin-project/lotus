# Genesis block

Seems a good way to start exploring the VM state through the instantiation of its different actors like the storage power.

Explain where we load the genesis block, the CAR entries, and we set the root of the state. Follow the daemon command option, `chain.LoadGenesis()` saves all the blocks of the CAR file into the store provided by `ChainBlockstore` (this should already be explained in the previous section). The CAR root (MT root?) of those blocks is decoded into the `BlockHeader` that will be the Filecoin (genesis) block of the chain, but most of the information was stored in the raw data (non-Filecoin, what's the correct term?) blocks forwarded directly to the chain, the block header just has a pointer to it.

`SetGenesis` block with name 0. `(ChainStore).SetGenesis()` stores it there.

`MakeInitialStateTree` (`chain/gen/genesis/genesis.go`, used to construct the genesis block (`MakeGenesisBlock()`), constructs the state tree (`NewStateTree`) which is just a "pointer" (root node in the HAMT) to the different actors. It will be continuously used in `(*StateTree).SetActor()` an `types.Actor` structure under a certain `Address` (in the HAMT). (How does the `stateSnaps` work? It has no comments.)

From this point we can follow different setup functions like:

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

Follow the `lotus-miner` command to see how a miner is created, from the command to the message to the storage power logic.

# Directory structure so far, main structures seen, their relation

List what are the main directories we should be looking at (e.g., `chain/`) and the most important structures (e.g., `StateTree`, `Runtime`, etc.)

# Tests

Run a few messages and observe state changes. What is the easiest test that also lets us "interact" with it (modify something and observe the difference).

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

This duality permeates the code (and the Filecoin spec for that matter) but it is usually clear within the context to which block we are referring to. Normally the unqualified *block* is reserved for the Filecoin block and we won't usually refer to the IPFS one but only implicitly through the concept of its CID. With enough understanding of both stack's architecture the two definitions can coexist without much confusion as we will abstract away the IPFS layer and just use the CID as an identifier that we know is unique for two sequences of different *raw* byte strings.

(FIXME: We use to do this presentation when talking about `gossipsub` topics and incoming blocks, and had to deal with, besides the block ambiguity, a similar confusion with the *message* term, used in libp2p to name anything that comes through the network, needing to present the extremely confusing hierarchy of a libp2p message containing a Filecoin block, identified by an IPFS block CID, containing Filecoin messages.)

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
  lotus log set-level error # Or an env.var in the daemon command.

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
