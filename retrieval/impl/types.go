package retrievalimpl

import (
	"github.com/filecoin-project/lotus/api"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
)

/* These types are all the types provided by Lotus, which diverge even from
spec V0 -- prior to the "update to spec epic", we are using these types internally
and switching to spec at the boundaries of the module */

type OldQueryResponseStatus uint64

const (
	Available OldQueryResponseStatus = iota
	Unavailable
)

const (
	Accepted = iota
	Error
	Rejected
	Unsealing
)

type OldQuery struct {
	Piece cid.Cid
	// TODO: payment
}

type OldQueryResponse struct {
	Status OldQueryResponseStatus

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

type OldDealProposal struct {
	Payment api.PaymentInfo

	Ref    cid.Cid
	Params RetParams
}

type OldDealResponse struct {
	Status  uint64
	Message string
}

type Block struct { // TODO: put in spec
	Prefix []byte // TODO: fix cid.Prefix marshaling somehow
	Data   []byte
}
