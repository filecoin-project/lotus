package retrieval

import (
	"github.com/filecoin-project/lotus/api"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
)

const ProtocolID = "/fil/retrieval/-1.0.0"          // TODO: spec
const QueryProtocolID = "/fil/retrieval/qry/-1.0.0" // TODO: spec

type QueryResponseStatus uint64

const (
	Available QueryResponseStatus = iota
	Unavailable
)

const (
	Accepted = iota
	Error
	Rejected
	Unsealing
)

func init() {
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
	Offset uint64
	Size   uint64
}

type RetParams struct {
	Unixfs0 *Unixfs0Offer
}

type DealProposal struct {
	Payment api.PaymentInfo

	Ref    cid.Cid
	Params RetParams
}

type DealResponse struct {
	Status  uint64
	Message string
}

type Block struct { // TODO: put in spec
	Prefix []byte // TODO: fix cid.Prefix marshaling somehow
	Data   []byte
}
