package main

import (
	"bytes"
	"context"
	"encoding/hex"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagesigner"
	"github.com/filecoin-project/lotus/chain/types"
)

type LoggedWallet struct {
	under api.Wallet
}

func (c *LoggedWallet) WalletNew(ctx context.Context, typ types.KeyType) (address.Address, error) {
	log.Infow("WalletNew", "type", typ)

	return c.under.WalletNew(ctx, typ)
}

func (c *LoggedWallet) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	log.Infow("WalletHas", "address", addr)

	return c.under.WalletHas(ctx, addr)
}

func (c *LoggedWallet) WalletList(ctx context.Context) ([]address.Address, error) {
	log.Infow("WalletList")

	return c.under.WalletList(ctx)
}

func (c *LoggedWallet) WalletSign(ctx context.Context, k address.Address, msg []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	switch meta.Type {
	case api.MTChainMsg:
		cmsg, err := chainMsgForSigning(k, msg, meta)
		if err != nil {
			return nil, err
		}

		log.Infow("WalletSign",
			"address", k,
			"type", meta.Type,
			"from", cmsg.From,
			"to", cmsg.To,
			"value", types.FIL(cmsg.Value),
			"feecap", types.FIL(cmsg.RequiredFunds()),
			"method", cmsg.Method,
			"params", hex.EncodeToString(cmsg.Params))
	default:
		log.Infow("WalletSign", "address", k, "type", meta.Type)
	}

	return c.under.WalletSign(ctx, k, msg, meta)
}

// chainMsgForSigning decodes the message in meta.Extra and checks that msg is
// exactly the bytes a node signs for that message from k, so the message that
// is logged or shown for approval is the one the signature covers.
func chainMsgForSigning(k address.Address, msg []byte, meta api.MsgMeta) (*types.Message, error) {
	var cmsg types.Message
	if err := cmsg.UnmarshalCBOR(bytes.NewReader(meta.Extra)); err != nil {
		return nil, xerrors.Errorf("unmarshalling message: %w", err)
	}

	sb, err := messagesigner.SigningBytes(&cmsg, k.Protocol())
	if err != nil {
		return nil, xerrors.Errorf("getting signing bytes for message: %w", err)
	}

	if !bytes.Equal(sb, msg) {
		return nil, xerrors.Errorf("signing bytes do not match the message in meta.Extra")
	}

	return &cmsg, nil
}

func (c *LoggedWallet) WalletExport(ctx context.Context, a address.Address) (*types.KeyInfo, error) {
	log.Infow("WalletExport", "address", a)

	return c.under.WalletExport(ctx, a)
}

func (c *LoggedWallet) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	log.Infow("WalletImport", "type", ki.Type)

	return c.under.WalletImport(ctx, ki)
}

func (c *LoggedWallet) WalletDelete(ctx context.Context, addr address.Address) error {
	log.Infow("WalletDelete", "address", addr)

	return c.under.WalletDelete(ctx, addr)
}
