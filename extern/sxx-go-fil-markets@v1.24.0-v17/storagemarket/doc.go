/*
Package storagemarket implements the Filecoin storage protocol.

An overview of the storage protocol can be found in the Filecoin specification:

https://filecoin-project.github.io/specs/#systems__filecoin_markets__storage_market

The following architectural components provide a brief overview of the design of
the storagemarket module:

Public Interfaces And Node Dependencies

A core goal of this module is to isolate the negotiation of deals from the actual chain operations
performed by the node to put the deal on chain. The module primarily orchestrates the storage deal
flow, rather than performing specific chain operations which are delegated to the node.

As such, for both the client and the provider in the storage market, the module defines a top level
public interface which it provides an implementation for, and a node interface that must be implemented
by the Filecoin node itself, and provided as a dependency. These node interfaces provide a universal way to
talk to potentially multiple different Filecoin node implementations, and can be implemented using HTTP
or some other interprocess communication to talk to a node implementation running in a different process.

The top level interfaces this package implements are StorageClient & StorageProvider. The dependencies the Filecoin
node is expected to implement are StorageClientNode & StorageProviderNode. Further documentation of exactly what those
dependencies should do can be found in the readme.

Finite State Machines and Resumability

Making deals in Filecoin is a highly asynchronous process. For a large piece of data, it's possible that the entire
process of proposing a deal, transferring data, publishing the deal, putting the data in a sector and sealing it
could take hours or even days. Not surprisingly, many things can go wrong along the way. To manage the process
of orchestrating deals, we use finite state machines that update deal state when discrete events occur. State updates
always persist state to disk. This means we have a permanent record of exactly what's going on with deals at any time,
and we can ideally survive our Filecoin processes shutting down and restarting.

The following diagrams visualize the statemachine flows for the client and the provider:

Client FSM - https://raw.githubusercontent.com/filecoin-project/go-fil-markets/master/docs/storageclient.mmd.svg

Provider FSM - https://raw.githubusercontent.com/filecoin-project/go-fil-markets/master/docs/storageprovider.mmd.svg

Identifying Providers For A Deal

The StorageClient provides two functions to locate a provider with whom to make a deal:

`ListProviders` returns a list of storage providers on the Filecoin network. This list is assembled by
querying the chain state for active storage miners.

`QueryAsk` queries a single provider for more specific details about the kinds of deals they accept, as
expressed through a `StorageAsk`.

Deal Flow

The primary mechanism for initiating storage deals is the `ProposeStorageDeal` method on the StorageClient.

When `ProposeStorageDeal` is called, it constructs and signs a DealProposal, initiates tracking of deal state
and hands the deal to the Client FSM, returning the CID of the DealProposal which constitutes the identifier for
that deal.

After some preparation steps, the FSM will send the deal proposal to the StorageProvider, which receives the deal
in `HandleDealStream`. `HandleDealStream` initiates tracking of deal state on the Provider side and hands the deal to
the Provider FSM, which handles the rest of deal flow.

From this point forward, deal negotiation is completely asynchronous and runs in the FSMs.

A user of the modules can monitor deal progress through `SubscribeToEvents` methods on StorageClient and StorageProvider,
or by simply calling `ListLocalDeals` to get all deal statuses.

The FSMs implement every step in deal negotiation up to deal publishing. However, adding the deal to a sector and sealing
it is handled outside this module. When a deal is published, the StorageProvider calls `OnDealComplete` on the StorageProviderNode
interface (the node itself likely delegates management of sectors and sealing to an implementation of the Storage Mining subsystem
of the Filecoin spec). At this point, the markets implementations essentially shift to being monitors of deal progression:
they wait to see and record when the deal becomes active and later expired or slashed.

When a deal becomes active on chain, the provider records the location of where it's stored in a sector in the PieceStore,
so that it's available for retrieval.

Major Dependencies

Other libraries in go-fil-markets:

https://github.com/filecoin-project/go-fil-markets/tree/master/filestore - used to store pieces and other
temporary data before it's transferred to either a sector or the PieceStore.

https://github.com/filecoin-project/go-fil-markets/tree/master/pieceio - used to convert back and forth between raw
payload data and pieces that fit in sector. Also provides utilities for generating CommP.

https://github.com/filecoin-project/go-fil-markets/tree/master/piecestore - used to write information about where data
lives in sectors so that it can later be retrieved.

https://github.com/filecoin-project/go-fil-markets/tree/master/shared - types and utility functions shared with
retrievalmarket package.

Other Filecoin Repos:

https://github.com/filecoin-project/go-data-transfer - for transferring data, via go-graphsync

https://github.com/filecoin-project/go-statemachine - a finite state machine that tracks deal state

https://github.com/filecoin-project/go-storedcounter - for generating and persisting unique deal IDs

https://github.com/filecoin-project/specs-actors - the Filecoin actors

IPFS Project Repos:

https://github.com/ipfs/go-graphsync - used by go-data-transfer

https://github.com/ipfs/go-datastore - for persisting statemachine state for deals

https://github.com/ipfs/go-ipfs-blockstore - for storing and retrieving block data for deals

Other Repos:

https://github.com/libp2p/go-libp2p) the network over which retrieval deal data is exchanged.

https://github.com/hannahhoward/go-pubsub - for pub/sub notifications external to the statemachine

Root Package

This top level package defines top level enumerations and interfaces. The primary implementation
lives in the `impl` directory

*/
package storagemarket
