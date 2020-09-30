package main

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

const LookbackCap = time.Hour

var (
	ErrLookbackTooLong = fmt.Errorf("lookbacks of more than %s are disallowed", LookbackCap)
)

type GatewayAPI struct {
	api api.FullNode
}

func (a *GatewayAPI) getTipsetTimestamp(ctx context.Context, tsk types.TipSetKey) (time.Time, error) {
	if tsk.IsEmpty() {
		return time.Now(), nil
	}

	ts, err := a.api.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(int64(ts.Blocks()[0].Timestamp), 0), nil
}

func (a *GatewayAPI) checkTipset(ctx context.Context, ts types.TipSetKey) error {
	when, err := a.getTipsetTimestamp(ctx, ts)
	if err != nil {
		return err
	}

	if time.Since(when) > time.Hour {
		return ErrLookbackTooLong
	}

	return nil
}

func (a *GatewayAPI) ChainHead(ctx context.Context) (*types.TipSet, error) {
	// TODO: cache and invalidate cache when timestamp is up (or have internal ChainNotify)

	return a.api.ChainHead(ctx)
}

func (a *GatewayAPI) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	if err := a.checkTipset(ctx, tsk); err != nil {
		return nil, fmt.Errorf("bad tipset: %w", err)
	}

	// TODO: since we're limiting lookbacks, should just cache this (could really even cache the json response bytes)
	return a.api.ChainGetTipSet(ctx, tsk)
}

func (a *GatewayAPI) MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error) {
	// TODO: additional anti-spam checks

	return a.api.MpoolPushUntrusted(ctx, sm)
}

func (a *GatewayAPI) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := a.checkTipset(ctx, tsk); err != nil {
		return address.Undef, fmt.Errorf("bad tipset: %w", err)
	}

	return a.api.StateAccountKey(ctx, addr, tsk)
}

func (a *GatewayAPI) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	if err := a.checkTipset(ctx, tsk); err != nil {
		return nil, fmt.Errorf("bad tipset: %w", err)
	}

	return a.api.StateGetActor(ctx, actor, tsk)
}

func (a *GatewayAPI) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	if err := a.checkTipset(ctx, tsk); err != nil {
		return address.Undef, fmt.Errorf("bad tipset: %w", err)
	}

	return a.api.StateLookupID(ctx, addr, tsk)
}

var _ api.GatewayAPI = &GatewayAPI{}
