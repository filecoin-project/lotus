package main

import (
	"bytes"
	"context"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type FilteredWallet struct {
	under      api.Wallet
	mustAccept bool
}

func (c *FilteredWallet) WalletNew(ctx context.Context, typ types.KeyType) (address.Address, error) {
	log.Infow("WalletNew", "type", typ)
	// TODO FILTER!

	return c.under.WalletNew(ctx, typ)
}

func (c *FilteredWallet) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	log.Infow("WalletHas", "address", addr)
	// TODO FILTER!

	return c.under.WalletHas(ctx, addr)
}

func (c *FilteredWallet) WalletList(ctx context.Context) ([]address.Address, error) {
	// TODO FILTER!

	return c.under.WalletList(ctx)
}

func (c *FilteredWallet) WalletSign(ctx context.Context, k address.Address, msg []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	p, ok := ctx.Value(tokenKey).(jwtPayload)
	if !ok {
		return nil, xerrors.Errorf("jwt payload not set on request context")
	}
	if c.mustAccept && p.Rules == nil {
		return nil, xerrors.Errorf("token with no rules")
	}

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
		// TODO FILTER PARAMS!
		filterParams = map[FilterParam]interface{}{
			ParamAction:   ActionSign,
			ParamSignType: api.MTBlock,
		}
	case api.MTDealProposal:
		// TODO FILTER PARAMS!
		filterParams = map[FilterParam]interface{}{
			ParamAction:   ActionSign,
			ParamSignType: api.MTDealProposal,
		}
	default:
		// TODO FILTER PARAMS!
		filterParams = map[FilterParam]interface{}{
			ParamAction:   ActionSign,
			ParamSignType: api.MTUnknown,
		}
	}

	if p.Rules != nil {
		f, err := ParseRule(ctx, *p.Rules)
		if err != nil {
			return nil, xerrors.Errorf("parsing rules: %w", err)
		}

		fres := f(filterParams)
		switch fres {
		case nil:
			if c.mustAccept {
				return nil, xerrors.Errorf("filter didn't accept")
			}
			fallthrough
		case ErrAccept:

		default:
			return nil, xerrors.Errorf("filter error: %w", fres)
		}
	}

	return c.under.WalletSign(ctx, k, msg, meta)
}

func (c *FilteredWallet) WalletExport(ctx context.Context, a address.Address) (*types.KeyInfo, error) {
	log.Infow("WalletExport", "address", a)
	// TODO FILTER!
	return c.under.WalletExport(ctx, a)
}

func (c *FilteredWallet) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	log.Infow("WalletImport", "type", ki.Type)
	// TODO FILTER!
	return c.under.WalletImport(ctx, ki)
}

func (c *FilteredWallet) WalletDelete(ctx context.Context, addr address.Address) error {
	log.Infow("WalletDelete", "address", addr)
	// TODO FILTER!
	return c.under.WalletDelete(ctx, addr)
}
