package genesis

import (
	"encoding/json"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
)

type ActorType string
const (
	TAccount  ActorType = "account"
	TMultisig ActorType = "multisig"
)

type PreSeal struct {
	CommR    [32]byte
	CommD    [32]byte
	SectorID abi.SectorNumber
	Deal     market.DealProposal
}

type Miner struct {
	Owner  address.Address
	Worker address.Address

	MarketBalance abi.TokenAmount
	PowerBalance  abi.TokenAmount

	SectorSize abi.SectorSize

	Sectors []*PreSeal
}

type AccountMeta struct {
	Owner address.Address // bls / secpk
}

type MultisigMeta struct {
	// TODO
}

type Actor struct {
	Type    ActorType
	Balance abi.TokenAmount

	Meta json.RawMessage
}

type Template struct {
	Accounts []Actor
	Miners   []Miner

	NetworkName string
	Timestamp   uint64
}
