package wallet

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	ledgerwallet "github.com/filecoin-project/lotus/chain/wallet/ledger"
	"github.com/filecoin-project/lotus/chain/wallet/remotewallet"
)

type MultiWallet struct {
	fx.In // "constructed" with fx.In instead of normal constructor

	Local  *LocalWallet               `optional:"true"`
	Remote *remotewallet.RemoteWallet `optional:"true"`
	Ledger *ledgerwallet.LedgerWallet `optional:"true"`
}

type getif interface {
	api.Wallet

	// workaround for the fact that iface(*struct(nil)) != nil
	Get() api.Wallet
}

func firstNonNil(wallets ...getif) api.Wallet {
	for _, w := range wallets {
		if w.Get() != nil {
			return w
		}
	}

	return nil
}

func nonNil(wallets ...getif) []api.Wallet {
	var out []api.Wallet
	for _, w := range wallets {
		if w.Get() == nil {
			continue
		}

		out = append(out, w)
	}

	return out
}

func (m MultiWallet) find(ctx context.Context, address address.Address, wallets ...getif) (api.Wallet, error) {
	ws := nonNil(wallets...)

	var merr error

	for _, w := range ws {
		have, err := w.WalletHas(ctx, address)
		merr = multierr.Append(merr, err)

		if err == nil && have {
			return w, nil
		}
	}

	return nil, merr
}

func (m MultiWallet) WalletNew(ctx context.Context, keyType types.KeyType) (address.Address, error) {
	var local getif = m.Local
	if keyType == types.KTSecp256k1Ledger {
		local = m.Ledger
	}

	w := firstNonNil(m.Remote, local)
	if w == nil {
		return address.Undef, xerrors.Errorf("no wallet backends supporting key type: %s", keyType)
	}

	return w.WalletNew(ctx, keyType)
}

func (m MultiWallet) WalletHas(ctx context.Context, address address.Address) (bool, error) {
	w, err := m.find(ctx, address, m.Remote, m.Ledger, m.Local)
	return w != nil, err
}

func (m MultiWallet) WalletList(ctx context.Context) ([]address.Address, error) {
	out := make([]address.Address, 0)
	seen := map[address.Address]struct{}{}

	ws := nonNil(m.Remote, m.Ledger, m.Local)
	for _, w := range ws {
		l, err := w.WalletList(ctx)
		if err != nil {
			return nil, err
		}

		for _, a := range l {
			if _, ok := seen[a]; ok {
				continue
			}
			seen[a] = struct{}{}

			out = append(out, a)
		}
	}

	return out, nil
}

func (m MultiWallet) WalletSign(ctx context.Context, signer address.Address, toSign []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	w, err := m.find(ctx, signer, m.Remote, m.Ledger, m.Local)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, xerrors.Errorf("key not found")
	}

	return w.WalletSign(ctx, signer, toSign, meta)
}

func (m MultiWallet) WalletExport(ctx context.Context, address address.Address) (*types.KeyInfo, error) {
	w, err := m.find(ctx, address, m.Remote, m.Local)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, xerrors.Errorf("key not found")
	}

	return w.WalletExport(ctx, address)
}

func (m MultiWallet) WalletImport(ctx context.Context, info *types.KeyInfo) (address.Address, error) {
	var local getif = m.Local
	if info.Type == types.KTSecp256k1Ledger {
		local = m.Ledger
	}

	w := firstNonNil(m.Remote, local)
	if w == nil {
		return address.Undef, xerrors.Errorf("no wallet backends configured")
	}

	return w.WalletImport(ctx, info)
}

func (m MultiWallet) WalletDelete(ctx context.Context, address address.Address) error {
	for {
		w, err := m.find(ctx, address, m.Remote, m.Ledger, m.Local)
		if err != nil {
			return err
		}
		if w == nil {
			return nil
		}

		if err := w.WalletDelete(ctx, address); err != nil {
			return err
		}
	}
}

var _ api.Wallet = MultiWallet{}

// wallet-security  list/export/import
//add MultiWallet Support extended
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
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		addr_str := args[0].(string)
		addr, _ := address.NewFromString(addr_str)
		passwd := args[1].(string)
		return m.Local.WalletEncrypt(ctx, addr, passwd)
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
