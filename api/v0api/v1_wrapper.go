package v0api

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

type WrapperV1Full struct {
	v1api.FullNode
}

func (w *WrapperV1Full) StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, s abi.SectorNumber, tsk types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error) {
	pi, err := w.FullNode.StateSectorPreCommitInfo(ctx, maddr, s, tsk)
	if err != nil {
		return miner.SectorPreCommitOnChainInfo{}, err
	}
	if pi == nil {
		return miner.SectorPreCommitOnChainInfo{}, xerrors.Errorf("precommit info does not exist")
	}

	return *pi, nil
}

func (w *WrapperV1Full) StateSearchMsg(ctx context.Context, msg cid.Cid) (*api.MsgLookup, error) {
	return w.FullNode.StateSearchMsg(ctx, types.EmptyTSK, msg, api.LookbackNoLimit, true)
}

func (w *WrapperV1Full) StateSearchMsgLimited(ctx context.Context, msg cid.Cid, limit abi.ChainEpoch) (*api.MsgLookup, error) {
	return w.FullNode.StateSearchMsg(ctx, types.EmptyTSK, msg, limit, true)
}

func (w *WrapperV1Full) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	return w.FullNode.StateWaitMsg(ctx, msg, confidence, api.LookbackNoLimit, true)
}

func (w *WrapperV1Full) StateWaitMsgLimited(ctx context.Context, msg cid.Cid, confidence uint64, limit abi.ChainEpoch) (*api.MsgLookup, error) {
	return w.FullNode.StateWaitMsg(ctx, msg, confidence, limit, true)
}

func (w *WrapperV1Full) StateGetReceipt(ctx context.Context, msg cid.Cid, from types.TipSetKey) (*types.MessageReceipt, error) {
	ml, err := w.FullNode.StateSearchMsg(ctx, from, msg, api.LookbackNoLimit, true)
	if err != nil {
		return nil, err
	}

	if ml == nil {
		return nil, nil
	}

	return &ml.Receipt, nil
}

func (w *WrapperV1Full) Version(ctx context.Context) (api.APIVersion, error) {
	ver, err := w.FullNode.Version(ctx)
	if err != nil {
		return api.APIVersion{}, err
	}

	ver.APIVersion = api.FullAPIVersion0

	return ver, nil
}

func (w *WrapperV1Full) executePrototype(ctx context.Context, p *api.MessagePrototype) (cid.Cid, error) {
	sm, err := w.FullNode.MpoolPushMessage(ctx, &p.Message, nil)
	if err != nil {
		return cid.Undef, xerrors.Errorf("pushing message: %w", err)
	}

	return sm.Cid(), nil
}
func (w *WrapperV1Full) MsigCreate(ctx context.Context, req uint64, addrs []address.Address, duration abi.ChainEpoch, val types.BigInt, src address.Address, gp types.BigInt) (cid.Cid, error) {

	p, err := w.FullNode.MsigCreate(ctx, req, addrs, duration, val, src, gp)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigPropose(ctx context.Context, msig address.Address, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {

	p, err := w.FullNode.MsigPropose(ctx, msig, to, amt, src, method, params)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}
func (w *WrapperV1Full) MsigApprove(ctx context.Context, msig address.Address, txID uint64, src address.Address) (cid.Cid, error) {

	p, err := w.FullNode.MsigApprove(ctx, msig, txID, src)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigApproveTxnHash(ctx context.Context, msig address.Address, txID uint64, proposer address.Address, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {
	p, err := w.FullNode.MsigApproveTxnHash(ctx, msig, txID, proposer, to, amt, src, method, params)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigCancel(ctx context.Context, msig address.Address, txID uint64, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {
	p, err := w.FullNode.MsigCancelTxnHash(ctx, msig, txID, to, amt, src, method, params)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigAddPropose(ctx context.Context, msig address.Address, src address.Address, newAdd address.Address, inc bool) (cid.Cid, error) {

	p, err := w.FullNode.MsigAddPropose(ctx, msig, src, newAdd, inc)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigAddApprove(ctx context.Context, msig address.Address, src address.Address, txID uint64, proposer address.Address, newAdd address.Address, inc bool) (cid.Cid, error) {

	p, err := w.FullNode.MsigAddApprove(ctx, msig, src, txID, proposer, newAdd, inc)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigAddCancel(ctx context.Context, msig address.Address, src address.Address, txID uint64, newAdd address.Address, inc bool) (cid.Cid, error) {

	p, err := w.FullNode.MsigAddCancel(ctx, msig, src, txID, newAdd, inc)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigSwapPropose(ctx context.Context, msig address.Address, src address.Address, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {

	p, err := w.FullNode.MsigSwapPropose(ctx, msig, src, oldAdd, newAdd)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigSwapApprove(ctx context.Context, msig address.Address, src address.Address, txID uint64, proposer address.Address, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {

	p, err := w.FullNode.MsigSwapApprove(ctx, msig, src, txID, proposer, oldAdd, newAdd)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigSwapCancel(ctx context.Context, msig address.Address, src address.Address, txID uint64, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {

	p, err := w.FullNode.MsigSwapCancel(ctx, msig, src, txID, oldAdd, newAdd)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) MsigRemoveSigner(ctx context.Context, msig address.Address, proposer address.Address, toRemove address.Address, decrease bool) (cid.Cid, error) {

	p, err := w.FullNode.MsigRemoveSigner(ctx, msig, proposer, toRemove, decrease)
	if err != nil {
		return cid.Undef, xerrors.Errorf("creating prototype: %w", err)
	}

	return w.executePrototype(ctx, p)
}

func (w *WrapperV1Full) ChainGetRandomnessFromTickets(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	return w.StateGetRandomnessFromTickets(ctx, personalization, randEpoch, entropy, tsk)
}

func (w *WrapperV1Full) ChainGetRandomnessFromBeacon(ctx context.Context, tsk types.TipSetKey, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error) {
	return w.StateGetRandomnessFromBeacon(ctx, personalization, randEpoch, entropy, tsk)
}

func (w *WrapperV1Full) PaychGet(ctx context.Context, from, to address.Address, amt types.BigInt) (*api.ChannelInfo, error) {
	return w.FullNode.PaychFund(ctx, from, to, amt)
}

func (w *WrapperV1Full) BeaconGetEntry(ctx context.Context, epoch abi.ChainEpoch) (*types.BeaconEntry, error) {
	return w.StateGetBeaconEntry(ctx, epoch)
}

var _ FullNode = &WrapperV1Full{}
