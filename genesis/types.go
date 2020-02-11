package genesis

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/lotus/chain/types"
)

type PreSeal struct {
	CommR    [32]byte
	CommD    [32]byte
	SectorID abi.SectorNumber
	Deal     market.DealProposal
}

type GenesisMiner struct {
	Owner  address.Address
	Worker address.Address

	MarketBalance abi.TokenAmount
	PowerBalance  abi.TokenAmount
	WorkerBalance abi.TokenAmount

	SectorSize abi.SectorSize

	Sectors []*PreSeal

	Key types.KeyInfo // TODO: separate file
}
