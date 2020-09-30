package modules

import (
	"context"

	"github.com/filecoin-project/lotus/api"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
)

type RPCStateManager struct {
	gapi api.GatewayAPI
}

func NewRPCStateManager(api api.GatewayAPI) *RPCStateManager {
	return &RPCStateManager{gapi: api}
}

func (s *RPCStateManager) LoadActorTsk(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	return s.gapi.StateGetActor(ctx, addr, tsk)
}

func (s *RPCStateManager) LookupID(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return s.gapi.StateLookupID(ctx, addr, ts.Key())
}

func (s *RPCStateManager) ResolveToKeyAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	return s.gapi.StateAccountKey(ctx, addr, ts.Key())
}

var _ stmgr.StateManagerAPI = (*RPCStateManager)(nil)
