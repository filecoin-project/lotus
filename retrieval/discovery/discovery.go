package discovery

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
)

func init() {
	cbor.RegisterCborType(RetrievalPeer{})
}

type RetrievalPeer struct {
	Address address.Address
	ID      peer.ID // optional
}

type PeerResolver interface {
	GetPeers(data cid.Cid) ([]RetrievalPeer, error) // TODO: channel
}

func Multi(r PeerResolver) PeerResolver { // TODO: actually support multiple mechanisms
	return r
}
