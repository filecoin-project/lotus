package types

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

type MsgLookup struct {
	Message   cid.Cid // Can be different than requested, in case it was replaced, but only gas values changed
	Receipt   MessageReceipt
	ReturnDec interface{}
	TipSet    TipSetKey
	Height    abi.ChainEpoch
}

type InvocResult struct {
	MsgCid         cid.Cid
	Msg            *Message
	MsgRct         *MessageReceipt
	GasCost        MsgGasCost
	ExecutionTrace ExecutionTrace
	Error          string
	Duration       time.Duration
}

type MsgGasCost struct {
	Message            cid.Cid // Can be different than requested, in case it was replaced, but only gas values changed
	GasUsed            abi.TokenAmount
	BaseFeeBurn        abi.TokenAmount
	OverEstimationBurn abi.TokenAmount
	MinerPenalty       abi.TokenAmount
	MinerTip           abi.TokenAmount
	Refund             abi.TokenAmount
	TotalCost          abi.TokenAmount
}

type BlockTemplate struct {
	Miner            address.Address
	Parents          TipSetKey
	Ticket           *Ticket
	Eproof           *ElectionProof
	BeaconValues     []BeaconEntry
	Messages         []*SignedMessage
	Epoch            abi.ChainEpoch
	Timestamp        uint64
	WinningPoStProof []builtin.PoStProof
}

type MarketDeal struct {
	Proposal market.DealProposal
	State    market.DealState
}

type MiningBaseInfo struct {
	MinerPower        BigInt
	NetworkPower      BigInt
	Sectors           []builtin.ExtendedSectorInfo
	WorkerKey         address.Address
	SectorSize        abi.SectorSize
	PrevBeaconEntry   BeaconEntry
	BeaconEntries     []BeaconEntry
	EligibleForMining bool
}

type MarketBalance struct {
	Escrow big.Int
	Locked big.Int
}

type CirculatingSupply struct {
	FilVested           abi.TokenAmount
	FilMined            abi.TokenAmount
	FilBurnt            abi.TokenAmount
	FilLocked           abi.TokenAmount
	FilCirculating      abi.TokenAmount
	FilReserveDisbursed abi.TokenAmount
}

type MsgSigningMeta struct {
	Type MsgType

	// Additional data related to what is signed. Should be verifiable with the
	// signed bytes (e.g. CID(Extra).Bytes() == toSign)
	Extra []byte
}

type MsgType string

const (
	MTUnknown = "unknown"

	// Signing message CID. MsgSigningMeta.Extra contains raw cbor message bytes
	MTChainMsg = "message"

	// Signing a blockheader. signing raw cbor block bytes (MsgSigningMeta.Extra is empty)
	MTBlock = "block"

	// Signing a deal proposal. signing raw cbor proposal bytes (MsgSigningMeta.Extra is empty)
	MTDealProposal = "dealproposal"

	// TODO: Deals, Vouchers, VRF
)

type HeadChange struct {
	Type string
	Val  *TipSet
}

const LookbackNoLimit = abi.ChainEpoch(-1)

type Signer interface {
	WalletSign(ctx context.Context, signer address.Address, toSign []byte, meta MsgSigningMeta) (*crypto.Signature, error)
}
