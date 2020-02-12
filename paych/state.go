package paych

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func (pm *Manager) loadPaychState(ctx context.Context, ch address.Address) (*types.Actor, *actors.PaymentChannelActorState, error) {
	var pcast actors.PaymentChannelActorState
	act, err := pm.sm.LoadActorState(ctx, ch, &pcast, nil)
	if err != nil {
		return nil, nil, err
	}

	return act, &pcast, nil
}

func findLane(states []*paych.LaneState, lane uint64) *paych.LaneState {
	var ls *paych.LaneState
	for _, laneState := range states {
		if uint64(laneState.ID) == lane {
			ls = laneState
			break
		}
	}
	return ls
}

func (pm *Manager) laneState(ctx context.Context, ch address.Address, lane uint64) (paych.LaneState, error) {
	_, state, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return paych.LaneState{}, err
	}

	// TODO: we probably want to call UpdateChannelState with all vouchers to be fully correct
	//  (but technically dont't need to)
	// TODO: make sure this is correct

	ls := findLane(state.LaneStates, lane)
	if ls == nil {
		ls = &paych.LaneState{
			ID: int64(lane),
			Redeemed: types.NewInt(0),
			Nonce:    0,
		}
	}

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

		if uint64(v.Voucher.Lane) != lane {
			continue
		}

		if uint64(v.Voucher.Nonce) < uint64(ls.Nonce) {
			log.Warnf("Found outdated voucher: ch=%s, lane=%d, v.nonce=%d lane.nonce=%d", ch, lane, v.Voucher.Nonce, ls.Nonce)
			continue
		}

		ls.Nonce = int64(v.Voucher.Nonce)
		ls.Redeemed = v.Voucher.Amount
	}

	return *ls, nil
}
