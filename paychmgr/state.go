package paychmgr

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/specs-actors/actors/builtin/account"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/lotus/chain/types"
)

type stateAccessor struct {
	sm stateManagerAPI
}

func (ca *stateAccessor) loadPaychActorState(ctx context.Context, ch address.Address) (*types.Actor, *paych.State, error) {
	var pcast paych.State
	act, err := ca.sm.LoadActorState(ctx, ch, &pcast, nil)
	if err != nil {
		return nil, nil, err
	}

	return act, &pcast, nil
}

func (ca *stateAccessor) loadStateChannelInfo(ctx context.Context, ch address.Address, dir uint64) (*ChannelInfo, error) {
	_, st, err := ca.loadPaychActorState(ctx, ch)
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

func (ca *stateAccessor) nextLaneFromState(ctx context.Context, st *paych.State) (uint64, error) {
	store := ca.sm.AdtStore(ctx)
	laneStates, err := adt.AsArray(store, st.LaneStates)
	if err != nil {
		return 0, err
	}
	if laneStates.Length() == 0 {
		return 0, nil
	}

	maxID := int64(0)
	if err := laneStates.ForEach(nil, func(i int64) error {
		if i > maxID {
			maxID = i
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return uint64(maxID + 1), nil
}
