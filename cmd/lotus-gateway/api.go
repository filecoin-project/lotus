package main

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"

	"go.opencensus.io/trace"
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

func (a *GatewayAPI) StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error) {
	ctx, span := trace.StartSpan(ctx, "StateGetActor")
	defer span.End()

	when, err := a.getTipsetTimestamp(ctx, ts)
	if err != nil {
		return nil, err
	}

	if time.Since(when) > time.Hour {
		return nil, ErrLookbackTooLong
	}

	return a.api.StateGetActor(ctx, actor, ts)
}
