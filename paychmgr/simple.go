package paychmgr

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func (pm *Manager) createPaych(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, cid.Cid, error) {
	params, aerr := actors.SerializeParams(&paych.ConstructorParams{From: from, To: to})
	if aerr != nil {
		return address.Undef, cid.Undef, aerr
	}

	enc, aerr := actors.SerializeParams(&init_.ExecParams{
		CodeCID:           builtin.PaymentChannelActorCodeID,
		ConstructorParams: params,
	})
	if aerr != nil {
		return address.Undef, cid.Undef, aerr
	}

	msg := &types.Message{
		To:       builtin.InitActorAddr,
		From:     from,
		Value:    amt,
		Method:   builtin.MethodsInit.Exec,
		Params:   enc,
		GasLimit: types.NewInt(1000000),
		GasPrice: types.NewInt(0),
	}

	smsg, err := pm.mpool.MpoolPushMessage(ctx, msg)
	if err != nil {
		return address.Undef, cid.Undef, xerrors.Errorf("initializing paych actor: %w", err)
	}

	mcid := smsg.Cid()

	// TODO: wait outside the store lock!
	//  (tricky because we need to setup channel tracking before we know it's address)
	mwait, err := pm.state.StateWaitMsg(ctx, mcid)
	if err != nil {
		return address.Undef, cid.Undef, xerrors.Errorf("wait msg: %w", err)
	}

	if mwait.Receipt.ExitCode != 0 {
		return address.Undef, cid.Undef, fmt.Errorf("payment channel creation failed (exit code %d)", mwait.Receipt.ExitCode)
	}

	var decodedReturn init_.ExecReturn
	err = decodedReturn.UnmarshalCBOR(bytes.NewReader(mwait.Receipt.Return))
	if err != nil {
		return address.Undef, cid.Undef, err
	}
	paychaddr := decodedReturn.RobustAddress

	ci, err := pm.loadOutboundChannelInfo(ctx, paychaddr)
	if err != nil {
		return address.Undef, cid.Undef, xerrors.Errorf("loading channel info: %w", err)
	}

	if err := pm.store.trackChannel(ci); err != nil {
		return address.Undef, cid.Undef, xerrors.Errorf("tracking channel: %w", err)
	}

	return paychaddr, mcid, nil
}

func (pm *Manager) addFunds(ctx context.Context, ch address.Address, from address.Address, amt types.BigInt) error {
	msg := &types.Message{
		To:       ch,
		From:     from,
		Value:    amt,
		Method:   0,
		GasLimit: types.NewInt(1000000),
		GasPrice: types.NewInt(0),
	}

	smsg, err := pm.mpool.MpoolPushMessage(ctx, msg)
	if err != nil {
		return err
	}

	mwait, err := pm.state.StateWaitMsg(ctx, smsg.Cid()) // TODO: wait outside the store lock!
	if err != nil {
		return err
	}

	if mwait.Receipt.ExitCode != 0 {
		return fmt.Errorf("voucher channel creation failed: adding funds (exit code %d)", mwait.Receipt.ExitCode)
	}

	return nil
}

func (pm *Manager) GetPaych(ctx context.Context, from, to address.Address, ensureFree types.BigInt) (address.Address, cid.Cid, error) {
	pm.store.lk.Lock()
	defer pm.store.lk.Unlock()

	ch, err := pm.store.findChan(func(ci *ChannelInfo) bool {
		if ci.Direction != DirOutbound {
			return false
		}
		return ci.Control == from && ci.Target == to
	})
	if err != nil {
		return address.Undef, cid.Undef, xerrors.Errorf("findChan: %w", err)
	}
	if ch != address.Undef {
		// TODO: Track available funds
		return ch, cid.Undef, pm.addFunds(ctx, ch, from, ensureFree)
	}

	return pm.createPaych(ctx, from, to, ensureFree)
}
