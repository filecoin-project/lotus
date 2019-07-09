package api

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain"

	"github.com/ipfs/go-cid"
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

	ChainHead(context.Context) ([]cid.Cid, error)
	ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error // TODO: check serialization

	// messages

	// // wait
	// // send
	// // status
	// // mpool
	// // // ls / show / rm
	MpoolPending(context.Context) ([]*chain.SignedMessage, error)

	// dag

	// // get block
	// // (cli: show / info)

	// network

	NetPeers(context.Context) ([]peer.AddrInfo, error)
	NetConnect(context.Context, peer.AddrInfo) error
	NetAddrsListen(context.Context) (peer.AddrInfo, error)
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
