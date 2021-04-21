package main

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/ipfs/go-cid"
)

const (
	LookbackCap            = time.Hour * 24
	StateWaitLookbackLimit = abi.ChainEpoch(20)
)

var (
	ErrLookbackTooLong = fmt.Errorf("lookbacks of more than %s are disallowed", LookbackCap)
)

// gatewayDepsAPI defines the API methods that the GatewayAPI depends on
// (to make it easy to mock for tests)
type gatewayDepsAPI interface {
	Version(context.Context) (api.APIVersion, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error)
	ChainGetNode(ctx context.Context, p string) (*api.IpldObject, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*api.HeadChange, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
	MpoolPushUntrusted(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error)
	MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
	MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
	MsigGetPending(ctx context.Context, addr address.Address, ts types.TipSetKey) ([]*api.MsigTransaction, error)
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error)
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error)
	StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error)
	StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error)
	StateNetworkVersion(context.Context, types.TipSetKey) (network.Version, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*api.ActorState, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (*api.MinerPower, error)
	StateMinerFaults(context.Context, address.Address, types.TipSetKey) (bitfield.BitField, error)
	StateMinerRecoveries(context.Context, address.Address, types.TipSetKey) (bitfield.BitField, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (miner.MinerInfo, error)
	StateMinerDeadlines(context.Context, address.Address, types.TipSetKey) ([]api.Deadline, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (types.BigInt, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	StateCirculatingSupply(context.Context, types.TipSetKey) (abi.TokenAmount, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error)
	StateVMCirculatingSupplyInternal(context.Context, types.TipSetKey) (api.CirculatingSupply, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error) //perm:read
}

var _ gatewayDepsAPI = *new(api.FullNode) // gateway depends on latest

type GatewayAPI struct {
	api                    gatewayDepsAPI
	lookbackCap            time.Duration
	stateWaitLookbackLimit abi.ChainEpoch
}

// NewGatewayAPI creates a new GatewayAPI with the default lookback cap
func NewGatewayAPI(api gatewayDepsAPI) *GatewayAPI {
	return newGatewayAPI(api, LookbackCap, StateWaitLookbackLimit)
}

// used by the tests
func newGatewayAPI(api gatewayDepsAPI, lookbackCap time.Duration, stateWaitLookbackLimit abi.ChainEpoch) *GatewayAPI {
	return &GatewayAPI{api: api, lookbackCap: lookbackCap, stateWaitLookbackLimit: stateWaitLookbackLimit}
}

func (a *GatewayAPI) checkTipsetKey(ctx context.Context, tsk types.TipSetKey) error {
	if tsk.IsEmpty() {
		return nil
	}

	ts, err := a.api.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return err
	}

	return a.checkTipset(ts)
}

func (a *GatewayAPI) checkTipset(ts *types.TipSet) error {
	at := time.Unix(int64(ts.Blocks()[0].Timestamp), 0)
	if err := a.checkTimestamp(at); err != nil {
		return fmt.Errorf("bad tipset: %w", err)
	}
	return nil
}

func (a *GatewayAPI) checkTipsetHeight(ts *types.TipSet, h abi.ChainEpoch) error {
	tsBlock := ts.Blocks()[0]
	heightDelta := time.Duration(uint64(tsBlock.Height-h)*build.BlockDelaySecs) * time.Second
	timeAtHeight := time.Unix(int64(tsBlock.Timestamp), 0).Add(-heightDelta)

	if err := a.checkTimestamp(timeAtHeight); err != nil {
		return fmt.Errorf("bad tipset height: %w", err)
	}
	return nil
}

func (a *GatewayAPI) checkTimestamp(at time.Time) error {
	if time.Since(at) > a.lookbackCap {
		return ErrLookbackTooLong
	}

	return nil
}

func (a *GatewayAPI) Version(ctx context.Context) (api.APIVersion, error) {
	return a.api.Version(ctx)
}

func (a *GatewayAPI) ChainGetBlockMessages(ctx context.Context, c cid.Cid) (*api.BlockMessages, error) {
	return a.api.ChainGetBlockMessages(ctx, c)
}

func (a *GatewayAPI) ChainHasObj(ctx context.Context, c cid.Cid) (bool, error) {
	return a.api.ChainHasObj(ctx, c)
}

func (a *GatewayAPI) ChainHead(ctx context.Context) (*types.TipSet, error) {
	// TODO: cache and invalidate cache when timestamp is up (or have internal ChainNotify)

	return a.api.ChainHead(ctx)
}

func (a *GatewayAPI) ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error) {
	return a.api.ChainGetMessage(ctx, mc)
}

func (a *GatewayAPI) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return a.api.ChainGetTipSet(ctx, tsk)
}

func (a *GatewayAPI) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	var ts *types.TipSet
	if tsk.IsEmpty() {
		head, err := a.api.ChainHead(ctx)
		if err != nil {
			return nil, err
		}
		ts = head
	} else {
		gts, err := a.api.ChainGetTipSet(ctx, tsk)
		if err != nil {
			return nil, err
		}
		ts = gts
	}

	// Check if the tipset key refers to a tipset that's too far in the past
	if err := a.checkTipset(ts); err != nil {
		return nil, err
	}

	// Check if the height is too far in the past
	if err := a.checkTipsetHeight(ts, h); err != nil {
		return nil, err
	}

	return a.api.ChainGetTipSetByHeight(ctx, h, tsk)
}

func (a *GatewayAPI) ChainGetNode(ctx context.Context, p string) (*api.IpldObject, error) {
	return a.api.ChainGetNode(ctx, p)
}

func (a *GatewayAPI) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	return a.api.ChainNotify(ctx)
}

func (a *GatewayAPI) ChainReadObj(ctx context.Context, c cid.Cid) ([]byte, error) {
	return a.api.ChainReadObj(ctx, c)
}

func (a *GatewayAPI) GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}

	return a.api.GasEstimateMessageGas(ctx, msg, spec, tsk)
}

func (a *GatewayAPI) MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error) {
	// TODO: additional anti-spam checks
	return a.api.MpoolPushUntrusted(ctx, sm)
}

func (a *GatewayAPI) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return types.NewInt(0), err
	}

	return a.api.MsigGetAvailableBalance(ctx, addr, tsk)
}

func (a *GatewayAPI) MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error) {
	if err := a.checkTipsetKey(ctx, start); err != nil {
		return types.NewInt(0), err
	}
	if err := a.checkTipsetKey(ctx, end); err != nil {
		return types.NewInt(0), err
	}

	return a.api.MsigGetVested(ctx, addr, start, end)
}

func (a *GatewayAPI) MsigGetPending(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*api.MsigTransaction, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}

	return a.api.MsigGetPending(ctx, addr, tsk)
}

func (a *GatewayAPI) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return address.Undef, err
	}

	return a.api.StateAccountKey(ctx, addr, tsk)
}

func (a *GatewayAPI) StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return api.DealCollateralBounds{}, err
	}

	return a.api.StateDealProviderCollateralBounds(ctx, size, verified, tsk)
}

func (a *GatewayAPI) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}

	return a.api.StateGetActor(ctx, actor, tsk)
}

func (a *GatewayAPI) StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}

	return a.api.StateListMiners(ctx, tsk)
}

func (a *GatewayAPI) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return address.Undef, err
	}

	return a.api.StateLookupID(ctx, addr, tsk)
}

func (a *GatewayAPI) StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return api.MarketBalance{}, err
	}

	return a.api.StateMarketBalance(ctx, addr, tsk)
}

func (a *GatewayAPI) StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}

	return a.api.StateMarketStorageDeal(ctx, dealId, tsk)
}

func (a *GatewayAPI) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return network.VersionMax, err
	}

	return a.api.StateNetworkVersion(ctx, tsk)
}

func (a *GatewayAPI) StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	if limit == api.LookbackNoLimit {
		limit = a.stateWaitLookbackLimit
	}
	if a.stateWaitLookbackLimit != api.LookbackNoLimit && limit > a.stateWaitLookbackLimit {
		limit = a.stateWaitLookbackLimit
	}
	if err := a.checkTipsetKey(ctx, from); err != nil {
		return nil, err
	}

	return a.api.StateSearchMsg(ctx, from, msg, limit, allowReplaced)
}

func (a *GatewayAPI) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	if limit == api.LookbackNoLimit {
		limit = a.stateWaitLookbackLimit
	}
	if a.stateWaitLookbackLimit != api.LookbackNoLimit && limit > a.stateWaitLookbackLimit {
		limit = a.stateWaitLookbackLimit
	}

	return a.api.StateWaitMsg(ctx, msg, confidence, limit, allowReplaced)
}

func (a *GatewayAPI) StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*api.ActorState, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return a.api.StateReadState(ctx, actor, tsk)
}

func (a *GatewayAPI) StateMinerPower(ctx context.Context, m address.Address, tsk types.TipSetKey) (*api.MinerPower, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return a.api.StateMinerPower(ctx, m, tsk)
}

func (a *GatewayAPI) StateMinerFaults(ctx context.Context, m address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return bitfield.BitField{}, err
	}
	return a.api.StateMinerFaults(ctx, m, tsk)
}
func (a *GatewayAPI) StateMinerRecoveries(ctx context.Context, m address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return bitfield.BitField{}, err
	}
	return a.api.StateMinerRecoveries(ctx, m, tsk)
}

func (a *GatewayAPI) StateMinerInfo(ctx context.Context, m address.Address, tsk types.TipSetKey) (miner.MinerInfo, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return miner.MinerInfo{}, err
	}
	return a.api.StateMinerInfo(ctx, m, tsk)
}

func (a *GatewayAPI) StateMinerDeadlines(ctx context.Context, m address.Address, tsk types.TipSetKey) ([]api.Deadline, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return a.api.StateMinerDeadlines(ctx, m, tsk)
}

func (a *GatewayAPI) StateMinerAvailableBalance(ctx context.Context, m address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return types.BigInt{}, err
	}
	return a.api.StateMinerAvailableBalance(ctx, m, tsk)
}

func (a *GatewayAPI) StateMinerProvingDeadline(ctx context.Context, m address.Address, tsk types.TipSetKey) (*dline.Info, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return a.api.StateMinerProvingDeadline(ctx, m, tsk)
}

func (a *GatewayAPI) StateCirculatingSupply(ctx context.Context, tsk types.TipSetKey) (abi.TokenAmount, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return types.BigInt{}, err
	}
	return a.api.StateCirculatingSupply(ctx, tsk)
}

func (a *GatewayAPI) StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return a.api.StateSectorGetInfo(ctx, maddr, n, tsk)
}

func (a *GatewayAPI) StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}
	return a.api.StateVerifiedClientStatus(ctx, addr, tsk)
}

func (a *GatewayAPI) StateVMCirculatingSupplyInternal(ctx context.Context, tsk types.TipSetKey) (api.CirculatingSupply, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return api.CirculatingSupply{}, err
	}
	return a.api.StateVMCirculatingSupplyInternal(ctx, tsk)
}

func (a *GatewayAPI) WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *crypto.Signature) (bool, error) {
	return sigs.Verify(sig, k, msg) == nil, nil
}

func (a *GatewayAPI) WalletBalance(ctx context.Context, k address.Address) (types.BigInt, error) {
	return a.api.WalletBalance(ctx, k)
}

var _ api.Gateway = (*GatewayAPI)(nil)
var _ full.ChainModuleAPI = (*GatewayAPI)(nil)
var _ full.GasModuleAPI = (*GatewayAPI)(nil)
var _ full.MpoolModuleAPI = (*GatewayAPI)(nil)
var _ full.StateModuleAPI = (*GatewayAPI)(nil)
