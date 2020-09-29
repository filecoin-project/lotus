package api

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

type GatewayAPI interface {
	StateGetActor(ctx context.Context, actor address.Address, ts types.TipSetKey) (*types.Actor, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)
	ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	MpoolPush(ctx context.Context, sm *types.SignedMessage) (cid.Cid, error)
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
}
