package main

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/ipfs/go-cid"
)

const (
	LookbackCap            = time.Hour
	stateWaitLookbackLimit = abi.ChainEpoch(20)
)

var (
	ErrLookbackTooLong = fmt.Errorf("lookbacks of more than %s are disallowed", LookbackCap)
)

// gatewayDepsAPI defines the API methods that the GatewayAPI depends on
// (to make it easy to mock for tests)
type gatewayDepsAPI interface {
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, tsk types.TipSetKey) (*types.Message, error)
	MpoolPushUntrusted(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error)
	MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
	MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateWaitMsgLimited(ctx context.Context, msg cid.Cid, confidence uint64, h abi.ChainEpoch) (*api.MsgLookup, error)
}

type GatewayAPI struct {
	api gatewayDepsAPI
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
	if time.Since(at) > LookbackCap {
		return ErrLookbackTooLong
	}

	return nil
}

func (a *GatewayAPI) ChainHead(ctx context.Context) (*types.TipSet, error) {
	// TODO: cache and invalidate cache when timestamp is up (or have internal ChainNotify)

	return a.api.ChainHead(ctx)
}

func (a *GatewayAPI) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return a.api.ChainGetTipSet(ctx, tsk)
}

func (a *GatewayAPI) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	ts, err := a.api.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return nil, err
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

func (a *GatewayAPI) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return address.Undef, err
	}

	return a.api.StateAccountKey(ctx, addr, tsk)
}

func (a *GatewayAPI) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return nil, err
	}

	return a.api.StateGetActor(ctx, actor, tsk)
}

func (a *GatewayAPI) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := a.checkTipsetKey(ctx, tsk); err != nil {
		return address.Undef, err
	}

	return a.api.StateLookupID(ctx, addr, tsk)
}

func (a *GatewayAPI) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	return a.api.StateWaitMsgLimited(ctx, msg, confidence, stateWaitLookbackLimit)
}

var _ api.GatewayAPI = (*GatewayAPI)(nil)
var _ full.ChainModuleAPI = (*GatewayAPI)(nil)
var _ full.GasModuleAPI = (*GatewayAPI)(nil)
var _ full.MpoolModuleAPI = (*GatewayAPI)(nil)
var _ full.StateModuleAPI = (*GatewayAPI)(nil)
