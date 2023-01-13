package paychmgr

import (
	"context"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/types"
)

type stateAccessor struct {
	sm stateManagerAPI
}

func (ca *stateAccessor) loadPaychActorState(ctx context.Context, ch address.Address) (*types.Actor, paych.State, error) {
	return ca.sm.GetPaychState(ctx, ch, nil)
}

func (ca *stateAccessor) loadStateChannelInfo(ctx context.Context, ch address.Address, dir uint64) (*ChannelInfo, error) {
	_, st, err := ca.loadPaychActorState(ctx, ch)
	if err != nil {
		return nil, err
	}

	// Load channel "From" account actor state
	f, err := st.From()
	if err != nil {
		return nil, err
	}
	from, err := ca.sm.ResolveToDeterministicAddress(ctx, f, nil)
	if err != nil {
		return nil, err
	}
	t, err := st.To()
	if err != nil {
		return nil, err
	}
	to, err := ca.sm.ResolveToDeterministicAddress(ctx, t, nil)
	if err != nil {
		return nil, err
	}

	nextLane, err := ca.nextLaneFromState(ctx, st)
	if err != nil {
		return nil, err
	}

	ci := &ChannelInfo{
		Channel:   &ch,
		Direction: dir,
		NextLane:  nextLane,
	}

	if dir == DirOutbound {
		ci.Control = from
		ci.Target = to
	} else {
		ci.Control = to
		ci.Target = from
	}

	return ci, nil
}

func (ca *stateAccessor) nextLaneFromState(ctx context.Context, st paych.State) (uint64, error) {
	laneCount, err := st.LaneCount()
	if err != nil {
		return 0, err
	}
	if laneCount == 0 {
		return 0, nil
	}

	maxID := uint64(0)
	if err := st.ForEachLaneState(func(idx uint64, _ paych.LaneState) error {
		if idx > maxID {
			maxID = idx
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return maxID + 1, nil
}
