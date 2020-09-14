package storageadapter

import (
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
)

// Journal entry types emitted from this module.
const (
	evtTypeDealAccepted = iota
	evtTypeDealSectorCommitted
	evtTypeDealExpired
	evtTypeDealSlashed
)

type ClientDealAcceptedEvt struct {
	ID     abi.DealID
	Deal   storagemarket.ClientDeal
	Height abi.ChainEpoch
}

type ClientDealSectorCommittedEvt struct {
	ID     abi.DealID
	State  market.DealState
	Height abi.ChainEpoch
}

type ClientDealExpiredEvt struct {
	ID     abi.DealID
	State  market.DealState
	Height abi.ChainEpoch
}

type ClientDealSlashedEvt struct {
	ID     abi.DealID
	State  market.DealState
	Height abi.ChainEpoch
}

type MinerDealAcceptedEvt struct {
	ID     abi.DealID
	Deal   storagemarket.MinerDeal
	State  market.DealState
	Height abi.ChainEpoch
}

type MinerDealSectorCommittedEvt struct {
	ID     abi.DealID
	State  market.DealState
	Height abi.ChainEpoch
}

type MinerDealExpiredEvt struct {
	ID     abi.DealID
	State  market.DealState
	Height abi.ChainEpoch
}

type MinerDealSlashedEvt struct {
	ID     abi.DealID
	State  market.DealState
	Height abi.ChainEpoch
}
