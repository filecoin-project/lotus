package main

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type LoggedWallet struct {
	under api.WalletAPI
}

func (c *LoggedWallet) WalletNew(ctx context.Context, typ crypto.SigType) (address.Address, error) {
	n, err := typ.Name()
	if err != nil {
		return address.Address{}, err
	}

	log.Infow("WalletNew", "type", n)

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

func (c *LoggedWallet) WalletSign(ctx context.Context, k address.Address, msg []byte) (*crypto.Signature, error) {
	log.Infow("WalletSign", "address", k)

	return c.under.WalletSign(ctx, k, msg)
}

func (c *LoggedWallet) WalletSignMessage(ctx context.Context, k address.Address, msg *types.Message) (*types.SignedMessage, error) {
	log.Infow("WalletSignMessage", "address", k)

	return c.under.WalletSignMessage(ctx, k, msg)
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