package retrieval

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-lotus/chain/types"
)

type QueryResponse int
const (
	Available QueryResponse = iota
	Unavailable
)

type RetDealProposal struct {
	Piece cid.Cid
	Price types.BigInt
	Payment types.SignedVoucher
}

type RetQuery struct {
	Piece cid.Cid
}

type RetQueryResponse struct {
	Status QueryResponse

	MinPricePerMiB types.BigInt // TODO: check units used for sector size
}


