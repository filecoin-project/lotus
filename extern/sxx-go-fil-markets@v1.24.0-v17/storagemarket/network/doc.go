/*
Package network providers an abstraction over a libp2p host for managing storage markets's Libp2p protocols:

network.go - defines the interfaces that must be implemented to serve as a storage network layer
deal_stream.go - implements the `StorageDealStream` interface, a data stream for proposing storage deals
ask_stream.go  - implements the `StorageAskStream` interface, a data stream for querying provider asks
deal_status_stream.go - implements the `StorageDealStatusStream` interface, a data stream for querying for deal status
libp2p_impl.go - provides the production implementation of the `StorageMarketNetwork` interface.
types.go - types for messages sent on the storage market libp2p protocols
*/
package network
