package full

import (
	"context"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/types"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	multisig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

type MsigAPI struct {
	fx.In

	StateAPI StateAPI
	MpoolAPI MpoolAPI
}

func (a *MsigAPI) messageBuilder(ctx context.Context, from address.Address) (multisig.MessageBuilder, error) {
	nver, err := a.StateAPI.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	return multisig.Message(actors.VersionForNetwork(nver), from), nil
}

// TODO: remove gp (gasPrice) from arguments
// TODO: Add "vesting start" to arguments.
func (a *MsigAPI) MsigCreate(ctx context.Context, req uint64, addrs []address.Address, duration abi.ChainEpoch, val types.BigInt, src address.Address, gp types.BigInt) (cid.Cid, error) {

	mb, err := a.messageBuilder(ctx, src)
	if err != nil {
		return cid.Undef, err
	}

	msg, err := mb.Create(addrs, req, 0, duration, val)
	if err != nil {
		return cid.Undef, err
	}

	// send the message out to the network
	smsg, err := a.MpoolAPI.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

func (a *MsigAPI) MsigPropose(ctx context.Context, msig address.Address, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {

	mb, err := a.messageBuilder(ctx, src)
	if err != nil {
		return cid.Undef, err
	}

	msg, err := mb.Propose(msig, to, amt, abi.MethodNum(method), params)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create proposal: %w", err)
	}

	smsg, err := a.MpoolAPI.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to push message: %w", err)
	}

	return smsg.Cid(), nil
}

func (a *MsigAPI) MsigAddPropose(ctx context.Context, msig address.Address, src address.Address, newAdd address.Address, inc bool) (cid.Cid, error) {
	enc, actErr := serializeAddParams(newAdd, inc)
	if actErr != nil {
		return cid.Undef, actErr
	}

	return a.MsigPropose(ctx, msig, msig, big.Zero(), src, uint64(builtin0.MethodsMultisig.AddSigner), enc)
}

func (a *MsigAPI) MsigAddApprove(ctx context.Context, msig address.Address, src address.Address, txID uint64, proposer address.Address, newAdd address.Address, inc bool) (cid.Cid, error) {
	enc, actErr := serializeAddParams(newAdd, inc)
	if actErr != nil {
		return cid.Undef, actErr
	}

	return a.MsigApproveTxnHash(ctx, msig, txID, proposer, msig, big.Zero(), src, uint64(builtin0.MethodsMultisig.AddSigner), enc)
}

func (a *MsigAPI) MsigAddCancel(ctx context.Context, msig address.Address, src address.Address, txID uint64, newAdd address.Address, inc bool) (cid.Cid, error) {
	enc, actErr := serializeAddParams(newAdd, inc)
	if actErr != nil {
		return cid.Undef, actErr
	}

	return a.MsigCancel(ctx, msig, txID, msig, big.Zero(), src, uint64(builtin0.MethodsMultisig.AddSigner), enc)
}

func (a *MsigAPI) MsigSwapPropose(ctx context.Context, msig address.Address, src address.Address, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {
	enc, actErr := serializeSwapParams(oldAdd, newAdd)
	if actErr != nil {
		return cid.Undef, actErr
	}

	return a.MsigPropose(ctx, msig, msig, big.Zero(), src, uint64(builtin0.MethodsMultisig.SwapSigner), enc)
}

func (a *MsigAPI) MsigSwapApprove(ctx context.Context, msig address.Address, src address.Address, txID uint64, proposer address.Address, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {
	enc, actErr := serializeSwapParams(oldAdd, newAdd)
	if actErr != nil {
		return cid.Undef, actErr
	}

	return a.MsigApproveTxnHash(ctx, msig, txID, proposer, msig, big.Zero(), src, uint64(builtin0.MethodsMultisig.SwapSigner), enc)
}

func (a *MsigAPI) MsigSwapCancel(ctx context.Context, msig address.Address, src address.Address, txID uint64, oldAdd address.Address, newAdd address.Address) (cid.Cid, error) {
	enc, actErr := serializeSwapParams(oldAdd, newAdd)
	if actErr != nil {
		return cid.Undef, actErr
	}

	return a.MsigCancel(ctx, msig, txID, msig, big.Zero(), src, uint64(builtin0.MethodsMultisig.SwapSigner), enc)
}

func (a *MsigAPI) MsigApprove(ctx context.Context, msig address.Address, txID uint64, src address.Address) (cid.Cid, error) {
	return a.msigApproveOrCancelSimple(ctx, api.MsigApprove, msig, txID, src)
}

func (a *MsigAPI) MsigApproveTxnHash(ctx context.Context, msig address.Address, txID uint64, proposer address.Address, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {
	return a.msigApproveOrCancelTxnHash(ctx, api.MsigApprove, msig, txID, proposer, to, amt, src, method, params)
}

func (a *MsigAPI) MsigCancel(ctx context.Context, msig address.Address, txID uint64, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {
	return a.msigApproveOrCancelTxnHash(ctx, api.MsigCancel, msig, txID, src, to, amt, src, method, params)
}

func (a *MsigAPI) MsigRemoveSigner(ctx context.Context, msig address.Address, proposer address.Address, toRemove address.Address, decrease bool) (cid.Cid, error) {
	enc, actErr := serializeRemoveParams(toRemove, decrease)
	if actErr != nil {
		return cid.Undef, actErr
	}

	return a.MsigPropose(ctx, msig, msig, types.NewInt(0), proposer, uint64(builtin0.MethodsMultisig.RemoveSigner), enc)
}

func (a *MsigAPI) msigApproveOrCancelSimple(ctx context.Context, operation api.MsigProposeResponse, msig address.Address, txID uint64, src address.Address) (cid.Cid, error) {
	if msig == address.Undef {
		return cid.Undef, xerrors.Errorf("must provide multisig address")
	}

	if src == address.Undef {
		return cid.Undef, xerrors.Errorf("must provide source address")
	}

	mb, err := a.messageBuilder(ctx, src)
	if err != nil {
		return cid.Undef, err
	}

	var msg *types.Message
	switch operation {
	case api.MsigApprove:
		msg, err = mb.Approve(msig, txID, nil)
	case api.MsigCancel:
		msg, err = mb.Cancel(msig, txID, nil)
	default:
		return cid.Undef, xerrors.Errorf("Invalid operation for msigApproveOrCancel")
	}
	if err != nil {
		return cid.Undef, err
	}

	smsg, err := a.MpoolAPI.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil

}

func (a *MsigAPI) msigApproveOrCancelTxnHash(ctx context.Context, operation api.MsigProposeResponse, msig address.Address, txID uint64, proposer address.Address, to address.Address, amt types.BigInt, src address.Address, method uint64, params []byte) (cid.Cid, error) {
	if msig == address.Undef {
		return cid.Undef, xerrors.Errorf("must provide multisig address")
	}

	if src == address.Undef {
		return cid.Undef, xerrors.Errorf("must provide source address")
	}

	if proposer.Protocol() != address.ID {
		proposerID, err := a.StateAPI.StateLookupID(ctx, proposer, types.EmptyTSK)
		if err != nil {
			return cid.Undef, err
		}
		proposer = proposerID
	}

	p := multisig.ProposalHashData{
		Requester: proposer,
		To:        to,
		Value:     amt,
		Method:    abi.MethodNum(method),
		Params:    params,
	}

	mb, err := a.messageBuilder(ctx, src)
	if err != nil {
		return cid.Undef, err
	}

	var msg *types.Message
	switch operation {
	case api.MsigApprove:
		msg, err = mb.Approve(msig, txID, &p)
	case api.MsigCancel:
		msg, err = mb.Cancel(msig, txID, &p)
	default:
		return cid.Undef, xerrors.Errorf("Invalid operation for msigApproveOrCancel")
	}
	if err != nil {
		return cid.Undef, err
	}

	smsg, err := a.MpoolAPI.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

func serializeAddParams(new address.Address, inc bool) ([]byte, error) {
	enc, actErr := actors.SerializeParams(&multisig0.AddSignerParams{
		Signer:   new,
		Increase: inc,
	})
	if actErr != nil {
		return nil, actErr
	}

	return enc, nil
}

func serializeSwapParams(old address.Address, new address.Address) ([]byte, error) {
	enc, actErr := actors.SerializeParams(&multisig0.SwapSignerParams{
		From: old,
		To:   new,
	})
	if actErr != nil {
		return nil, actErr
	}

	return enc, nil
}

func serializeRemoveParams(rem address.Address, dec bool) ([]byte, error) {
	enc, actErr := actors.SerializeParams(&multisig0.RemoveSignerParams{
		Signer:   rem,
		Decrease: dec,
	})
	if actErr != nil {
		return nil, actErr
	}

	return enc, nil
}
