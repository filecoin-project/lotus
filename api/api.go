package api

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Version provides various build-time information
type Version struct {
	Version string

	// TODO: git commit / os / genesis cid?
}

// API is a low-level interface to the Filecoin network
type API interface {
	// chain

	// // head

	// messages

	// // wait
	// // send
	// // status
	// // mpool
	// // // ls / show / rm

	// dag

	// // get block
	// // (cli: show / info)

	// network

	NetPeers(context.Context) ([]peer.AddrInfo, error) // TODO: check serialization
	NetConnect(context.Context, peer.AddrInfo) error
	// // ping

	// Struct

	// miner

	// // create
	// // owner
	// // power
	// // set-price
	// // set-perrid

	// // UX ?

	// wallet

	// // import
	// // export
	// // list
	// // (on cli - cmd to list associations)

	// dht

	// // need ?

	// paych

	// // todo

	// retrieval

	// // retrieve piece

	// Other

	// // ID (on cli - print with other info)

	// ID returns peerID of libp2p node backing this API
	ID(context.Context) (peer.ID, error)

	// Version provides information about API provider
	Version(context.Context) (Version, error)
}
