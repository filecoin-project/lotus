/*
Package network providers an abstraction over a libp2p host for managing retrieval's Libp2p protocols:

network.go - defines the interfaces that must be implemented to serve as a retrieval network
query_stream.go  - implements the `RetrievalQueryStream` interface, a data stream for retrieval query traffic only
libp2p_impl.go - provides the production implementation of the `RetrievalMarketNetwork` interface.
*/
package network
