package actors

import (
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
)

type smaMethods struct {
	Constructor                  uint64
	WithdrawBalance              uint64
	AddBalance                   uint64
	CheckLockedBalance           uint64
	PublishStorageDeals          uint64
	HandleCronAction             uint64
	SettleExpiredDeals           uint64
	ProcessStorageDealsPayment   uint64
	SlashStorageDealCollateral   uint64
	GetLastExpirationFromDealIDs uint64
	ActivateStorageDeals         uint64
	ComputeDataCommitment        uint64
}

var SMAMethods = smaMethods{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

type StorageMarketState = market.State

// TODO: Drop in favour of car storage
type SerializationMode = uint64

const (
	SerializationUnixFSv0 = iota
	// IPLD / car
)

type StorageDealProposal = market.DealProposal

type WithdrawBalanceParams = market.WithdrawBalanceParams

type PublishStorageDealsParams = market.PublishStorageDealsParams

type PublishStorageDealResponse = market.PublishStorageDealsReturn

type ActivateStorageDealsParams = market.VerifyDealsOnSectorProveCommitParams

type ComputeDataCommitmentParams = market.ComputeDataCommitmentParams

/*
func (sma StorageMarketActor) HandleCronAction(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) SettleExpiredDeals(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) SlashStorageDealCollateral(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) GetLastExpirationFromDealIDs(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}
*/
