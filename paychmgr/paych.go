package paychmgr

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/api"

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

type StateManagerApi interface {
	LoadActorState(ctx context.Context, a address.Address, out interface{}, ts *types.TipSet) (*types.Actor, error)
	Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error)
}

type Manager struct {
	store *Store
	sm    StateManagerApi

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

// Used by the tests to supply mocks
func newManager(sm StateManagerApi, pchstore *Store) *Manager {
	return &Manager{
		store: pchstore,
		sm:    sm,
	}
}

func (pm *Manager) TrackOutboundChannel(ctx context.Context, ch address.Address) error {
	return pm.trackChannel(ctx, ch, DirOutbound)
}

func (pm *Manager) TrackInboundChannel(ctx context.Context, ch address.Address) error {
	return pm.trackChannel(ctx, ch, DirInbound)
}

func (pm *Manager) trackChannel(ctx context.Context, ch address.Address, dir uint64) error {
	ci, err := pm.loadStateChannelInfo(ctx, ch, dir)
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

// CheckVoucherValid checks if the given voucher is valid (is or could become spendable at some point)
func (pm *Manager) CheckVoucherValid(ctx context.Context, ch address.Address, sv *paych.SignedVoucher) error {
	_, err := pm.checkVoucherValid(ctx, ch, sv)
	return err
}

func (pm *Manager) checkVoucherValid(ctx context.Context, ch address.Address, sv *paych.SignedVoucher) (map[uint64]*paych.LaneState, error) {
	act, pchState, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return nil, err
	}

	var account account.State
	_, err = pm.sm.LoadActorState(ctx, pchState.From, &account, nil)
	if err != nil {
		return nil, err
	}
	from := account.Address

	// verify signature
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
	laneStates, err := pm.laneState(pchState, ch)
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
	totalRedeemed, err := pm.totalRedeemedWithVoucher(laneStates, sv)
	if err != nil {
		return nil, err
	}

	// Total required balance = total redeemed + toSend
	// Must not exceed actor balance
	newTotal := types.BigAdd(totalRedeemed, pchState.ToSend)
	if act.Balance.LessThan(newTotal) {
		return nil, fmt.Errorf("not enough funds in channel to cover voucher")
	}

	if len(sv.Merges) != 0 {
		return nil, fmt.Errorf("dont currently support paych lane merges")
	}

	return laneStates, nil
}

// CheckVoucherSpendable checks if the given voucher is currently spendable
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
	pm.store.lk.Lock()
	defer pm.store.lk.Unlock()

	ci, err := pm.store.getChannelInfo(ch)
	if err != nil {
		return types.NewInt(0), err
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

			return types.NewInt(0), pm.store.putChannelInfo(ci)
		}

		// Otherwise just ignore the duplicate voucher
		log.Warnf("AddVoucher: voucher re-added with matching proof")
		return types.NewInt(0), nil
	}

	// Check voucher validity
	laneStates, err := pm.checkVoucherValid(ctx, ch, sv)
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

	return delta, pm.store.putChannelInfo(ci)
}

func (pm *Manager) AllocateLane(ch address.Address) (uint64, error) {
	// TODO: should this take into account lane state?
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
	// TODO: should this take into account lane state?
	vouchers, err := pm.store.VouchersForPaych(ch)
	if err != nil {
		return 0, err
	}

	var maxnonce uint64
	for _, v := range vouchers {
		if v.Voucher.Lane == lane {
			if v.Voucher.Nonce > maxnonce {
				maxnonce = v.Voucher.Nonce
			}
		}
	}

	return maxnonce + 1, nil
}
