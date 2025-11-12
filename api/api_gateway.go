package api

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/dline"

	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

//                       MODIFYING THE API INTERFACE
//
// When adding / changing methods in this file:
// * Do the change here
// * Adjust implementation in `node/impl/`
// * Run `make clean && make deps && make gen` - this will:
//  * Generate proxy structs
//  * Generate mocks
//  * Generate markdown docs
//  * Generate openrpc blobs

// Gateway provides a secure, limited interface to the Filecoin network for
// public-facing services. It exposes a carefully curated subset of the FullNode
// API that excludes administrative operations, wallet management, and other
// sensitive functionality that should not be accessible through public endpoints.
//
// This API is ideal for:
//   - Public RPC endpoints and infrastructure providers
//   - Applications that need read-only access to chain data
//   - Services requiring basic transaction submission without node administration
//   - Load balancers and proxy services in front of full nodes
//
// This represents the Lotus v1 API, which is stable and maintains backward
// compatibility.
type Gateway interface {
	ChainGetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainPutObj(context.Context, blocks.Block) error
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainGetEvents(context.Context, cid.Cid) ([]types.Event, error)
	ChainGetParentMessages(context.Context, cid.Cid) ([]Message, error)
	ChainGetParentReceipts(context.Context, cid.Cid) ([]*types.MessageReceipt, error)
	ChainGetMessagesInTipset(context.Context, types.TipSetKey) ([]Message, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*BlockMessages, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*HeadChange, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetFinalizedTipSet(ctx context.Context) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetAfterHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*HeadChange, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	ChainGetGenesis(context.Context) (*types.TipSet, error)

	GasEstimateGasPremium(context.Context, uint64, address.Address, int64, types.TipSetKey) (types.BigInt, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)

	MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error)
	MpoolPending(context.Context, types.TipSetKey) ([]*types.SignedMessage, error)
	MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error)

	MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
	MsigGetPending(context.Context, address.Address, types.TipSetKey) ([]*MsigTransaction, error)
	MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
	MsigGetVestingSchedule(ctx context.Context, addr address.Address, tsk types.TipSetKey) (MsigVesting, error)

	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (*InvocResult, error)
	StateReplay(context.Context, types.TipSetKey, cid.Cid) (*InvocResult, error)
	StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (DealCollateralBounds, error)
	StateDecodeParams(ctx context.Context, toAddr address.Address, method abi.MethodNum, params []byte, tsk types.TipSetKey) (interface{}, error)
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
	StateGetAllocations(ctx context.Context, clientAddr address.Address, tsk types.TipSetKey) (map[verifregtypes.AllocationId]verifregtypes.Allocation, error)
	StateGetClaim(ctx context.Context, providerAddr address.Address, claimId verifregtypes.ClaimId, tsk types.TipSetKey) (*verifregtypes.Claim, error)
	StateGetClaims(ctx context.Context, providerAddr address.Address, tsk types.TipSetKey) (map[verifregtypes.ClaimId]verifregtypes.Claim, error)
	StateGetNetworkParams(ctx context.Context) (*NetworkParams, error)
	StateGetRandomnessDigestFromBeacon(ctx context.Context, randEpoch abi.ChainEpoch, tsk types.TipSetKey) (abi.Randomness, error)
	StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*ActorState, error)
	StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error)
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (MarketBalance, error)
	StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*MarketDeal, error)
	StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (MinerInfo, error)
	StateMinerDeadlines(context.Context, address.Address, types.TipSetKey) ([]Deadline, error)
	StateMinerProvingDeadline(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*dline.Info, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (*MinerPower, error)
	StateMinerSectorCount(context.Context, address.Address, types.TipSetKey) (MinerSectors, error)
	StateNetworkName(context.Context) (dtypes.NetworkName, error)
	StateNetworkVersion(context.Context, types.TipSetKey) (apitypes.NetworkVersion, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateVerifierStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error)
	StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*MsgLookup, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*MsgLookup, error)

	MinerGetBaseInfo(context.Context, address.Address, abi.ChainEpoch, types.TipSetKey) (*MiningBaseInfo, error)

	WalletBalance(context.Context, address.Address) (types.BigInt, error)

	EthAddressToFilecoinAddress(ctx context.Context, ethAddress ethtypes.EthAddress) (address.Address, error)
	FilecoinAddressToEthAddress(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthAddress, error)
	EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error)
	EthBlockNumber(ctx context.Context) (ethtypes.EthUint64, error)
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum string) (ethtypes.EthUint64, error)
	EthGetBlockTransactionCountByHash(ctx context.Context, blkHash ethtypes.EthHash) (ethtypes.EthUint64, error)
	EthGetBlockByHash(ctx context.Context, blkHash ethtypes.EthHash, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetTransactionByHash(ctx context.Context, txHash *ethtypes.EthHash) (*ethtypes.EthTx, error)
	EthGetTransactionHashByCid(ctx context.Context, cid cid.Cid) (*ethtypes.EthHash, error)
	EthGetMessageCidByTransactionHash(ctx context.Context, txHash *ethtypes.EthHash) (*cid.Cid, error)
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error)
	EthGetTransactionReceipt(ctx context.Context, txHash ethtypes.EthHash) (*ethtypes.EthTxReceipt, error)
	EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*ethtypes.EthTxReceipt, error)
	EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error)
	EthChainId(ctx context.Context) (ethtypes.EthUint64, error)
	EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error)
	NetVersion(ctx context.Context) (string, error)
	NetListening(ctx context.Context) (bool, error)
	EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error)
	EthGasPrice(ctx context.Context) (ethtypes.EthBigInt, error)
	EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error)
	EthMaxPriorityFeePerGas(ctx context.Context) (ethtypes.EthBigInt, error)
	EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error)
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)
	EthGetLogs(ctx context.Context, filter *ethtypes.EthFilterSpec) (*ethtypes.EthFilterResult, error)
	EthGetFilterChanges(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error)
	EthGetFilterLogs(ctx context.Context, id ethtypes.EthFilterID) (*ethtypes.EthFilterResult, error)
	EthNewFilter(ctx context.Context, filter *ethtypes.EthFilterSpec) (ethtypes.EthFilterID, error)
	EthNewBlockFilter(ctx context.Context) (ethtypes.EthFilterID, error)
	EthNewPendingTransactionFilter(ctx context.Context) (ethtypes.EthFilterID, error)
	EthUninstallFilter(ctx context.Context, id ethtypes.EthFilterID) (bool, error)
	EthSubscribe(ctx context.Context, params jsonrpc.RawParams) (ethtypes.EthSubscriptionID, error)
	EthUnsubscribe(ctx context.Context, id ethtypes.EthSubscriptionID) (bool, error)
	Web3ClientVersion(ctx context.Context) (string, error)
	EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error)
	EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error)
	EthTraceTransaction(ctx context.Context, txHash string) ([]*ethtypes.EthTraceTransaction, error)
	EthTraceFilter(ctx context.Context, filter ethtypes.EthTraceFilterCriteria) ([]*ethtypes.EthTraceFilterResult, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, index ethtypes.EthUint64) (*ethtypes.EthTx, error)
	EthGetTransactionByBlockHashAndIndex(ctx context.Context, blkHash ethtypes.EthHash, index ethtypes.EthUint64) (*ethtypes.EthTx, error)
	GetActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) ([]*types.ActorEvent, error)
	SubscribeActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) (<-chan *types.ActorEvent, error)

	F3GetCertificate(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error)
	F3GetLatestCertificate(ctx context.Context) (*certs.FinalityCertificate, error)
	F3GetPowerTableByInstance(ctx context.Context, instance uint64) (gpbft.PowerEntries, error)

	Version(context.Context) (APIVersion, error)
	Discover(context.Context) (apitypes.OpenRPCDocument, error)
}
