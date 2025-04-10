package full

import (
	"context"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ StateModuleAPIv2 = (*StateModuleV2)(nil)

type StateModuleAPIv2 interface {
	StateGetActor(context.Context, address.Address, types.TipSetSelector) (*types.Actor, error)
	StateGetID(context.Context, address.Address, types.TipSetSelector) (*address.Address, error)
	StateCompute(context.Context, []*types.Message, types.TipSetSelector) (*api.ComputeStateOutput, error)
	StateSimulate(context.Context, []*types.Message, types.TipSetSelector, types.TipSetLimit) (*api.ComputeStateOutput, error)
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

func (a *StateModuleV2) StateCompute(ctx context.Context, msgs []*types.Message, selector types.TipSetSelector) (*api.ComputeStateOutput, error) {
	ts, err := a.Chain.ChainGetTipSet(ctx, selector)
	if err != nil {
		return nil, xerrors.Errorf("selecting tipset: %w", err)
	}

	return a.State.StateCompute(ctx, ts.Height(), msgs, ts.Key())
}

func (a *StateModuleV2) StateSimulate(ctx context.Context, msgs []*types.Message, selector types.TipSetSelector, limit types.TipSetLimit) (*api.ComputeStateOutput, error) {
	if err := limit.Validate(); err != nil {
		return nil, xerrors.Errorf("validating tipset limit: %w", err)
	}

	if limit == types.TipSetLimits.Unlimited {
		return nil, xerrors.Errorf("simluating state: tipset limit cannot be unlimited")
	}
	// TODO: Add upper-bound limit to how far of simulation is acceptable?

	ts, err := a.Chain.ChainGetTipSet(ctx, selector)
	if err != nil {
		return nil, xerrors.Errorf("selecting tipset: %w", err)
	}
	targetHeight := limit.HeightRelativeTo(ts.Height())
	if ts.Height() > targetHeight {
		return nil, xerrors.Errorf("tipset height %d is less than requested height at: %d", ts.Height(), limit)
	}
	return a.State.StateCompute(ctx, targetHeight, msgs, ts.Key())
}
