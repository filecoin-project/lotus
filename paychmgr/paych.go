package paychmgr

import (
	"bytes"
	"context"
	"fmt"
	"math"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"golang.org/x/xerrors"

	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/node/impl/full"
)

var log = logging.Logger("paych")

type ManagerApi struct {
	fx.In

	full.MpoolAPI
	full.WalletAPI
	full.StateAPI
}

type Manager struct {
	store *Store
	sm    *stmgr.StateManager

	mpool  full.MpoolAPI
	wallet full.WalletAPI
	state  full.StateAPI
}

func NewManager(sm *stmgr.StateManager, pchstore *Store, api ManagerApi) *Manager {
	return &Manager{
		store: pchstore,
		sm:    sm,

		mpool:  api.MpoolAPI,
		wallet: api.WalletAPI,
		state:  api.StateAPI,
	}
}

func maxLaneFromState(st *paych.State) (uint64, error) {
	maxLane := uint64(math.MaxInt64)
	for _, state := range st.LaneStates {
		if (state.ID)+1 > maxLane+1 {
			maxLane = state.ID
		}
	}
	return maxLane, nil
}

func (pm *Manager) TrackInboundChannel(ctx context.Context, ch address.Address) error {
	_, st, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return err
	}

	var account account.State
	_, err = pm.sm.LoadActorState(ctx, st.From, &account, nil)
	if err != nil {
		return err
	}
	from := account.Address
	_, err = pm.sm.LoadActorState(ctx, st.To, &account, nil)
	if err != nil {
		return err
	}
	to := account.Address

	maxLane, err := maxLaneFromState(st)
	if err != nil {
		return err
	}

	return pm.store.TrackChannel(&ChannelInfo{
		Channel: ch,
		Control: to,
		Target:  from,

		Direction: DirInbound,
		NextLane:  maxLane + 1,
	})
}

func (pm *Manager) loadOutboundChannelInfo(ctx context.Context, ch address.Address) (*ChannelInfo, error) {
	_, st, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return nil, err
	}

	maxLane, err := maxLaneFromState(st)
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

	return &ChannelInfo{
		Channel: ch,
		Control: from,
		Target:  to,

		Direction: DirOutbound,
		NextLane:  maxLane + 1,
	}, nil
}

func (pm *Manager) TrackOutboundChannel(ctx context.Context, ch address.Address) error {
	ci, err := pm.loadOutboundChannelInfo(ctx, ch)
	if err != nil {
		return err
	}

	return pm.store.TrackChannel(ci)
}

func (pm *Manager) ListChannels() ([]address.Address, error) {
	return pm.store.ListChannels()
}

func (pm *Manager) GetChannelInfo(addr address.Address) (*ChannelInfo, error) {
	return pm.store.getChannelInfo(addr)
}

// checks if the given voucher is valid (is or could become spendable at some point)
func (pm *Manager) CheckVoucherValid(ctx context.Context, ch address.Address, sv *paych.SignedVoucher) error {
	act, pca, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return err
	}

	var account account.State
	_, err = pm.sm.LoadActorState(ctx, pca.From, &account, nil)
	if err != nil {
		return err
	}
	from := account.Address

	// verify signature
	vb, err := sv.SigningBytes()
	if err != nil {
		return err
	}

	// TODO: technically, either party may create and sign a voucher.
	// However, for now, we only accept them from the channel creator.
	// More complex handling logic can be added later
	if err := sigs.Verify(sv.Signature, from, vb); err != nil {
		return err
	}

	sendAmount := sv.Amount

	// now check the lane state
	// TODO: should check against vouchers in our local store too
	// there might be something conflicting
	ls := findLane(pca.LaneStates, uint64(sv.Lane))
	if ls == nil {
	} else {
		if (ls.Nonce) >= sv.Nonce {
			return fmt.Errorf("nonce too low")
		}

		sendAmount = types.BigSub(sv.Amount, ls.Redeemed)
	}

	// TODO: also account for vouchers on other lanes we've received
	newTotal := types.BigAdd(sendAmount, pca.ToSend)
	if act.Balance.LessThan(newTotal) {
		return fmt.Errorf("not enough funds in channel to cover voucher")
	}

	if len(sv.Merges) != 0 {
		return fmt.Errorf("dont currently support paych lane merges")
	}

	return nil
}

// checks if the given voucher is currently spendable
func (pm *Manager) CheckVoucherSpendable(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	owner, err := pm.getPaychOwner(ctx, ch)
	if err != nil {
		return false, err
	}

	if sv.Extra != nil && proof == nil {
		known, err := pm.ListVouchers(ctx, ch)
		if err != nil {
			return false, err
		}

		for _, v := range known {
			eq, err := cborutil.Equals(v.Voucher, sv)
			if err != nil {
				return false, err
			}
			if v.Proof != nil && eq {
				log.Info("CheckVoucherSpendable: using stored proof")
				proof = v.Proof
				break
			}
		}
		if proof == nil {
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

	ret, err := pm.sm.Call(ctx, &types.Message{
		From:   owner,
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

func (pm *Manager) getPaychOwner(ctx context.Context, ch address.Address) (address.Address, error) {
	var state paych.State
	if _, err := pm.sm.LoadActorState(ctx, ch, &state, nil); err != nil {
		return address.Address{}, err
	}

	return state.From, nil
}

func (pm *Manager) AddVoucher(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	if err := pm.CheckVoucherValid(ctx, ch, sv); err != nil {
		return types.NewInt(0), err
	}

	pm.store.lk.Lock()
	defer pm.store.lk.Unlock()

	ci, err := pm.store.getChannelInfo(ch)
	if err != nil {
		return types.NewInt(0), err
	}

	laneState, err := pm.laneState(ctx, ch, uint64(sv.Lane))
	if err != nil {
		return types.NewInt(0), err
	}

	if minDelta.GreaterThan(types.NewInt(0)) && laneState.Nonce > sv.Nonce {
		return types.NewInt(0), xerrors.Errorf("already storing voucher with higher nonce; %d > %d", laneState.Nonce, sv.Nonce)
	}

	// look for duplicates
	for i, v := range ci.Vouchers {
		eq, err := cborutil.Equals(sv, v.Voucher)
		if err != nil {
			return types.BigInt{}, err
		}
		if !eq {
			continue
		}
		if v.Proof != nil {
			if !bytes.Equal(v.Proof, proof) {
				log.Warnf("AddVoucher: multiple proofs for single voucher, storing both")
				break
			}
			log.Warnf("AddVoucher: voucher re-added with matching proof")
			return types.NewInt(0), nil
		}

		log.Warnf("AddVoucher: adding proof to stored voucher")
		ci.Vouchers[i] = &VoucherInfo{
			Voucher: v.Voucher,
			Proof:   proof,
		}

		return types.NewInt(0), pm.store.putChannelInfo(ci)
	}

	delta := types.BigSub(sv.Amount, laneState.Redeemed)
	if minDelta.GreaterThan(delta) {
		return delta, xerrors.Errorf("addVoucher: supplied token amount too low; minD=%s, D=%s; laneAmt=%s; v.Amt=%s", minDelta, delta, laneState.Redeemed, sv.Amount)
	}

	ci.Vouchers = append(ci.Vouchers, &VoucherInfo{
		Voucher: sv,
		Proof:   proof,
	})

	if ci.NextLane <= (sv.Lane) {
		ci.NextLane = sv.Lane + 1
	}

	return delta, pm.store.putChannelInfo(ci)
}

func (pm *Manager) AllocateLane(ch address.Address) (uint64, error) {
	return pm.store.AllocateLane(ch)
}

func (pm *Manager) ListVouchers(ctx context.Context, ch address.Address) ([]*VoucherInfo, error) {
	// TODO: just having a passthrough method like this feels odd. Seems like
	// there should be some filtering we're doing here
	return pm.store.VouchersForPaych(ch)
}

func (pm *Manager) OutboundChanTo(from, to address.Address) (address.Address, error) {
	pm.store.lk.Lock()
	defer pm.store.lk.Unlock()

	return pm.store.findChan(func(ci *ChannelInfo) bool {
		if ci.Direction != DirOutbound {
			return false
		}
		return ci.Control == from && ci.Target == to
	})
}

func (pm *Manager) NextNonceForLane(ctx context.Context, ch address.Address, lane uint64) (uint64, error) {
	vouchers, err := pm.store.VouchersForPaych(ch)
	if err != nil {
		return 0, err
	}

	var maxnonce uint64
	for _, v := range vouchers {
		if uint64(v.Voucher.Lane) == lane {
			if uint64(v.Voucher.Nonce) > maxnonce {
				maxnonce = uint64(v.Voucher.Nonce)
			}
		}
	}

	return maxnonce + 1, nil
}
