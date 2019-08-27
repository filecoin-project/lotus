package retrieval

import (
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-lotus/chain/types"
)

const ProtocolID = "/fil/retrieval/-1.0.0"          // TODO: spec
const QueryProtocolID = "/fil/retrieval/qry/-1.0.0" // TODO: spec

type QueryResponseStatus int

const (
	Available QueryResponseStatus = iota
	Unavailable
)

const (
	Accepted = iota
	Error
	Rejected
)

func init() {
	cbor.RegisterCborType(Deal{})

	cbor.RegisterCborType(Query{})
	cbor.RegisterCborType(QueryResponse{})
	cbor.RegisterCborType(Unixfs0Offer{})

	cbor.RegisterCborType(DealResponse{})
}

type Query struct {
	Piece cid.Cid
	// TODO: payment
}

type QueryResponse struct {
	Status QueryResponseStatus

	Size uint64 // TODO: spec
	// TODO: unseal price (+spec)
	// TODO: sectors to unseal
	// TODO: address to send money for the deal?
	MinPrice types.BigInt
}

type Unixfs0Offer struct {
	Root   cid.Cid
	Offset uint64
	Size   uint64
}

type Deal struct {
	Unixfs0 *Unixfs0Offer
}

type DealResponse struct {
	Status  int // TODO: make this more spec complainant
	Message string
}
