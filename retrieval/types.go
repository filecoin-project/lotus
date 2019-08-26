package retrieval

import (
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-lotus/chain/types"
)

const ProtocolID = "/fil/retrieval/-1.0.0"          // TODO: spec
const QueryProtocolID = "/fil/retrieval/qry/-1.0.0" // TODO: spec

type QueryResponse int

const (
	Available QueryResponse = iota
	Unavailable
)

func init() {
	cbor.RegisterCborType(RetDealProposal{})

	cbor.RegisterCborType(RetQuery{})
	cbor.RegisterCborType(RetQueryResponse{})
}

type RetDealProposal struct {
	Piece   cid.Cid
	Price   types.BigInt
	Payment types.SignedVoucher
}

type RetQuery struct {
	Piece cid.Cid
}

type RetQueryResponse struct {
	Status QueryResponse

	Size uint64 // TODO: spec
	// TODO: unseal price (+spec)
	// TODO: address to send money for the deal?
	MinPrice types.BigInt
}
