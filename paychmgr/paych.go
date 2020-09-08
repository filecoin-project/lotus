package paychmgr

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/api"

	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"golang.org/x/xerrors"
)

// insufficientFundsErr indicates that there are not enough funds in the
// channel to create a voucher
type insufficientFundsErr interface {
	Shortfall() types.BigInt
}

type ErrInsufficientFunds struct {
	shortfall types.BigInt
}

func newErrInsufficientFunds(shortfall types.BigInt) *ErrInsufficientFunds {
	return &ErrInsufficientFunds{shortfall: shortfall}
}

func (e *ErrInsufficientFunds) Error() string {
	return fmt.Sprintf("not enough funds in channel to cover voucher - shortfall: %d", e.shortfall)
}

func (e *ErrInsufficientFunds) Shortfall() types.BigInt {
	return e.shortfall
}

// channelAccessor is used to simplify locking when accessing a channel
type channelAccessor struct {
	from address.Address
	to   address.Address

	// chctx is used by background processes (eg when waiting for things to be
	// confirmed on chain)
	chctx         context.Context
	sa            *stateAccessor
	api           managerAPI
	store         *Store
	lk            *channelLock
	fundsReqQueue []*fundsReq
	msgListeners  msgListeners
}

func newChannelAccessor(pm *Manager, from address.Address, to address.Address) *channelAccessor {
	return &channelAccessor{
		from:         from,
		to:           to,
		chctx:        pm.ctx,
		sa:           pm.sa,
		api:          pm.pchapi,
		store:        pm.store,
		lk:           &channelLock{globalLock: &pm.lk},
		msgListeners: newMsgListeners(),
	}
}

func (ca *channelAccessor) getChannelInfo(addr address.Address) (*ChannelInfo, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	return ca.store.ByAddress(addr)
}

func (ca *channelAccessor) outboundActiveByFromTo(from, to address.Address) (*ChannelInfo, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	return ca.store.OutboundActiveByFromTo(from, to)
}

// createVoucher creates a voucher with the given specification, setting its
// nonce, signing the voucher and storing it in the local datastore.
// If there are not enough funds in the channel to create the voucher, returns
// the shortfall in funds.
func (ca *channelAccessor) createVoucher(ctx context.Context, ch address.Address, voucher paych.SignedVoucher) (*api.VoucherCreateResult, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Find the channel for the voucher
	ci, err := ca.store.ByAddress(ch)
	if err != nil {
		return nil, xerrors.Errorf("failed to get channel info by address: %w", err)
	}

	// Set the voucher channel
	sv := &voucher
	sv.ChannelAddr = ch

	// Get the next nonce on the given lane
	sv.Nonce = ca.nextNonceForLane(ci, voucher.Lane)

	// Sign the voucher
	vb, err := sv.SigningBytes()
	if err != nil {
		return nil, xerrors.Errorf("failed to get voucher signing bytes: %w", err)
	}

	sig, err := ca.api.WalletSign(ctx, ci.Control, vb)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign voucher: %w", err)
	}
	sv.Signature = sig

	// Store the voucher
	if _, err := ca.addVoucherUnlocked(ctx, ch, sv, nil, types.NewInt(0)); err != nil {
		// If there are not enough funds in the channel to cover the voucher,
		// return a voucher create result with the shortfall
		var ife insufficientFundsErr
		if xerrors.As(err, &ife) {
			return &api.VoucherCreateResult{
				Shortfall: ife.Shortfall(),
			}, nil
		}

		return nil, xerrors.Errorf("failed to persist voucher: %w", err)
	}

	return &api.VoucherCreateResult{Voucher: sv, Shortfall: types.NewInt(0)}, nil
}

func (ca *channelAccessor) nextNonceForLane(ci *ChannelInfo, lane uint64) uint64 {
	var maxnonce uint64
	for _, v := range ci.Vouchers {
		if v.Voucher.Lane == lane {
			if v.Voucher.Nonce > maxnonce {
				maxnonce = v.Voucher.Nonce
			}
		}
	}

	return maxnonce + 1
}

func (ca *channelAccessor) checkVoucherValid(ctx context.Context, ch address.Address, sv *paych.SignedVoucher) (map[uint64]*paych.LaneState, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	return ca.checkVoucherValidUnlocked(ctx, ch, sv)
}

func (ca *channelAccessor) checkVoucherValidUnlocked(ctx context.Context, ch address.Address, sv *paych.SignedVoucher) (map[uint64]*paych.LaneState, error) {
	if sv.ChannelAddr != ch {
		return nil, xerrors.Errorf("voucher ChannelAddr doesn't match channel address, got %s, expected %s", sv.ChannelAddr, ch)
	}

	// Load payment channel actor state
	act, pchState, err := ca.sa.loadPaychActorState(ctx, ch)
	if err != nil {
		return nil, err
	}

	// Load channel "From" account actor state
	var actState account.State
	_, err = ca.api.LoadActorState(ctx, pchState.From, &actState, nil)
	if err != nil {
		return nil, err
	}
	from := actState.Address

	// verify voucher signature
	vb, err := sv.SigningBytes()
	if err != nil {
		return nil, err
	}

	// TODO: technically, either party may create and sign a voucher.
	// However, for now, we only accept them from the channel creator.
	// More complex handling logic can be added later
	if err := sigs.Verify(sv.Signature, from, vb); err != nil {
		return nil, err
	}

	// Check the voucher against the highest known voucher nonce / value
	laneStates, err := ca.laneState(ctx, pchState, ch)
	if err != nil {
		return nil, err
	}

	// If the new voucher nonce value is less than the highest known
	// nonce for the lane
	ls, lsExists := laneStates[sv.Lane]
	if lsExists && sv.Nonce <= ls.Nonce {
		return nil, fmt.Errorf("nonce too low")
	}

	// If the voucher amount is less than the highest known voucher amount
	if lsExists && sv.Amount.LessThanEqual(ls.Redeemed) {
		return nil, fmt.Errorf("voucher amount is lower than amount for voucher with lower nonce")
	}

	// Total redeemed is the total redeemed amount for all lanes, including
	// the new voucher
	// eg
	//
	// lane 1 redeemed:            3
	// lane 2 redeemed:            2
	// voucher for lane 1:         5
	//
	// Voucher supersedes lane 1 redeemed, therefore
	// effective lane 1 redeemed:  5
	//
	// lane 1:  5
	// lane 2:  2
	//          -
	// total:   7
	totalRedeemed, err := ca.totalRedeemedWithVoucher(laneStates, sv)
	if err != nil {
		return nil, err
	}

	// Total required balance = total redeemed + toSend
	// Must not exceed actor balance
	newTotal := types.BigAdd(totalRedeemed, pchState.ToSend)
	if act.Balance.LessThan(newTotal) {
		return nil, newErrInsufficientFunds(types.BigSub(newTotal, act.Balance))
	}

	if len(sv.Merges) != 0 {
		return nil, fmt.Errorf("dont currently support paych lane merges")
	}

	return laneStates, nil
}

func (ca *channelAccessor) checkVoucherSpendable(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	recipient, err := ca.getPaychRecipient(ctx, ch)
	if err != nil {
		return false, err
	}

	ci, err := ca.store.ByAddress(ch)
	if err != nil {
		return false, err
	}

	// Check if voucher has already been submitted
	submitted, err := ci.wasVoucherSubmitted(sv)
	if err != nil {
		return false, err
	}
	if submitted {
		return false, nil
	}

	// If proof is needed and wasn't supplied as a parameter, get it from the
	// datastore
	if sv.Extra != nil && proof == nil {
		vi, err := ci.infoForVoucher(sv)
		if err != nil {
			return false, err
		}

		if vi.Proof != nil {
			log.Info("CheckVoucherSpendable: using stored proof")
			proof = vi.Proof
		} else {
			log.Warn("CheckVoucherSpendable: nil proof for voucher with validation")
		}
	}

	enc, err := actors.SerializeParams(&paych.UpdateChannelStateParams{
		Sv:     *sv,
		Secret: secret,
		Proof:  proof,
	})
	if err != nil {
		return false, err
	}

	ret, err := ca.api.Call(ctx, &types.Message{
		From:   recipient,
		To:     ch,
		Method: builtin.MethodsPaych.UpdateChannelState,
		Params: enc,
	}, nil)
	if err != nil {
		return false, err
	}

	if ret.MsgRct.ExitCode != 0 {
		return false, nil
	}

	return true, nil
}

func (ca *channelAccessor) getPaychRecipient(ctx context.Context, ch address.Address) (address.Address, error) {
	var state paych.State
	if _, err := ca.api.LoadActorState(ctx, ch, &state, nil); err != nil {
		return address.Address{}, err
	}

	return state.To, nil
}

func (ca *channelAccessor) addVoucher(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	return ca.addVoucherUnlocked(ctx, ch, sv, proof, minDelta)
}

func (ca *channelAccessor) addVoucherUnlocked(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	ci, err := ca.store.ByAddress(ch)
	if err != nil {
		return types.BigInt{}, err
	}

	// Check if the voucher has already been added
	for i, v := range ci.Vouchers {
		eq, err := cborutil.Equals(sv, v.Voucher)
		if err != nil {
			return types.BigInt{}, err
		}
		if !eq {
			continue
		}

		// This is a duplicate voucher.
		// Update the proof on the existing voucher
		if len(proof) > 0 && !bytes.Equal(v.Proof, proof) {
			log.Warnf("AddVoucher: adding proof to stored voucher")
			ci.Vouchers[i] = &VoucherInfo{
				Voucher: v.Voucher,
				Proof:   proof,
			}

			return types.NewInt(0), ca.store.putChannelInfo(ci)
		}

		// Otherwise just ignore the duplicate voucher
		log.Warnf("AddVoucher: voucher re-added with matching proof")
		return types.NewInt(0), nil
	}

	// Check voucher validity
	laneStates, err := ca.checkVoucherValidUnlocked(ctx, ch, sv)
	if err != nil {
		return types.NewInt(0), err
	}

	// The change in value is the delta between the voucher amount and
	// the highest previous voucher amount for the lane
	laneState, exists := laneStates[sv.Lane]
	redeemed := big.NewInt(0)
	if exists {
		redeemed = laneState.Redeemed
	}

	delta := types.BigSub(sv.Amount, redeemed)
	if minDelta.GreaterThan(delta) {
		return delta, xerrors.Errorf("addVoucher: supplied token amount too low; minD=%s, D=%s; laneAmt=%s; v.Amt=%s", minDelta, delta, redeemed, sv.Amount)
	}

	ci.Vouchers = append(ci.Vouchers, &VoucherInfo{
		Voucher: sv,
		Proof:   proof,
	})

	if ci.NextLane <= sv.Lane {
		ci.NextLane = sv.Lane + 1
	}

	return delta, ca.store.putChannelInfo(ci)
}

func (ca *channelAccessor) submitVoucher(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (cid.Cid, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	ci, err := ca.store.ByAddress(ch)
	if err != nil {
		return cid.Undef, err
	}

	// If voucher needs proof, and none was supplied, check datastore for proof
	if sv.Extra != nil && proof == nil {
		vi, err := ci.infoForVoucher(sv)
		if err != nil {
			return cid.Undef, err
		}

		if vi.Proof != nil {
			log.Info("SubmitVoucher: using stored proof")
			proof = vi.Proof
		} else {
			log.Warn("SubmitVoucher: nil proof for voucher with validation")
		}
	}

	has, err := ci.hasVoucher(sv)
	if err != nil {
		return cid.Undef, err
	}

	// If the channel has the voucher
	if has {
		// Check that the voucher hasn't already been submitted
		submitted, err := ci.wasVoucherSubmitted(sv)
		if err != nil {
			return cid.Undef, err
		}
		if submitted {
			return cid.Undef, xerrors.Errorf("cannot submit voucher that has already been submitted")
		}
	}

	enc, err := actors.SerializeParams(&paych.UpdateChannelStateParams{
		Sv:     *sv,
		Secret: secret,
		Proof:  proof,
	})
	if err != nil {
		return cid.Undef, err
	}

	msg := &types.Message{
		From:   ci.Control,
		To:     ch,
		Value:  types.NewInt(0),
		Method: builtin.MethodsPaych.UpdateChannelState,
		Params: enc,
	}

	smsg, err := ca.api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, err
	}

	// If the channel didn't already have the voucher
	if !has {
		// Add the voucher to the channel
		ci.Vouchers = append(ci.Vouchers, &VoucherInfo{
			Voucher: sv,
			Proof:   proof,
		})
	}

	// Mark the voucher and any lower-nonce vouchers as having been submitted
	err = ca.store.MarkVoucherSubmitted(ci, sv)
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

func (ca *channelAccessor) allocateLane(ch address.Address) (uint64, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	// TODO: should this take into account lane state?
	return ca.store.AllocateLane(ch)
}

func (ca *channelAccessor) listVouchers(ctx context.Context, ch address.Address) ([]*VoucherInfo, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	// TODO: just having a passthrough method like this feels odd. Seems like
	// there should be some filtering we're doing here
	return ca.store.VouchersForPaych(ch)
}

// laneState gets the LaneStates from chain, then applies all vouchers in
// the data store over the chain state
func (ca *channelAccessor) laneState(ctx context.Context, state *paych.State, ch address.Address) (map[uint64]*paych.LaneState, error) {
	// TODO: we probably want to call UpdateChannelState with all vouchers to be fully correct
	//  (but technically dont't need to)

	// Get the lane state from the chain
	store := ca.api.AdtStore(ctx)
	lsamt, err := adt.AsArray(store, state.LaneStates)
	if err != nil {
		return nil, err
	}

	// Note: we use a map instead of an array to store laneStates because the
	// client sets the lane ID (the index) and potentially they could use a
	// very large index.
	var ls paych.LaneState
	laneStates := make(map[uint64]*paych.LaneState, lsamt.Length())
	err = lsamt.ForEach(&ls, func(i int64) error {
		current := ls
		laneStates[uint64(i)] = &current
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Apply locally stored vouchers
	vouchers, err := ca.store.VouchersForPaych(ch)
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
func (ca *channelAccessor) totalRedeemedWithVoucher(laneStates map[uint64]*paych.LaneState, sv *paych.SignedVoucher) (big.Int, error) {
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
		// is higher than the lane nonce
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

func (ca *channelAccessor) settle(ctx context.Context, ch address.Address) (cid.Cid, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	ci, err := ca.store.ByAddress(ch)
	if err != nil {
		return cid.Undef, err
	}

	msg := &types.Message{
		To:     ch,
		From:   ci.Control,
		Value:  types.NewInt(0),
		Method: builtin.MethodsPaych.Settle,
	}
	smgs, err := ca.api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, err
	}

	ci.Settling = true
	err = ca.store.putChannelInfo(ci)
	if err != nil {
		log.Errorf("Error marking channel as settled: %s", err)
	}

	return smgs.Cid(), err
}

func (ca *channelAccessor) collect(ctx context.Context, ch address.Address) (cid.Cid, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	ci, err := ca.store.ByAddress(ch)
	if err != nil {
		return cid.Undef, err
	}

	msg := &types.Message{
		To:     ch,
		From:   ci.Control,
		Value:  types.NewInt(0),
		Method: builtin.MethodsPaych.Collect,
	}

	smsg, err := ca.api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}
