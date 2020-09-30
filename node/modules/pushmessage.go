package modules

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
)

type RPCPushMessageAPI struct {
	gapi api.GatewayAPI
}

func NewRPCPushMessageAPI(api api.GatewayAPI) *RPCPushMessageAPI {
	return &RPCPushMessageAPI{gapi: api}
}

func (p *RPCPushMessageAPI) MpoolPush(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error) {
	return p.gapi.MpoolPush(ctx, smsg)
}

var _ full.PushMessageAPI = (*RPCPushMessageAPI)(nil)
