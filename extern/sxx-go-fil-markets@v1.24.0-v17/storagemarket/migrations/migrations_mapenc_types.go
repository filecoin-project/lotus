package migrations

import (
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"
	marketOld "github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

// Some of the types in the migrations file are CBOR array-encoded, and some
// are map-encoded. The --map-encoding parameter must be specified in a
// generate directive in a separate file. So we define CBOR map-encoded types
// in this file

//go:generate cbor-gen-for --map-encoding Proposal1 MinerDeal1

// Proposal1 is version 1 of Proposal (used by deal proposal protocol v1.1.0)
type Proposal1 struct {
	DealProposal  *marketOld.ClientDealProposal
	Piece         *storagemarket.DataRef
	FastRetrieval bool
}

// MinerDeal1 is version 1 of MinerDeal
type MinerDeal1 struct {
	marketOld.ClientDealProposal
	ProposalCid           cid.Cid
	AddFundsCid           *cid.Cid
	PublishCid            *cid.Cid
	Miner                 peer.ID
	Client                peer.ID
	State                 storagemarket.StorageDealStatus
	PiecePath             filestore.Path
	MetadataPath          filestore.Path
	SlashEpoch            abi.ChainEpoch
	FastRetrieval         bool
	Message               string
	FundsReserved         abi.TokenAmount
	Ref                   *storagemarket.DataRef
	AvailableForRetrieval bool

	DealID       abi.DealID
	CreationTime cbg.CborTime

	TransferChannelId *datatransfer.ChannelID
	SectorNumber      abi.SectorNumber

	InboundCAR string
}
