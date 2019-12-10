package discovery

import (
	retrievalmarket "github.com/filecoin-project/lotus/retrieval"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbor.RegisterCborType(retrievalmarket.RetrievalPeer{})
}

func Multi(r retrievalmarket.PeerResolver) retrievalmarket.PeerResolver { // TODO: actually support multiple mechanisms
	return r
}
