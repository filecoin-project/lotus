package v0api

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/dline"
	abinetwork "github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

//                       MODIFYING THE API INTERFACE
//
// NOTE: This is the v0 (Deprecated) API - when adding methods to this interface,
// you'll need to make sure they are also present on the v1 (Stable) API
//
// This API is implemented in `v1_wrapper.go` as a compatibility layer backed
// by the v1 api
//
// When adding / changing methods in this file:
// * Do the change here
// * Adjust implementation in `node/impl/`
// * Run `make gen` - this will:
//  * Generate proxy structs
//  * Generate mocks
//  * Generate markdown docs
//  * Generate openrpc blobs

type Gateway interface {
	MpoolPending(context.Context, types.TipSetKey) ([]*types.SignedMessage, error)
	ChainGetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
	MinerGetBaseInfo(context.Context, address.Address, abi.ChainEpoch, types.TipSetKey) (*api.MiningBaseInfo, error)
	StateMinerSectorCount(context.Context, address.Address, types.TipSetKey) (api.MinerSectors, error)
	GasEstimateGasPremium(context.Context, uint64, address.Address, int64, types.TipSetKey) (types.BigInt, error)
	StateReplay(context.Context, types.TipSetKey, cid.Cid) (*api.InvocResult, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainPutObj(context.Context, blocks.Block) error
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
	MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error)
	MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error)
	MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
	MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
	MsigGetPending(context.Context, address.Address, types.TipSetKey) ([]*api.MsigTransaction, error)
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (*api.InvocResult, error)
	StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error)
	StateDecodeParams(ctx context.Context, toAddr address.Address, method abi.MethodNum, params []byte, tsk types.TipSetKey) (interface{}, error)
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
	StateGetAllocations(ctx context.Context, clientAddr address.Address, tsk types.TipSetKey) (map[verifregtypes.AllocationId]verifregtypes.Allocation, error)
	StateGetClaim(ctx context.Context, providerAddr address.Address, claimId verifregtypes.ClaimId, tsk types.TipSetKey) (*verifregtypes.Claim, error)
	StateGetClaims(ctx context.Context, providerAddr address.Address, tsk types.TipSetKey) (map[verifregtypes.ClaimId]verifregtypes.Claim, error)
	StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error)
	StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error)
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error)
	StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error)
	StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (api.MinerInfo, error)
	StateMinerProvingDeadline(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*dline.Info, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (*api.MinerPower, error)
	StateNetworkName(context.Context) (dtypes.NetworkName, error)
	StateNetworkVersion(context.Context, types.TipSetKey) (abinetwork.Version, error)
	StateSearchMsg(ctx context.Context, msg cid.Cid) (*api.MsgLookup, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error)
	StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	Version(context.Context) (api.APIVersion, error)
}

var _ Gateway = *new(FullNode)
