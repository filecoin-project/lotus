package types

import (
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
)

type MarketDeal struct {
	Proposal market.DealProposal
	State    market.DealState
}

type MarketBalance struct {
	Escrow big.Int
	Locked big.Int
}
