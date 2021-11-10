package api

import (
	"context"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"

	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

//                       MODIFYING THE API INTERFACE
//
// NOTE: This is the V1 (Unstable) API - to add methods to the V0 (Stable) API
// you'll have to add those methods to interfaces in `api/v0api`
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
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*BlockMessages, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*HeadChange, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetAfterHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*HeadChange, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
	MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error)
	MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
	MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
	MsigGetPending(context.Context, address.Address, types.TipSetKey) ([]*MsigTransaction, error)
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (DealCollateralBounds, error)
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*ActorState, error) //perm:read
	StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error)
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (MarketBalance, error)
	StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*MarketDeal, error)
	StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (miner.MinerInfo, error)
	StateMinerProvingDeadline(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*dline.Info, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (*MinerPower, error)
	StateNetworkVersion(context.Context, types.TipSetKey) (apitypes.NetworkVersion, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*MsgLookup, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*MsgLookup, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	Version(context.Context) (APIVersion, error)
}
