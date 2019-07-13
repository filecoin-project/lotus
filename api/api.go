package api

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/address"

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

	ChainHead(context.Context) (*chain.TipSet, error)                // TODO: check serialization
	ChainSubmitBlock(ctx context.Context, blk *chain.BlockMsg) error // TODO: check serialization
	ChainGetRandomness(context.Context, *chain.TipSet) ([]byte, error)

	// messages

	// // wait
	// // send
	// // status
	// // mpool
	// // // ls / show / rm
	MpoolPending(context.Context, *chain.TipSet) ([]*chain.SignedMessage, error)

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
	MinerStart(context.Context, address.Address) error
	MinerCreateBlock(context.Context, address.Address, *chain.TipSet, []chain.Ticket, chain.ElectionProof, []*chain.SignedMessage) (*chain.BlockMsg, error)

	// // UX ?

	// wallet

	WalletNew(context.Context, string) (address.Address, error)
	WalletList(context.Context) ([]address.Address, error)
	// // import
	// // export
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
