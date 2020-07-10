package paychmgr

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi/big"

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

// laneState gets the LaneStates from chain, then applies all vouchers in
// the data store over the chain state
func (pm *Manager) laneState(state *paych.State, ch address.Address) (map[uint64]*paych.LaneState, error) {
	// TODO: we probably want to call UpdateChannelState with all vouchers to be fully correct
	//  (but technically dont't need to)
	laneStates := make(map[uint64]*paych.LaneState, len(state.LaneStates))

	// Get the lane state from the chain
	for _, laneState := range state.LaneStates {
		laneStates[laneState.ID] = laneState
	}

	// Apply locally stored vouchers
	vouchers, err := pm.store.VouchersForPaych(ch)
	if err != nil && err != ErrChannelNotTracked {
		return nil, err
	}

	for _, v := range vouchers {
		for range v.Voucher.Merges {
			return nil, xerrors.Errorf("paych merges not handled yet")
		}

		// If there's a voucher for a lane that isn't in chain state just
		// create it
		ls, ok := laneStates[v.Voucher.Lane]
		if !ok {
			ls = &paych.LaneState{
				ID:       v.Voucher.Lane,
				Redeemed: types.NewInt(0),
				Nonce:    0,
			}
			laneStates[v.Voucher.Lane] = ls
		}

		if v.Voucher.Nonce < ls.Nonce {
			continue
		}

		ls.Nonce = v.Voucher.Nonce
		ls.Redeemed = v.Voucher.Amount
	}

	return laneStates, nil
}

// Get the total redeemed amount across all lanes, after applying the voucher
func (pm *Manager) totalRedeemedWithVoucher(laneStates map[uint64]*paych.LaneState, sv *paych.SignedVoucher) (big.Int, error) {
	// TODO: merges
	if len(sv.Merges) != 0 {
		return big.Int{}, xerrors.Errorf("dont currently support paych lane merges")
	}

	total := big.NewInt(0)
	for _, ls := range laneStates {
		total = big.Add(total, ls.Redeemed)
	}

	lane, ok := laneStates[sv.Lane]
	if ok {
		// If the voucher is for an existing lane, and the voucher nonce
		// and is higher than the lane nonce
		if sv.Nonce > lane.Nonce {
			// Add the delta between the redeemed amount and the voucher
			// amount to the total
			delta := big.Sub(sv.Amount, lane.Redeemed)
			total = big.Add(total, delta)
		}
	} else {
		// If the voucher is *not* for an existing lane, just add its
		// value (implicitly a new lane will be created for the voucher)
		total = big.Add(total, sv.Amount)
	}

	return total, nil
}
