package full

import (
	"context"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types"
)

var _ StateModuleAPIv2 = (*StateModuleV2)(nil)

type StateModuleAPIv2 interface {
	StateGetActor(context.Context, address.Address, types.TipSetSelector) (*types.Actor, error)
	StateGetID(context.Context, address.Address, types.TipSetSelector) (*address.Address, error)
}

type StateModuleV2 struct {
	State StateAPI
	Chain ChainModuleV2

	fx.In
}

// TODO: Discussion: do we want to optionally strict the type of selectors that
//       can be supplied here to avoid foot-guns? For example, only accept TipSetKey
//       using a soft condition check here to avoid potential foot-gun but also make it
//       easy to expand in the future without breaking changes if users asked for it.

func (s *StateModuleV2) StateGetActor(ctx context.Context, addr address.Address, selector types.TipSetSelector) (*types.Actor, error) {
	ts, err := s.Chain.ChainGetTipSet(ctx, selector)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset: %w", err)
	}
	return s.State.StateGetActor(ctx, addr, ts.Key())
}

func (s *StateModuleV2) StateGetID(ctx context.Context, addr address.Address, selector types.TipSetSelector) (*address.Address, error) {
	ts, err := s.Chain.ChainGetTipSet(ctx, selector)
	if err != nil {
		return nil, xerrors.Errorf("getting tipset: %w", err)
	}
	id, err := s.State.StateLookupID(ctx, addr, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("looking up ID: %w", err)
	}
	return &id, nil
}
