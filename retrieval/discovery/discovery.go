package discovery

import (
	cbor "github.com/ipfs/go-ipld-cbor"

	retrievalmarket "github.com/filecoin-project/lotus/retrieval"
)

func init() {
	cbor.RegisterCborType(retrievalmarket.RetrievalPeer{})
}

func Multi(r retrievalmarket.PeerResolver) retrievalmarket.PeerResolver { // TODO: actually support multiple mechanisms
	return r
}
