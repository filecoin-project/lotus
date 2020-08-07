package genesis

import (
	"encoding/json"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

type ActorType string

const (
	TAccount  ActorType = "account"
	TMultisig ActorType = "multisig"
)

type PreSeal struct {
	CommR     cid.Cid
	CommD     cid.Cid
	SectorID  abi.SectorNumber
	Deal      market.DealProposal
	ProofType abi.RegisteredSealProof
}

type Miner struct {
	Owner  address.Address
	Worker address.Address
	PeerId peer.ID //nolint:golint

	MarketBalance abi.TokenAmount
	PowerBalance  abi.TokenAmount

	SectorSize abi.SectorSize

	Sectors []*PreSeal
}

type AccountMeta struct {
	Owner address.Address // bls / secpk
}

func (am *AccountMeta) ActorMeta() json.RawMessage {
	out, err := json.Marshal(am)
	if err != nil {
		panic(err)
	}
	return out
}

type MultisigMeta struct {
	Signers         []address.Address
	Threshold       int
	VestingDuration int
	VestingStart    int
}

func (mm *MultisigMeta) ActorMeta() json.RawMessage {
	out, err := json.Marshal(mm)
	if err != nil {
		panic(err)
	}
	return out
}

type BindMiner struct {
	SealProof abi.RegisteredSealProof
	Address   address.Address
}

type Actor struct {
	Type       ActorType
	Balance    abi.TokenAmount
	BindMiners []BindMiner
	Meta       json.RawMessage
}

type Template struct {
	Accounts []Actor
	Miners   []Miner

	NetworkName string
	InitIDStart uint64
	Timestamp   uint64 `json:",omitempty"`

	VerifregRootKey Actor
}
