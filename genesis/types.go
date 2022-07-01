package genesis

import (
	"encoding/json"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	market8 "github.com/filecoin-project/go-state-types/builtin/v8/market"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/wallet/key"
)

type ActorType string

const (
	TAccount  ActorType = "account"
	TMultisig ActorType = "multisig"
)

type PreSeal struct {
	CommR         cid.Cid
	CommD         cid.Cid
	SectorID      abi.SectorNumber
	Deal          market8.DealProposal
	DealClientKey *key.Key
	ProofType     abi.RegisteredSealProof
}

type Miner struct {
	ID     address.Address
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

type Actor struct {
	Type    ActorType
	Balance abi.TokenAmount

	Meta json.RawMessage
}

type Template struct {
	NetworkVersion network.Version
	Accounts       []Actor
	Miners         []Miner

	NetworkName string
	Timestamp   uint64 `json:",omitempty"`

	VerifregRootKey  Actor
	RemainderAccount Actor
}
