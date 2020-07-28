package paychmgr

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/builtin/account"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/lotus/chain/types"
)

type stateAccessor struct {
	sm StateManagerApi
}

func (ca *stateAccessor) loadPaychState(ctx context.Context, ch address.Address) (*types.Actor, *paych.State, error) {
	var pcast paych.State
	act, err := ca.sm.LoadActorState(ctx, ch, &pcast, nil)
	if err != nil {
		return nil, nil, err
	}

	return act, &pcast, nil
}

func (ca *stateAccessor) loadStateChannelInfo(ctx context.Context, ch address.Address, dir uint64) (*ChannelInfo, error) {
	_, st, err := ca.loadPaychState(ctx, ch)
	if err != nil {
		return nil, err
	}

	var account account.State
	_, err = ca.sm.LoadActorState(ctx, st.From, &account, nil)
	if err != nil {
		return nil, err
	}
	from := account.Address
	_, err = ca.sm.LoadActorState(ctx, st.To, &account, nil)
	if err != nil {
		return nil, err
	}
	to := account.Address

	ci := &ChannelInfo{
		Channel:   &ch,
		Direction: dir,
		NextLane:  nextLaneFromState(st),
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

func nextLaneFromState(st *paych.State) uint64 {
	if len(st.LaneStates) == 0 {
		return 0
	}

	maxLane := st.LaneStates[0].ID
	for _, state := range st.LaneStates {
		if state.ID > maxLane {
			maxLane = state.ID
		}
	}
	return maxLane + 1
}
