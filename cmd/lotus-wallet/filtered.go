package main

import (
	"bytes"
	"context"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type FilteredWallet struct {
	under      api.Wallet
	mustAccept bool
}

func (c *FilteredWallet) applyRules(ctx context.Context, filterParams map[FilterParam]interface{}) error {
	p, ok := ctx.Value(tokenKey).(jwtPayload)
	if !ok {
		return xerrors.Errorf("jwt payload not set on request context")
	}
	if c.mustAccept && p.Rules == nil {
		return xerrors.Errorf("token with no rules")
	}
	if p.Rules != nil {
		f, err := ParseRule(ctx, *p.Rules)
		if err != nil {
			return xerrors.Errorf("parsing rules: %w", err)
		}

		fres := f(filterParams)
		switch fres {
		case nil:
			if c.mustAccept {
				return xerrors.Errorf("filter didn't accept")
			}
			fallthrough
		case ErrAccept:

		default:
			return xerrors.Errorf("filter error: %w", fres)
		}
	}

	return nil
}

func (c *FilteredWallet) WalletNew(ctx context.Context, typ types.KeyType) (address.Address, error) {
	filterParams := map[FilterParam]interface{}{
		ParamAction: ActionNew,
	}
	if err := c.applyRules(ctx, filterParams); err != nil {
		return address.Undef, err
	}

	return c.under.WalletNew(ctx, typ)
}

func (c *FilteredWallet) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	filterParams := map[FilterParam]interface{}{
		ParamAction: ActionHas,
	}
	if err := c.applyRules(ctx, filterParams); err != nil {
		return false, err
	}

	return c.under.WalletHas(ctx, addr)
}

func (c *FilteredWallet) WalletList(ctx context.Context) ([]address.Address, error) {
	filterParams := map[FilterParam]interface{}{
		ParamAction: ActionList,
	}
	if err := c.applyRules(ctx, filterParams); err != nil {
		return nil, err
	}

	return c.under.WalletList(ctx)
}

func (c *FilteredWallet) WalletSign(ctx context.Context, k address.Address, msg []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	var filterParams map[FilterParam]interface{}

	switch meta.Type {
	case api.MTChainMsg:
		var cmsg types.Message
		if err := cmsg.UnmarshalCBOR(bytes.NewReader(meta.Extra)); err != nil {
			return nil, xerrors.Errorf("unmarshalling message: %w", err)
		}

		_, bc, err := cid.CidFromBytes(msg)
		if err != nil {
			return nil, xerrors.Errorf("getting cid from signing bytes: %w", err)
		}

		if !cmsg.Cid().Equals(bc) {
			return nil, xerrors.Errorf("cid(meta.Extra).bytes() != msg")
		}

		filterParams = map[FilterParam]interface{}{
			ParamAction:   ActionSign,
			ParamSignType: api.MTChainMsg,

			ParamSource:      cmsg.From,
			ParamDestination: cmsg.To,
			ParamValue:       cmsg.Value,
			ParamMethod:      cmsg.Method,
			ParamMaxFee:      cmsg.RequiredFunds(),
		}
	case api.MTBlock:
		bh, err := types.DecodeBlock(msg)
		if err != nil {
			return nil, xerrors.Errorf("decode block header: %w", err)
		}

		filterParams = map[FilterParam]interface{}{
			ParamAction:   ActionSign,
			ParamSignType: api.MTBlock,

			ParamMiner: bh.Miner,
		}
	case api.MTDealProposal:
		var dp market0.DealProposal
		if err := dp.UnmarshalCBOR(bytes.NewReader(msg)); err != nil {
			return nil, xerrors.Errorf("unmarshalling deal proposal: %w", err)
		}

		filterParams = map[FilterParam]interface{}{
			ParamAction:   ActionSign,
			ParamSignType: api.MTDealProposal,
		}
	default:
		filterParams = map[FilterParam]interface{}{
			ParamAction:   ActionSign,
			ParamSignType: api.MTUnknown,
		}
	}

	filterParams[ParamSigner] = k

	if err := c.applyRules(ctx, filterParams); err != nil {
		return nil, err
	}

	return c.under.WalletSign(ctx, k, msg, meta)
}

func (c *FilteredWallet) WalletExport(ctx context.Context, a address.Address) (*types.KeyInfo, error) {
	filterParams := map[FilterParam]interface{}{
		ParamAction: ActionExport,
	}
	if err := c.applyRules(ctx, filterParams); err != nil {
		return nil, err
	}

	return c.under.WalletExport(ctx, a)
}

func (c *FilteredWallet) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	filterParams := map[FilterParam]interface{}{
		ParamAction: ActionImport,
	}
	if err := c.applyRules(ctx, filterParams); err != nil {
		return address.Undef, err
	}

	return c.under.WalletImport(ctx, ki)
}

func (c *FilteredWallet) WalletDelete(ctx context.Context, addr address.Address) error {
	filterParams := map[FilterParam]interface{}{
		ParamAction: ActionDelete,
	}
	if err := c.applyRules(ctx, filterParams); err != nil {
		return err
	}

	return c.under.WalletDelete(ctx, addr)
}
