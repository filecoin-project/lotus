package gateway

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func (gw *Node) MpoolPending(ctx context.Context, tsk types.TipSetKey) ([]*types.SignedMessage, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.MpoolPending(ctx, tsk)
}

func (gw *Node) ChainGetBlock(ctx context.Context, c cid.Cid) (*types.BlockHeader, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetBlock(ctx, c)
}

func (gw *Node) MinerGetBaseInfo(ctx context.Context, addr address.Address, h abi.ChainEpoch, tsk types.TipSetKey) (*api.MiningBaseInfo, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.MinerGetBaseInfo(ctx, addr, h, tsk)
}

func (gw *Node) StateReplay(ctx context.Context, tsk types.TipSetKey, c cid.Cid) (*api.InvocResult, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateReplay(ctx, tsk, c)
}

func (gw *Node) GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, tsk types.TipSetKey) (types.BigInt, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return types.BigInt{}, err
	}
	return gw.target.GasEstimateGasPremium(ctx, nblocksincl, sender, gaslimit, tsk)
}

func (gw *Node) StateMinerSectorCount(ctx context.Context, m address.Address, tsk types.TipSetKey) (api.MinerSectors, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return api.MinerSectors{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return api.MinerSectors{}, err
	}
	return gw.target.StateMinerSectorCount(ctx, m, tsk)
}

func (gw *Node) Discover(ctx context.Context) (apitypes.OpenRPCDocument, error) {
	return build.OpenRPCDiscoverJSON_Gateway(), nil
}

func (gw *Node) Version(ctx context.Context) (api.APIVersion, error) {
	if err := gw.limit(ctx, basicRateLimitTokens); err != nil {
		return api.APIVersion{}, err
	}
	return gw.target.Version(ctx)
}

func (gw *Node) ChainGetParentMessages(ctx context.Context, c cid.Cid) ([]api.Message, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetParentMessages(ctx, c)
}

func (gw *Node) ChainGetParentReceipts(ctx context.Context, c cid.Cid) ([]*types.MessageReceipt, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetParentReceipts(ctx, c)
}

func (gw *Node) ChainGetBlockMessages(ctx context.Context, c cid.Cid) (*api.BlockMessages, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetBlockMessages(ctx, c)
}

func (gw *Node) ChainHasObj(ctx context.Context, c cid.Cid) (bool, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return false, err
	}
	return gw.target.ChainHasObj(ctx, c)
}

func (gw *Node) ChainHead(ctx context.Context) (*types.TipSet, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}

	return gw.target.ChainHead(ctx)
}

func (gw *Node) ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetMessage(ctx, mc)
}

func (gw *Node) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetTipSet(ctx, tsk)
}

func (gw *Node) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipSetHeight(ctx, h, tsk); err != nil {
		return nil, err
	}
	return gw.target.ChainGetTipSetByHeight(ctx, h, tsk)
}

func (gw *Node) ChainGetTipSetAfterHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipSetHeight(ctx, h, tsk); err != nil {
		return nil, err
	}
	return gw.target.ChainGetTipSetAfterHeight(ctx, h, tsk)
}

func (gw *Node) checkTipSetHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) error {
	var ts *types.TipSet
	if tsk.IsEmpty() {
		head, err := gw.target.ChainHead(ctx)
		if err != nil {
			return err
		}
		ts = head
	} else {
		gts, err := gw.target.ChainGetTipSet(ctx, tsk)
		if err != nil {
			return err
		}
		ts = gts
	}

	// Check if the tipset key refers to gw tipset that's too far in the past
	if err := gw.checkTipset(ts); err != nil {
		return err
	}

	// Check if the height is too far in the past
	if err := gw.checkTipsetHeight(ts, h); err != nil {
		return err
	}

	return nil
}

func (gw *Node) ChainGetNode(ctx context.Context, p string) (*api.IpldObject, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetNode(ctx, p)
}

func (gw *Node) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainNotify(ctx)
}

func (gw *Node) ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*api.HeadChange, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, from); err != nil {
		return nil, xerrors.Errorf("gateway: checking 'from' tipset: %w", err)
	}
	if err := gw.checkTipsetKey(ctx, to); err != nil {
		return nil, xerrors.Errorf("gateway: checking 'to' tipset: %w", err)
	}
	return gw.target.ChainGetPath(ctx, from, to)
}

func (gw *Node) ChainGetGenesis(ctx context.Context) (*types.TipSet, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetGenesis(ctx)
}

func (gw *Node) ChainReadObj(ctx context.Context, c cid.Cid) ([]byte, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainReadObj(ctx, c)
}

func (gw *Node) ChainPutObj(context.Context, blocks.Block) error {
	return xerrors.New("not supported")
}

func (gw *Node) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.GasEstimateMessageGas(ctx, msg, spec, tsk)
}

func (gw *Node) MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return 0, err
	}
	return gw.target.MpoolGetNonce(ctx, addr)
}

func (gw *Node) MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return cid.Cid{}, err
	}
	// TODO: additional anti-spam checks
	return gw.target.MpoolPushUntrusted(ctx, sm)
}

func (gw *Node) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	if err := gw.limit(ctx, walletRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return types.NewInt(0), err
	}
	return gw.target.MsigGetAvailableBalance(ctx, addr, tsk)
}

func (gw *Node) MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error) {
	if err := gw.limit(ctx, walletRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	if err := gw.checkTipsetKey(ctx, end); err != nil {
		return types.NewInt(0), err
	}
	return gw.target.MsigGetVested(ctx, addr, start, end)
}

func (gw *Node) MsigGetVestingSchedule(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MsigVesting, error) {
	if err := gw.limit(ctx, walletRateLimitTokens); err != nil {
		return api.MsigVesting{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return api.MsigVesting{}, err
	}
	return gw.target.MsigGetVestingSchedule(ctx, addr, tsk)
}

func (gw *Node) MsigGetPending(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*api.MsigTransaction, error) {
	if err := gw.limit(ctx, walletRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.MsigGetPending(ctx, addr, tsk)
}

func (gw *Node) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return address.Address{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return address.Undef, err
	}
	return gw.target.StateAccountKey(ctx, addr, tsk)
}

func (gw *Node) StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (*api.InvocResult, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateCall(ctx, msg, tsk)
}

func (gw *Node) StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.DealCollateralBounds{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return api.DealCollateralBounds{}, err
	}
	return gw.target.StateDealProviderCollateralBounds(ctx, size, verified, tsk)
}

func (gw *Node) StateDecodeParams(ctx context.Context, toAddr address.Address, method abi.MethodNum, params []byte, tsk types.TipSetKey) (interface{}, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateDecodeParams(ctx, toAddr, method, params, tsk)
}

func (gw *Node) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateGetActor(ctx, actor, tsk)
}

func (gw *Node) StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateListMiners(ctx, tsk)
}

func (gw *Node) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return address.Address{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return address.Undef, err
	}
	return gw.target.StateLookupID(ctx, addr, tsk)
}

func (gw *Node) StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.MarketBalance{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return api.MarketBalance{}, err
	}
	return gw.target.StateMarketBalance(ctx, addr, tsk)
}

func (gw *Node) StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateMarketStorageDeal(ctx, dealId, tsk)
}

func (gw *Node) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return *new(dtypes.NetworkName), err
	}
	return gw.target.StateNetworkName(ctx)
}

func (gw *Node) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return network.VersionMax, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return network.VersionMax, err
	}
	return gw.target.StateNetworkVersion(ctx, tsk)
}

func (gw *Node) StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if limit == api.LookbackNoLimit {
		limit = gw.maxMessageLookbackEpochs
	}
	if gw.maxMessageLookbackEpochs != api.LookbackNoLimit && limit > gw.maxMessageLookbackEpochs {
		limit = gw.maxMessageLookbackEpochs
	}
	if err := gw.checkTipsetKey(ctx, from); err != nil {
		return nil, err
	}
	return gw.target.StateSearchMsg(ctx, from, msg, limit, allowReplaced)
}

func (gw *Node) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if limit == api.LookbackNoLimit {
		limit = gw.maxMessageLookbackEpochs
	}
	if gw.maxMessageLookbackEpochs != api.LookbackNoLimit && limit > gw.maxMessageLookbackEpochs {
		limit = gw.maxMessageLookbackEpochs
	}
	return gw.target.StateWaitMsg(ctx, msg, confidence, limit, allowReplaced)
}

func (gw *Node) GetActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) ([]*types.ActorEvent, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if filter != nil && filter.FromHeight != nil {
		if err := gw.checkTipSetHeight(ctx, *filter.FromHeight, types.EmptyTSK); err != nil {
			return nil, err
		}
	}
	return gw.target.GetActorEventsRaw(ctx, filter)
}

func (gw *Node) SubscribeActorEventsRaw(ctx context.Context, filter *types.ActorEventFilter) (<-chan *types.ActorEvent, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if filter != nil && filter.FromHeight != nil {
		if err := gw.checkTipSetHeight(ctx, *filter.FromHeight, types.EmptyTSK); err != nil {
			return nil, err
		}
	}
	return gw.target.SubscribeActorEventsRaw(ctx, filter)
}

func (gw *Node) ChainGetEvents(ctx context.Context, eventsRoot cid.Cid) ([]types.Event, error) {
	if err := gw.limit(ctx, chainRateLimitTokens); err != nil {
		return nil, err
	}
	return gw.target.ChainGetEvents(ctx, eventsRoot)
}

func (gw *Node) StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*api.ActorState, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateReadState(ctx, actor, tsk)
}

func (gw *Node) StateMinerPower(ctx context.Context, m address.Address, tsk types.TipSetKey) (*api.MinerPower, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateMinerPower(ctx, m, tsk)
}

func (gw *Node) StateMinerFaults(ctx context.Context, m address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return bitfield.BitField{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return bitfield.BitField{}, err
	}
	return gw.target.StateMinerFaults(ctx, m, tsk)
}

func (gw *Node) StateMinerRecoveries(ctx context.Context, m address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return bitfield.BitField{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return bitfield.BitField{}, err
	}
	return gw.target.StateMinerRecoveries(ctx, m, tsk)
}

func (gw *Node) StateMinerInfo(ctx context.Context, m address.Address, tsk types.TipSetKey) (api.MinerInfo, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.MinerInfo{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return api.MinerInfo{}, err
	}
	return gw.target.StateMinerInfo(ctx, m, tsk)
}

func (gw *Node) StateMinerDeadlines(ctx context.Context, m address.Address, tsk types.TipSetKey) ([]api.Deadline, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateMinerDeadlines(ctx, m, tsk)
}

func (gw *Node) StateMinerAvailableBalance(ctx context.Context, m address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return types.BigInt{}, err
	}
	return gw.target.StateMinerAvailableBalance(ctx, m, tsk)
}

func (gw *Node) StateMinerProvingDeadline(ctx context.Context, m address.Address, tsk types.TipSetKey) (*dline.Info, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateMinerProvingDeadline(ctx, m, tsk)
}

func (gw *Node) StateCirculatingSupply(ctx context.Context, tsk types.TipSetKey) (abi.TokenAmount, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return abi.TokenAmount{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return abi.TokenAmount{}, err
	}
	return gw.target.StateCirculatingSupply(ctx, tsk)
}

func (gw *Node) StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateSectorGetInfo(ctx, maddr, n, tsk)
}

func (gw *Node) StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateVerifiedClientStatus(ctx, addr, tsk)
}

func (gw *Node) StateVerifierStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateVerifierStatus(ctx, addr, tsk)
}

func (gw *Node) StateVMCirculatingSupplyInternal(ctx context.Context, tsk types.TipSetKey) (api.CirculatingSupply, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return api.CirculatingSupply{}, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return api.CirculatingSupply{}, err
	}
	return gw.target.StateVMCirculatingSupplyInternal(ctx, tsk)
}

func (gw *Node) WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *crypto.Signature) (bool, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return false, err
	}
	return sigs.Verify(sig, k, msg) == nil, nil
}

func (gw *Node) WalletBalance(ctx context.Context, k address.Address) (types.BigInt, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return types.BigInt{}, err
	}
	return gw.target.WalletBalance(ctx, k)
}

func (gw *Node) StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateGetAllocationForPendingDeal(ctx, dealId, tsk)
}

func (gw *Node) StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateGetAllocation(ctx, clientAddr, allocationId, tsk)
}

func (gw *Node) StateGetAllocations(ctx context.Context, clientAddr address.Address, tsk types.TipSetKey) (map[verifregtypes.AllocationId]verifregtypes.Allocation, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateGetAllocations(ctx, clientAddr, tsk)
}

func (gw *Node) StateGetClaim(ctx context.Context, providerAddr address.Address, claimId verifregtypes.ClaimId, tsk types.TipSetKey) (*verifregtypes.Claim, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateGetClaim(ctx, providerAddr, claimId, tsk)
}

func (gw *Node) StateGetClaims(ctx context.Context, providerAddr address.Address, tsk types.TipSetKey) (map[verifregtypes.ClaimId]verifregtypes.Claim, error) {
	if err := gw.limit(ctx, stateRateLimitTokens); err != nil {
		return nil, err
	}
	if err := gw.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return gw.target.StateGetClaims(ctx, providerAddr, tsk)
}
