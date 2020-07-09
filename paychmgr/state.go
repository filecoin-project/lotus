package paychmgr

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/builtin/account"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

func (pm *Manager) loadPaychState(ctx context.Context, ch address.Address) (*types.Actor, *paych.State, error) {
	var pcast paych.State
	act, err := pm.sm.LoadActorState(ctx, ch, &pcast, nil)
	if err != nil {
		return nil, nil, err
	}

	return act, &pcast, nil
}

func (pm *Manager) loadStateChannelInfo(ctx context.Context, ch address.Address, dir uint64) (*ChannelInfo, error) {
	_, st, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return nil, err
	}

	var account account.State
	_, err = pm.sm.LoadActorState(ctx, st.From, &account, nil)
	if err != nil {
		return nil, err
	}
	from := account.Address
	_, err = pm.sm.LoadActorState(ctx, st.To, &account, nil)
	if err != nil {
		return nil, err
	}
	to := account.Address

	ci := &ChannelInfo{
		Channel:   ch,
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

func findLane(states []*paych.LaneState, lane uint64) *paych.LaneState {
	var ls *paych.LaneState
	for _, laneState := range states {
		if laneState.ID == lane {
			ls = laneState
			break
		}
	}
	return ls
}

func (pm *Manager) laneState(state *paych.State, ch address.Address, lane uint64) (paych.LaneState, error) {
	// TODO: we probably want to call UpdateChannelState with all vouchers to be fully correct
	//  (but technically dont't need to)
	// TODO: make sure this is correct

	// Get the lane state from the chain
	ls := findLane(state.LaneStates, lane)
	if ls == nil {
		ls = &paych.LaneState{
			ID:       lane,
			Redeemed: types.NewInt(0),
			Nonce:    0,
		}
	}

	// Apply locally stored vouchers
	vouchers, err := pm.store.VouchersForPaych(ch)
	if err != nil {
		if err == ErrChannelNotTracked {
			return *ls, nil
		}
		return paych.LaneState{}, err
	}

	for _, v := range vouchers {
		for range v.Voucher.Merges {
			return paych.LaneState{}, xerrors.Errorf("paych merges not handled yet")
		}

		if v.Voucher.Lane != lane {
			continue
		}

		if v.Voucher.Nonce < ls.Nonce {
			log.Warnf("Found outdated voucher: ch=%s, lane=%d, v.nonce=%d lane.nonce=%d", ch, lane, v.Voucher.Nonce, ls.Nonce)
			continue
		}

		ls.Nonce = v.Voucher.Nonce
		ls.Redeemed = v.Voucher.Amount
	}

	return *ls, nil
}
