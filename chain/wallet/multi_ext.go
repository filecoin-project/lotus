package wallet

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

// wallet-security MultiWallet func
// add MultiWallet Support extended
func (m MultiWallet) WalletListEncrypt(ctx context.Context) ([]api.AddrListEncrypt, error) {
	out := make([]api.AddrListEncrypt, 0)
	ws := nonNil(m.Remote, m.Ledger, m.Local)
	for _, w := range ws {
		if w == m.Local.Get() {
			l, err := m.Local.WalletListEncrypt(ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, l...)
		} else {
			l, err := w.WalletList(ctx)
			if err != nil {
				return nil, err
			}
			for _, v := range l {
				out = append(out, api.AddrListEncrypt{
					Addr:    v,
					Encrypt: false,
				})
			}
		}
	}
	return out, nil
}
func (m MultiWallet) WalletExportEncrypt(ctx context.Context, address address.Address, passwd string) (*types.KeyInfo, error) {
	w, err := m.find(ctx, address, m.Remote, m.Local)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, xerrors.Errorf("key not found")
	}
	if w == m.Local.Get() {
		return m.Local.WalletExportEncrypt(ctx, address, passwd)
	}
	return w.WalletExport(ctx, address)
}
func (m MultiWallet) WalletDeleteEncrypt(ctx context.Context, address address.Address, passwd string) error {
	w, err := m.find(ctx, address, m.Remote, m.Ledger, m.Local)
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	if w == m.Local.Get() {
		return m.Local.WalletDeleteEncrypt(ctx, address, passwd)
	}
	return w.WalletDelete(ctx, address)
}

// wallet-security MultiWallet WalletCustomMethod
// WalletCustomMethod dont use this method for MultiWallet
func (m MultiWallet) WalletCustomMethod(ctx context.Context, meth api.WalletMethod, args []interface{}) (interface{}, error) {
	switch meth {
	case api.WalletListForEnc:
		return m.WalletListEncrypt(ctx)
	case api.WalletExportForEnc:
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		addr_str := args[0].(string)
		addr, _ := address.NewFromString(addr_str)
		passwd := args[1].(string)
		return m.WalletExportEncrypt(ctx, addr, passwd)
	case api.WalletDeleteForEnc:
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		addr_str := args[0].(string)
		addr, _ := address.NewFromString(addr_str)
		passwd := args[1].(string)
		return nil, m.WalletDeleteEncrypt(ctx, addr, passwd)
	case api.WalletEncrypt:
		if len(args) < 1 {
			return nil, xerrors.Errorf("args must is 1 for exec method, but get args is %v", len(args))
		}
		addr_str := args[0].(string)
		addr, _ := address.NewFromString(addr_str)
		return m.Local.WalletEncrypt(ctx, addr)
	case api.WalletDecrypt:
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		addr_str := args[0].(string)
		addr, _ := address.NewFromString(addr_str)
		passwd := args[1].(string)
		return m.Local.WalletDecrypt(ctx, addr, passwd)
	case api.WalletIsEncrypt:
		if len(args) < 1 {
			return nil, xerrors.Errorf("args must is 1 for exec method, but get args is %v", len(args))
		}
		addr_str := args[0].(string)
		addr, _ := address.NewFromString(addr_str)
		return m.Local.WalletIsEncrypt(ctx, addr)
	}
	return m.Local.WalletCustomMethod(ctx, meth, args)
}

var _ api.Wallet = MultiWallet{}
