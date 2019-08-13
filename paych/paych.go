package paych

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"

	hamt "github.com/ipfs/go-hamt-ipld"
)

type Manager struct {
	chain *store.ChainStore
	store *Store
}

func NewManager(chain *store.ChainStore, pchstore *Store) *Manager {
	return &Manager{
		chain: chain,
		store: pchstore,
	}
}

func (pm *Manager) TrackInboundChannel(ctx context.Context, ch address.Address) error {
	_, st, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return err
	}

	return pm.store.TrackChannel(&ChannelInfo{
		Channel:     ch,
		Direction:   DirInbound,
		ControlAddr: st.To,
	})
}

func (pm *Manager) TrackOutboundChannel(ctx context.Context, ch address.Address) error {
	_, st, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return err
	}

	return pm.store.TrackChannel(&ChannelInfo{
		Channel:     ch,
		Direction:   DirOutbound,
		ControlAddr: st.From,
	})
}

func (pm *Manager) ListChannels() ([]address.Address, error) {
	return pm.store.ListChannels()
}

func (pm *Manager) GetChannelInfo(addr address.Address) (*ChannelInfo, error) {
	return pm.store.getChannelInfo(addr)
}

// checks if the given voucher is valid (is or could become spendable at some point)
func (pm *Manager) CheckVoucherValid(ctx context.Context, ch address.Address, sv *types.SignedVoucher) error {
	act, pca, err := pm.loadPaychState(ctx, ch)
	if err != nil {
		return err
	}

	// verify signature
	vb, err := sv.SigningBytes()
	if err != nil {
		return err
	}

	// TODO: technically, either party may create and sign a voucher.
	// However, for now, we only accept them from the channel creator.
	// More complex handling logic can be added later
	if err := sv.Signature.Verify(pca.From, vb); err != nil {
		return err
	}

	sendAmount := sv.Amount

	// now check the lane state
	// TODO: should check against vouchers in our local store too
	// there might be something conflicting
	ls, ok := pca.LaneStates[fmt.Sprint(sv.Lane)]
	if !ok {
	} else {
		if ls.Closed {
			return fmt.Errorf("voucher is on a closed lane")
		}
		if ls.Nonce >= sv.Nonce {
			return fmt.Errorf("nonce too low")
		}

		sendAmount = types.BigSub(sv.Amount, ls.Redeemed)
	}

	// TODO: also account for vouchers on other lanes we've received
	newTotal := types.BigAdd(sendAmount, pca.ToSend)
	if types.BigCmp(act.Balance, newTotal) < 0 {
		return fmt.Errorf("not enough funds in channel to cover voucher")
	}

	if len(sv.Merges) != 0 {
		return fmt.Errorf("dont currently support paych lane merges")
	}

	return nil
}

// checks if the given voucher is currently spendable
func (pm *Manager) CheckVoucherSpendable(ctx context.Context, ch address.Address, sv *types.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	owner, err := pm.getPaychOwner(ctx, ch)
	if err != nil {
		return false, err
	}

	enc, err := actors.SerializeParams(&actors.PCAUpdateChannelStateParams{
		Sv:     *sv,
		Secret: secret,
		Proof:  proof,
	})
	if err != nil {
		return false, err
	}

	ret, err := vm.Call(ctx, pm.chain, &types.Message{
		From:   owner,
		To:     ch,
		Method: actors.PCAMethods.UpdateChannelState,
		Params: enc,
	}, nil)
	if err != nil {
		return false, err
	}

	if ret.ExitCode != 0 {
		return false, nil
	}

	return true, nil
}

func (pm *Manager) loadPaychState(ctx context.Context, ch address.Address) (*types.Actor, *actors.PaymentChannelActorState, error) {
	st, err := pm.chain.TipSetState(pm.chain.GetHeaviestTipSet().Cids())
	if err != nil {
		return nil, nil, err
	}

	cst := hamt.CSTFromBstore(pm.chain.Blockstore())
	tree, err := state.LoadStateTree(cst, st)
	if err != nil {
		return nil, nil, err
	}

	act, err := tree.GetActor(ch)
	if err != nil {
		return nil, nil, err
	}

	var pcast actors.PaymentChannelActorState
	if err := cst.Get(ctx, act.Head, &pcast); err != nil {
		return nil, nil, err
	}

	return act, &pcast, nil
}

func (pm *Manager) getPaychOwner(ctx context.Context, ch address.Address) (address.Address, error) {
	ret, err := vm.Call(ctx, pm.chain, &types.Message{
		From:   ch,
		To:     ch,
		Method: actors.PCAMethods.GetOwner,
	}, nil)
	if err != nil {
		return address.Undef, err
	}

	if ret.ExitCode != 0 {
		return address.Undef, fmt.Errorf("failed to get payment channel owner (exit code %d)", ret.ExitCode)
	}

	return address.NewFromBytes(ret.Return)
}

func (pm *Manager) AddVoucher(ctx context.Context, ch address.Address, sv *types.SignedVoucher) error {
	if err := pm.CheckVoucherValid(ctx, ch, sv); err != nil {
		return err
	}

	return pm.store.AddVoucher(ch, sv)
}

func (pm *Manager) ListVouchers(ctx context.Context, ch address.Address) ([]*types.SignedVoucher, error) {
	// TODO: just having a passthrough method like this feels odd. Seems like
	// there should be some filtering we're doing here
	return pm.store.VouchersForPaych(ch)
}

func (pm *Manager) NextNonceForLane(ctx context.Context, ch address.Address, lane uint64) (uint64, error) {
	vouchers, err := pm.store.VouchersForPaych(ch)
	if err != nil {
		return 0, err
	}

	var maxnonce uint64
	for _, v := range vouchers {
		if v.Lane == lane {
			if v.Nonce > maxnonce {
				maxnonce = v.Nonce
			}
		}
	}

	return maxnonce + 1, nil
}
