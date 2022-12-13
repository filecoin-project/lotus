/*
Package retrievalmarket implements the Filecoin retrieval protocol.

An overview of the retrieval protocol can be found in the Filecoin specification:

https://filecoin-project.github.io/specs/#systems__filecoin_markets__retrieval_market

The following architectural components provide a brief overview of the design of
the retrieval market module:

Public Interfaces And Node Dependencies

While retrieval deals primarily happen off-chain, there are some chain operations
that must be performed by a Filecoin node implementation. The module is intended to separate
the primarily off-chain retrieval deal flow from the on-chain operations related primarily
to payment channels, the mechanism for getting paid for retrieval deals.

As such for both the client and the provider in the retrieval market, the module defines a top level
public interface which it provides an implementation for, and a node interface that must be implemented
by the Filecoin node itself, and provided as a dependency. These node interfaces provide a universal way to
talk to potentially multiple different Filecoin node implementations, and can be implemented as using HTTP
or other interprocess communication to talk to a node implementation running in a different process.

The top level interfaces this package implements are RetrievalClient & RetrievalProvider. The dependencies the Filecoin
node is expected to implement are RetrievalClientNode & RetrievalProviderNode. Further documentation of exactly what those
dependencies should do can be found in the readme.

Finite State Machines

While retrieval deals in general should be fairly fast, making a retrieval deal is still an asynchronous process.
As documented in the Filecoin spec, the basic architecture of the Filecoin retrieval protocol is incremental payments.
Because neither client nor provider trust each other, we bootstrap trust by essentially paying in small increments as we receive
data. The client only sends payment when it verifies data and the provider only sends more data when it receives payment.
Not surprisingly, many things can go wrong along the way. To manage this back and forth asynchronous process,
we use finite state machines that update deal state when discrete events occur. State updates
always persist state to disk. This means we have a permanent record of exactly what's going on with deals at any time,
and we can ideally survive our Filecoin processes shutting down and restarting.

The following diagrams visualize the statemachine flows for the client and the provider:

Client FSM - https://raw.githubusercontent.com/filecoin-project/go-fil-markets/master/docs/retrievalclient.mmd.svg

Provider FSM - https://raw.githubusercontent.com/filecoin-project/go-fil-markets/master/docs/retrievalprovider.mmd.svg

Identifying Retrieval Providers

The RetrievalClient provides two functions to locate a provider from which to retrieve data.

`FindProviders` returns a list of retrieval peers who may have the data your looking for. FindProviders delegates its work to
an implementation of the PeerResolver interface.

`Query` queries a specific retrieval provider to find out definitively if they have the requested data and if so, the
parameters they will accept for a retrieval deal.

Deal Flow

The primary mechanism for initiating storage deals is the `Retrieve` method on the RetrievalClient.

When `Retrieve` is called, it allocates a new DealID from its stored counter, constructs a DealProposal, sends
the deal proposal to the provider, initiates tracking of deal state and hands the deal to the Client FSM,
and returns the DealID which constitutes the identifier for that deal.

The Retrieval provider receives the deal in `HandleDealStream`. `HandleDealStream` initiates tracking of deal state
on the Provider side and hands the deal to the Provider FSM, which handles the rest of deal flow.

From this point forward, deal negotiation is completely asynchronous and runs in the FSMs.

A user of the modules can monitor deal progress through `SubscribeToEvents` methods on RetrievalClient and RetrievalProvider,
or by simply calling `ListDeals` to get all deal statuses.

The FSMs implement every remaining step in deal negotiation. Importantly, the RetrievalProvider delegates unsealing sectors
back to the node via the `UnsealSector` method (the node itself likely delegates management of sectors and sealing to an
implementation of the Storage Mining subsystem of the Filecoin spec). Sectors are unsealed on an as needed basis using
the `PieceStore` to locate sectors that contain data related to the deal.

Major Dependencies

Other libraries in go-fil-markets:

https://github.com/filecoin-project/go-fil-markets/tree/master/piecestore - used to locate data for deals in sectors
https://github.com/filecoin-project/go-fil-markets/tree/master/shared - types and utility functions shared with
storagemarket package

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

Root package

This top level package defines top level enumerations and interfaces. The primary implementation
lives in the `impl` directory

*/
package retrievalmarket
