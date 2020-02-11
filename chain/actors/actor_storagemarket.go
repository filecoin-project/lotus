package actors

import (
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
)

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
