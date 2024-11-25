package v2api

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

//go:generate go run github.com/golang/mock/mockgen -destination=v2mocks/mock_full.go -package=v2mocks . FullNode

type FullNode interface {
	// ChainNotify returns channel with chain head updates. Takes an optional
	// EpochSelector argument to specify the point in the chain to follow, which
	// can be either "latest" or "finalized". This defaults to "latest" if not
	// specified.
	//
	// The first message is guaranteed to be of length == 1, and type ==
	// "current".
	//
	// The optional argument is decoded as a types.OptionalEpochSelectorArg.
	ChainNotify(ctx context.Context, p jsonrpc.RawParams) (<-chan []*api.HeadChange, error) //perm:read

	// ChainHead returns the current head of the chain. Takens an optional
	// EpochSelector argument to specify the point in the chain to query, which
	// can be either "latest" or "finalized". This defaults to "latest" if not
	// specified.
	//
	// The optional argument is decoded as a types.OptionalEpochSelectorArg.
	ChainHead(ctx context.Context, p jsonrpc.RawParams) (*types.TipSet, error) //perm:read

	// ChainGetMessagesInTipset returns message stores in current tipset. Takes
	// a TipSetKey or an EpochSelector (either "latest" or "finalized") as an
	// argument.
	ChainGetMessagesInTipset(ctx context.Context, tskes types.TipSetKeyOrEpochSelector) ([]api.Message, error) //perm:read

	// ChainGetTipSetByHeight looks back for a tipset at at the specified epoch
	// or epoch selector (either "latest" or "finalized") and returns the tipset
	// at that epoch.
	// If there are no blocks at the specified epoch because it was a null
	// round, a tipset at an earlier epoch will be returned.
	// The second argument is an optional TipSetKey or EpochSelector (either
	// "latest" or "finalized") to specify the point in the chain to begin
	// looking for the tipset.
	ChainGetTipSetByHeight(ctx context.Context, hes types.HeightOrEpochSelector, tskes types.TipSetKeyOrEpochSelector) (*types.TipSet, error) //perm:read

	// ChainGetTipSetAfterHeight looks back for a tipset at the specified epoch
	// or epoch selector (either "latest" or "finalized") and returns the tipset
	// at that epoch.
	// If there are no blocks at the specified epoch because it was a null
	// round, a tipset at a later epoch will be returned.
	// The second argument is an optional TipSetKey or EpochSelector (either
	// "latest" or "finalized") to specify the point in the chain to begin
	// looking for the tipset.
	ChainGetTipSetAfterHeight(ctx context.Context, hes types.HeightOrEpochSelector, tskes types.TipSetKeyOrEpochSelector) (*types.TipSet, error) //perm:read

	// TODO: consider these for tsk argument extension using TipSetKeyOrEpochSelector
	//
	// ChainTipSetWeight
	// ChainGetPath -- from [foo,bar] to "latest" would be neat, or from "finalized" to "latest"
	// ChainExport -- maybe best to be explicit?
	// GasEstimateFeeCap
	// GasEstimateGasLimit
	// GasEstimateGasPremium
	// GasEstimateMessageGas
	// MpoolPending - is this useful? what would it buy us?
	// MpoolSelect - is this useful? what would it buy us?
	// MinerGetBaseInfo - I don't think this is useful to extend, it's for mining base
	// StateCall
	// StateReplay
	// StateGetActor
	// StateReadState
	// StateListMessages
	// * StateDecodeParams - not in Common Node API, and maybe not so useful to augment?
	// StateMinerSectors
	// StateMinerActiveSectors
	// StateMinerProvingDeadline
	// StateMinerPower
	// StateMinerInfo
	// StateMinerDeadlines
	// StateMinerPartitions
	// StateMinerFaults
	// * StateAllMinerFaults - not on Common Node API, probably used for CLI, check this
	// StateMinerRecoveries
	// StateMinerPreCommitDepositForPower
	// StateMinerInitialPledgeForSector
	// StateMinerAvailableBalance
	// StateMinerSectorAllocated
	// StateSectorPreCommitInfo
	// StateSectorGetInfo
	// StateSectorPartition
	// StateSearchMsg -- not last argument
	// StateListMiners
	// StateListActors
	// StateMarketBalance
	// StateMarketParticipants
	// StateMarketDeals
	// StateMarketStorageDeal
	// StateGetAllocationForPendingDeal
	// StateGetAllocationIdForPendingDeal
	// StateGetAllocation
	// StateGetAllocations
	// StateGetAllAllocations
	// StateGetClaim
	// StateGetClaims
	// StateGetAllClaims
	// * StateComputeDataCID - not in Common Node API, do people use this?
	// StateLookupID
	// StateAccountKey
	// StateLookupRobustAddress
	// StateMinerSectorCount
	// StateMinerAllocated
	// * StateCompute - not in Common Node API, is flexibility useful here?
	// StateVerifierStatus
	// StateVerifiedClientStatus
	// StateVerifiedRegistryRootKey
	// StateDealProviderCollateralBounds
	// StateCirculatingSupply
	// StateVMCirculatingSupplyInternal
	// * StateNetworkVersion - not in Common Node API
	// StateGetRandomnessFromTickets (not the randEpoch arg)
	// StateGetRandomnessFromBeacon (not the randEpoch arg)
	// StateGetRandomnessDigestFromTickets (not the randEpoch arg)
	// StateGetRandomnessDigestFromBeacon (not the randEpoch arg)
	// * MsigGetAvailableBalance - not in Common Node API
	// * MsigGetVestingSchedule - not in Common Node API
	// * MsigGetVested - not in Common Node API
	// * MsigGetPending - not in Common Node API

	// TODO: consider these for an optional extra argument EpochSelector
	// WalletBalance
	// StateWaitMsg - could we wait until "finalized"?

	// ActorEventFilter for SubscribeActorEventsRaw and GetActorEventsRaw
	// takes epochs in "from" and "to" fields, could be useful to extend
	// with EpochSelector
	// (not in Common Node API)

	// StateGetBeaconEntry - has an epoch arg, probably not useful to augment
}
