package maptypes

import (
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

//go:generate cbor-gen-for --map-encoding ClientDealState1 ProviderDealState1

// Version 1 of the ClientDealState
type ClientDealState1 struct {
	retrievalmarket.DealProposal
	StoreID              *uint64
	ChannelID            datatransfer.ChannelID
	LastPaymentRequested bool
	AllBlocksReceived    bool
	TotalFunds           abi.TokenAmount
	ClientWallet         address.Address
	MinerWallet          address.Address
	PaymentInfo          *retrievalmarket.PaymentInfo
	Status               retrievalmarket.DealStatus
	Sender               peer.ID
	TotalReceived        uint64
	Message              string
	BytesPaidFor         uint64
	CurrentInterval      uint64
	PaymentRequested     abi.TokenAmount
	FundsSpent           abi.TokenAmount
	UnsealFundsPaid      abi.TokenAmount
	WaitMsgCID           *cid.Cid
	VoucherShortfall     abi.TokenAmount
	LegacyProtocol       bool
}

// Version 1 of the ProviderDealState
type ProviderDealState1 struct {
	retrievalmarket.DealProposal
	StoreID         uint64
	ChannelID       datatransfer.ChannelID
	PieceInfo       *piecestore.PieceInfo
	Status          retrievalmarket.DealStatus
	Receiver        peer.ID
	TotalSent       uint64
	FundsReceived   abi.TokenAmount
	Message         string
	CurrentInterval uint64
	LegacyProtocol  bool
}
