package gateway

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-state-types/abi"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var _ api.Gateway = (*reverseProxyV1)(nil)

type reverseProxyV1 struct {
	gateway       *Node
	server        v1api.FullNode
	subscriptions *EthSubHandler
}

func (pv1 *reverseProxyV1) MpoolPending(ctx context.Context, tsk types.TipSetKey) ([]*types.SignedMessage, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.MpoolPending(ctx, tsk)
}

func (pv1 *reverseProxyV1) ChainGetBlock(ctx context.Context, c cid.Cid) (*types.BlockHeader, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetBlock(ctx, c)
}

func (pv1 *reverseProxyV1) MinerGetBaseInfo(ctx context.Context, addr address.Address, h abi.ChainEpoch, tsk types.TipSetKey) (*api.MiningBaseInfo, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.MinerGetBaseInfo(ctx, addr, h, tsk)
}

func (pv1 *reverseProxyV1) StateReplay(ctx context.Context, tsk types.TipSetKey, c cid.Cid) (*api.InvocResult, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateReplay(ctx, tsk, c)
}

func (pv1 *reverseProxyV1) GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return types.BigInt{}, err
	}
	return pv1.server.GasEstimateGasPremium(ctx, nblocksincl, sender, gaslimit, tsk)
}

func (pv1 *reverseProxyV1) StateMinerSectorCount(ctx context.Context, m address.Address, tsk types.TipSetKey) (api.MinerSectors, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return api.MinerSectors{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return api.MinerSectors{}, err
	}
	return pv1.server.StateMinerSectorCount(ctx, m, tsk)
}

func (pv1 *reverseProxyV1) Discover(ctx context.Context) (apitypes.OpenRPCDocument, error) {
	return build.OpenRPCDiscoverJSON_Gateway(), nil
}

func (pv1 *reverseProxyV1) Version(ctx context.Context) (api.APIVersion, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return api.APIVersion{}, err
	}
	return pv1.server.Version(ctx)
}

func (pv1 *reverseProxyV1) ChainGetParentMessages(ctx context.Context, c cid.Cid) ([]api.Message, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetParentMessages(ctx, c)
}

func (pv1 *reverseProxyV1) ChainGetParentReceipts(ctx context.Context, c cid.Cid) ([]*types.MessageReceipt, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetParentReceipts(ctx, c)
}

func (pv1 *reverseProxyV1) ChainGetMessagesInTipset(ctx context.Context, tsk types.TipSetKey) ([]api.Message, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetMessagesInTipset(ctx, tsk)
}

func (pv1 *reverseProxyV1) ChainGetBlockMessages(ctx context.Context, c cid.Cid) (*api.BlockMessages, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetBlockMessages(ctx, c)
}

func (pv1 *reverseProxyV1) ChainHasObj(ctx context.Context, c cid.Cid) (bool, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return false, err
	}
	return pv1.server.ChainHasObj(ctx, c)
}

func (pv1 *reverseProxyV1) ChainHead(ctx context.Context) (*types.TipSet, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}

	return pv1.server.ChainHead(ctx)
}

func (pv1 *reverseProxyV1) ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetMessage(ctx, mc)
}

func (pv1 *reverseProxyV1) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetTipSet(ctx, tsk)
}

func (pv1 *reverseProxyV1) ChainGetFinalizedTipSet(ctx context.Context) (*types.TipSet, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetFinalizedTipSet(ctx)
}

func (pv1 *reverseProxyV1) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkKeyedTipSetHeight(ctx, h, tsk); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetTipSetByHeight(ctx, h, tsk)
}

func (pv1 *reverseProxyV1) ChainGetTipSetAfterHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkKeyedTipSetHeight(ctx, h, tsk); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetTipSetAfterHeight(ctx, h, tsk)
}

func (pv1 *reverseProxyV1) ChainGetNode(ctx context.Context, param string) (*api.IpldObject, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetNode(ctx, param)
}

func (pv1 *reverseProxyV1) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainNotify(ctx)
}

func (pv1 *reverseProxyV1) ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*api.HeadChange, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, from); err != nil {
		return nil, xerrors.Errorf("gateway: checking 'from' tipset: %w", err)
	}
	if err := pv1.gateway.checkTipSetKey(ctx, to); err != nil {
		return nil, xerrors.Errorf("gateway: checking 'to' tipset: %w", err)
	}
	return pv1.server.ChainGetPath(ctx, from, to)
}

func (pv1 *reverseProxyV1) ChainGetGenesis(ctx context.Context) (*types.TipSet, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetGenesis(ctx)
}

func (pv1 *reverseProxyV1) ChainReadObj(ctx context.Context, c cid.Cid) ([]byte, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainReadObj(ctx, c)
}

func (pv1 *reverseProxyV1) ChainPutObj(context.Context, blocks.Block) error {
	return xerrors.New("not supported")
}

func (pv1 *reverseProxyV1) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.GasEstimateMessageGas(ctx, msg, spec, tsk)
}

func (pv1 *reverseProxyV1) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}
	return pv1.server.MpoolGetNonce(ctx, addr)
}

func (pv1 *reverseProxyV1) MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return cid.Cid{}, err
	}
	// TODO: additional anti-spam checks
	return pv1.server.MpoolPushUntrusted(ctx, sm)
}

func (pv1 *reverseProxyV1) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	if err := pv1.gateway.limit(ctx, walletRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return types.NewInt(0), err
	}
	return pv1.server.MsigGetAvailableBalance(ctx, addr, tsk)
}

func (pv1 *reverseProxyV1) MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error) {
	if err := pv1.gateway.limit(ctx, walletRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, end); err != nil {
		return types.NewInt(0), err
	}
	return pv1.server.MsigGetVested(ctx, addr, start, end)
}

func (pv1 *reverseProxyV1) MsigGetVestingSchedule(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MsigVesting, error) {
	if err := pv1.gateway.limit(ctx, walletRateLimitTokens); err != nil {
		return api.MsigVesting{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return api.MsigVesting{}, err
	}
	return pv1.server.MsigGetVestingSchedule(ctx, addr, tsk)
}

func (pv1 *reverseProxyV1) MsigGetPending(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*api.MsigTransaction, error) {
	if err := pv1.gateway.limit(ctx, walletRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.MsigGetPending(ctx, addr, tsk)
}

func (pv1 *reverseProxyV1) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return address.Address{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return address.Undef, err
	}
	return pv1.server.StateAccountKey(ctx, addr, tsk)
}

func (pv1 *reverseProxyV1) StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (*api.InvocResult, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateCall(ctx, msg, tsk)
}

func (pv1 *reverseProxyV1) StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return api.DealCollateralBounds{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return api.DealCollateralBounds{}, err
	}
	return pv1.server.StateDealProviderCollateralBounds(ctx, size, verified, tsk)
}

func (pv1 *reverseProxyV1) StateDecodeParams(ctx context.Context, toAddr address.Address, method abi.MethodNum, params []byte, tsk types.TipSetKey) (interface{}, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateDecodeParams(ctx, toAddr, method, params, tsk)
}

func (pv1 *reverseProxyV1) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateGetActor(ctx, actor, tsk)
}

func (pv1 *reverseProxyV1) StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateListMiners(ctx, tsk)
}

func (pv1 *reverseProxyV1) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return address.Address{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return address.Undef, err
	}
	return pv1.server.StateLookupID(ctx, addr, tsk)
}

func (pv1 *reverseProxyV1) StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return api.MarketBalance{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return api.MarketBalance{}, err
	}
	return pv1.server.StateMarketBalance(ctx, addr, tsk)
}

func (pv1 *reverseProxyV1) StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateMarketStorageDeal(ctx, dealId, tsk)
}

func (pv1 *reverseProxyV1) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return *new(dtypes.NetworkName), err
	}
	return pv1.server.StateNetworkName(ctx)
}

func (pv1 *reverseProxyV1) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return network.VersionMax, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return network.VersionMax, err
	}
	return pv1.server.StateNetworkVersion(ctx, tsk)
}

func (pv1 *reverseProxyV1) StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if limit == api.LookbackNoLimit {
		limit = pv1.gateway.maxMessageLookbackEpochs
	}
	if pv1.gateway.maxMessageLookbackEpochs != api.LookbackNoLimit && limit > pv1.gateway.maxMessageLookbackEpochs {
		limit = pv1.gateway.maxMessageLookbackEpochs
	}
	if err := pv1.gateway.checkTipSetKey(ctx, from); err != nil {
		return nil, err
	}
	return pv1.server.StateSearchMsg(ctx, from, msg, limit, allowReplaced)
}

func (pv1 *reverseProxyV1) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if limit == api.LookbackNoLimit {
		limit = pv1.gateway.maxMessageLookbackEpochs
	}
	if pv1.gateway.maxMessageLookbackEpochs != api.LookbackNoLimit && limit > pv1.gateway.maxMessageLookbackEpochs {
		limit = pv1.gateway.maxMessageLookbackEpochs
	}
	return pv1.server.StateWaitMsg(ctx, msg, confidence, limit, allowReplaced)
}

func (pv1 *reverseProxyV1) GetActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) ([]*types.ActorEvent, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if filter != nil && filter.FromHeight != nil {
		if err := pv1.gateway.checkKeyedTipSetHeight(ctx, *filter.FromHeight, types.EmptyTSK); err != nil {
			return nil, err
		}
	}
	return pv1.server.GetActorEventsRaw(ctx, filter)
}

func (pv1 *reverseProxyV1) SubscribeActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) (<-chan *types.ActorEvent, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if filter != nil && filter.FromHeight != nil {
		if err := pv1.gateway.checkKeyedTipSetHeight(ctx, *filter.FromHeight, types.EmptyTSK); err != nil {
			return nil, err
		}
	}
	return pv1.server.SubscribeActorEventsRaw(ctx, filter)
}

func (pv1 *reverseProxyV1) ChainGetEvents(ctx context.Context, eventsRoot cid.Cid) ([]types.Event, error) {
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.ChainGetEvents(ctx, eventsRoot)
}

func (pv1 *reverseProxyV1) StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*api.ActorState, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateReadState(ctx, actor, tsk)
}

func (pv1 *reverseProxyV1) StateMinerPower(ctx context.Context, m address.Address, tsk types.TipSetKey) (*api.MinerPower, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateMinerPower(ctx, m, tsk)
}

func (pv1 *reverseProxyV1) StateMinerFaults(ctx context.Context, m address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return bitfield.BitField{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return bitfield.BitField{}, err
	}
	return pv1.server.StateMinerFaults(ctx, m, tsk)
}

func (pv1 *reverseProxyV1) StateMinerRecoveries(ctx context.Context, m address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return bitfield.BitField{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return bitfield.BitField{}, err
	}
	return pv1.server.StateMinerRecoveries(ctx, m, tsk)
}

func (pv1 *reverseProxyV1) StateMinerInfo(ctx context.Context, m address.Address, tsk types.TipSetKey) (api.MinerInfo, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return api.MinerInfo{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return api.MinerInfo{}, err
	}
	return pv1.server.StateMinerInfo(ctx, m, tsk)
}

func (pv1 *reverseProxyV1) StateMinerDeadlines(ctx context.Context, m address.Address, tsk types.TipSetKey) ([]api.Deadline, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateMinerDeadlines(ctx, m, tsk)
}

func (pv1 *reverseProxyV1) StateMinerAvailableBalance(ctx context.Context, m address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return types.BigInt{}, err
	}
	return pv1.server.StateMinerAvailableBalance(ctx, m, tsk)
}

func (pv1 *reverseProxyV1) StateMinerProvingDeadline(ctx context.Context, m address.Address, tsk types.TipSetKey) (*dline.Info, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateMinerProvingDeadline(ctx, m, tsk)
}

func (pv1 *reverseProxyV1) StateCirculatingSupply(ctx context.Context, tsk types.TipSetKey) (abi.TokenAmount, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return abi.TokenAmount{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return abi.TokenAmount{}, err
	}
	return pv1.server.StateCirculatingSupply(ctx, tsk)
}

func (pv1 *reverseProxyV1) StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateSectorGetInfo(ctx, maddr, n, tsk)
}

func (pv1 *reverseProxyV1) StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateVerifiedClientStatus(ctx, addr, tsk)
}

func (pv1 *reverseProxyV1) StateVerifierStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateVerifierStatus(ctx, addr, tsk)
}

func (pv1 *reverseProxyV1) StateVMCirculatingSupplyInternal(ctx context.Context, tsk types.TipSetKey) (api.CirculatingSupply, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return api.CirculatingSupply{}, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return api.CirculatingSupply{}, err
	}
	return pv1.server.StateVMCirculatingSupplyInternal(ctx, tsk)
}

func (pv1 *reverseProxyV1) WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *crypto.Signature) (bool, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}
	return sigs.Verify(sig, k, msg) == nil, nil
}

func (pv1 *reverseProxyV1) WalletBalance(ctx context.Context, k address.Address) (types.BigInt, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	return pv1.server.WalletBalance(ctx, k)
}

func (pv1 *reverseProxyV1) StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateGetAllocationForPendingDeal(ctx, dealId, tsk)
}

func (pv1 *reverseProxyV1) StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateGetAllocation(ctx, clientAddr, allocationId, tsk)
}

func (pv1 *reverseProxyV1) StateGetAllocations(ctx context.Context, clientAddr address.Address, tsk types.TipSetKey) (map[verifregtypes.AllocationId]verifregtypes.Allocation, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateGetAllocations(ctx, clientAddr, tsk)
}

func (pv1 *reverseProxyV1) StateGetClaim(ctx context.Context, providerAddr address.Address, claimId verifregtypes.ClaimId, tsk types.TipSetKey) (*verifregtypes.Claim, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateGetClaim(ctx, providerAddr, claimId, tsk)
}

func (pv1 *reverseProxyV1) StateGetClaims(ctx context.Context, providerAddr address.Address, tsk types.TipSetKey) (map[verifregtypes.ClaimId]verifregtypes.Claim, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkTipSetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateGetClaims(ctx, providerAddr, tsk)
}

func (pv1 *reverseProxyV1) StateGetNetworkParams(ctx context.Context) (*api.NetworkParams, error) {
	if err := pv1.gateway.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.StateGetNetworkParams(ctx)
}

func (pv1 *reverseProxyV1) StateGetRandomnessDigestFromBeacon(ctx context.Context, randEpoch abi.ChainEpoch, tsk types.TipSetKey) (abi.Randomness, error) {
	// limited by chainRateLimitTokens not stateRateLimitTokens because this only needs to read from the chain
	if err := pv1.gateway.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := pv1.gateway.checkKeyedTipSetHeight(ctx, randEpoch, tsk); err != nil {
		return nil, err
	}
	return pv1.server.StateGetRandomnessDigestFromBeacon(ctx, randEpoch, tsk)
}

func (pv1 *reverseProxyV1) F3GetCertificate(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.F3GetCertificate(ctx, instance)
}

func (pv1 *reverseProxyV1) F3GetPowerTableByInstance(ctx context.Context, instance uint64) (gpbft.PowerEntries, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.F3GetPowerTableByInstance(ctx, instance)
}

func (pv1 *reverseProxyV1) F3GetLatestCertificate(ctx context.Context) (*certs.FinalityCertificate, error) {
	if err := pv1.gateway.limit(ctx, basicRateLimitTokens); err != nil {
		return nil, err
	}
	return pv1.server.F3GetLatestCertificate(ctx)
}
