package paych

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	xerrors "golang.org/x/xerrors"
)

func (pm *Manager) loadPaychState(ctx context.Context, ch address.Address) (*types.Actor, *actors.PaymentChannelActorState, error) {
	var pcast actors.PaymentChannelActorState
	act, err := pm.sm.LoadActorState(ctx, ch, &pcast, nil)
	if err != nil {
		return nil, nil, err
	}

	return act, &pcast, nil
}

func (pm *Manager) laneState(ctx context.Context, ch address.Address, lane uint64) (actors.LaneState, error) {
	_, state, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return actors.LaneState{}, err
	}

	// TODO: we probably want to call UpdateChannelState with all vouchers to be fully correct
	//  (but technically dont't need to)
	// TODO: make sure this is correct

	ls, ok := state.LaneStates[fmt.Sprintf("%d", lane)]
	if !ok {
		ls = &actors.LaneState{
			Closed:   false,
			Redeemed: types.NewInt(0),
			Nonce:    0,
		}
	}

	if ls.Closed {
		return *ls, nil
	}

	vouchers, err := pm.store.VouchersForPaych(ch)
	if err != nil {
		if err == ErrChannelNotTracked {
			return *ls, nil
		}
		return actors.LaneState{}, err
	}

	for _, v := range vouchers {
		for range v.Voucher.Merges {
			return actors.LaneState{}, xerrors.Errorf("paych merges not handled yet")
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
