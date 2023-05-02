package wallet

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"golang.org/x/xerrors"
)

// wallet-security LocalWallet WalletCustomMethod
func (w *LocalWallet) WalletCustomMethod(ctx context.Context, meth api.WalletMethod, args []interface{}) (interface{}, error) {

	switch meth {
	case api.Unknown:
		return nil, xerrors.Errorf("exec method is unknown")
	case api.WalletListForEnc:
		return w.WalletListEncrypt(ctx)
	case api.WalletExportForEnc:
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		addr := args[0].(address.Address)
		passwd := args[1].(string)
		return w.WalletExportEncrypt(ctx, addr, passwd)
	case api.WalletDeleteForEnc:
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		addr := args[0].(address.Address)
		passwd := args[1].(string)
		return nil, w.WalletDeleteEncrypt(ctx, addr, passwd)
	case api.WalletAddPasswd:
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		passwd := args[0].(string)
		path := args[1].(string)
		return nil, w.WalletAddPasswd(ctx, passwd, path)
	case api.WalletResetPasswd:
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		oldPasswd := args[0].(string)
		newPasswd := args[1].(string)
		return w.WalletResetPasswd(ctx, oldPasswd, newPasswd)
	case api.WalletClearPasswd:
		if len(args) < 1 {
			return nil, xerrors.Errorf("args must is 1 for exec method, but get args is %v", len(args))
		}
		passwd := args[0].(string)
		return w.WalletClearPasswd(ctx, passwd)
	case api.WalletCheckPasswd:
		if len(args) < 1 {
			return nil, xerrors.Errorf("args must is 1 for exec method, but get args is %v", len(args))
		}
		passwd := args[0].(string)
		return w.WalletCheckPasswd(ctx, passwd), nil
	case api.WalletEncrypt:
		if len(args) < 1 {
			return nil, xerrors.Errorf("args must is 1 for exec method, but get args is %v", len(args))
		}
		addr := args[0].(address.Address)
		return w.WalletEncrypt(ctx, addr)
	case api.WalletDecrypt:
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		addr := args[0].(address.Address)
		passwd := args[1].(string)
		return w.WalletDecrypt(ctx, addr, passwd)
	case api.WalletIsEncrypt:
		if len(args) < 1 {
			return nil, xerrors.Errorf("args must is 1 for exec method, but get args is %v", len(args))
		}
		addr := args[0].(address.Address)
		// passwd := args[1].(string)
		return w.WalletIsEncrypt(ctx, addr)
	default:
		return nil, xerrors.Errorf("exec method is unknown")
	}
}

// wallet-security LocalWallet func
func (w *LocalWallet) WalletListEncrypt(context.Context) ([]api.AddrListEncrypt, error) {
	all, err := w.keystore.List()
	if err != nil {
		return nil, xerrors.Errorf("listing keystore: %w", err)
	}

	sort.Strings(all)

	seen := map[address.Address]struct{}{}
	out := make([]api.AddrListEncrypt, 0, len(all))
	for _, a := range all {
		var addr address.Address
		var err error
		if strings.HasPrefix(a, KNamePrefix) {
			name := strings.TrimPrefix(a, KNamePrefix)
			addr, err = address.NewFromString(name)
			if err != nil {
				return nil, xerrors.Errorf("converting name to address: %w", err)
			}

			if _, ok := seen[addr]; ok {
				continue // got duplicate with a different prefix
			}
			seen[addr] = struct{}{}

			key, err := w.findKey(addr)
			if err != nil {
				return nil, err
			}
			pk := key.PrivateKey
			var encrypt bool
			if pk[0] == 0xff && pk[1] == 0xff && pk[2] == 0xff && pk[3] == 0xff {
				encrypt = true
			}

			out = append(out, api.AddrListEncrypt{Addr: addr, Encrypt: encrypt})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Addr.String() < out[j].Addr.String()
	})

	return out, nil
}

func (w *LocalWallet) WalletExportEncrypt(ctx context.Context, addr address.Address, passwd string) (*types.KeyInfo, error) {
	if key.IsSetup() {
		if err := CheckPasswd([]byte(passwd)); err != nil {
			return nil, err
		}
	}

	k, err := w.findKey(addr)
	if err != nil {
		return nil, xerrors.Errorf("failed to find key to export: %w", err)
	}
	if k == nil {
		return nil, xerrors.Errorf("key not found")
	}

	// wallet-security 解密
	pk, err := key.UnMakeByte(k.PrivateKey)
	if err != nil {
		return nil, err
	}
	var pki types.KeyInfo
	pki.Type = k.Type
	pki.PrivateKey = pk
	return &pki, nil
}

func (w *LocalWallet) WalletDeleteEncrypt(ctx context.Context, addr address.Address, passwd string) error {
	if key.IsSetup() {
		if err := CheckPasswd([]byte(passwd)); err != nil {
			return err
		}
	}

	if err := w.walletDelete(ctx, addr); err != nil {
		return xerrors.Errorf("wallet delete: %w", err)
	}

	if def, err := w.GetDefault(); err == nil {
		if def == addr {
			w.deleteDefault()
		}
	}
	return nil
}

func (w *LocalWallet) WalletAddPasswd(ctx context.Context, passwd string, path string) error {

	if key.IsSetup() {
		err := xerrors.Errorf("passwd is setup, no need to setup again")
		log.Warn(err.Error())
		return err
	}

	if err := RegexpPasswd(passwd); err != nil {
		return err
	}

	err := SetupPasswd([]byte(passwd), path)
	if err != nil {
		return err
	}

	return nil
}

func (w *LocalWallet) WalletResetPasswd(ctx context.Context, oldPasswd, newPasswd string) (bool, error) {
	if !key.IsSetup() {
		return false, xerrors.Errorf("passwd is not setup")
	}

	if err := CheckPasswd([]byte(oldPasswd)); err != nil {
		return false, err
	}

	if err := RegexpPasswd(newPasswd); err != nil {
		return false, err
	}

	addr_list, err := w.WalletList(ctx)
	if err != nil {
		return false, err
	}

	addr_all := make(map[address.Address]*KeyInfo)
	for _, v := range addr_list {
		k, err := w.findKey(v)
		if err != nil {
			return false, fmt.Errorf("failed to find key to export: %w", err)
		}
		if k == nil {
			return false, fmt.Errorf("key not found")
		}

		pk, err := key.UnMakeByte(k.PrivateKey)
		if err != nil {
			return false, err
		}

		ki := &KeyInfo{
			KeyInfo: types.KeyInfo{
				Type:       k.Type,
				PrivateKey: pk,
			},
			Enc: key.IsPrivateKeyEnc(k.PrivateKey),
		}
		addr_all[v] = ki

		err = w.WalletDeleteKey2(v)
		if err != nil {
			return false, err
		}
	}

	setDefault := true
	defalutAddr, err := w.GetDefault()
	if err != nil {
		setDefault = false
	}

	err = ResetPasswd([]byte(newPasswd))
	if err != nil {
		return false, err
	}

	for addr, v := range addr_all {
		k, err := key.NewKey(v.KeyInfo)
		if err != nil {
			return false, fmt.Errorf("failed to make key: %w", err)
		}

		if v.Enc {
			pk, err := key.MakeByte(k.PrivateKey)
			if err != nil {
				return false, err
			}
			k.PrivateKey = pk
		}

		if err := w.keystore.Put(KNamePrefix+k.Address.String(), k.KeyInfo); err != nil {
			return false, fmt.Errorf("saving to keystore: %w", err)
		}

		if k.Address != addr {
			return false, fmt.Errorf("import error")
		}
	}

	if setDefault {
		err = w.SetDefault(defalutAddr)
		if err != nil {
			return false, err
		}
	}

	w.WalletClearCache()

	return true, nil
}

func (w *LocalWallet) WalletClearPasswd(ctx context.Context, passwd string) (bool, error) {
	if !key.IsSetup() {
		return false, xerrors.Errorf("passwd is not setup")
	}

	if err := CheckPasswd([]byte(passwd)); err != nil {
		return false, err
	}

	addr_list, err := w.WalletList(ctx)
	if err != nil {
		return false, err
	}
	addr_all := make(map[address.Address]*types.KeyInfo)
	for _, v := range addr_list {
		addr_all[v], err = w.WalletExportEncrypt(ctx, v, passwd)
		if err != nil {
			return false, err
		}
		err = w.WalletDeleteKey2(v)
		if err != nil {
			return false, err
		}
	}

	setDefault := true
	defalutAddr, err := w.GetDefault()
	if err != nil {
		setDefault = false
	}

	err = ClearPasswd()
	if err != nil {
		return false, err
	}

	for k, v := range addr_all {
		addr, err := w.WalletImport(ctx, v)
		if err != nil {
			return false, nil
		} else if addr != k {
			return false, xerrors.Errorf("import error")
		}
	}

	if setDefault {
		err = w.SetDefault(defalutAddr)
		if err != nil {
			return false, err
		}
	}

	w.WalletClearCache()
	return true, nil
}

func (w *LocalWallet) WalletCheckPasswd(ctx context.Context, passwd string) bool {
	if err := CheckPasswd([]byte(passwd)); err != nil {
		return false
	}
	return true
}

func (w *LocalWallet) WalletEncrypt(ctx context.Context, addr address.Address) (bool, error) {
	if !key.IsSetup() {
		return false, xerrors.Errorf("passwd is not setup")
	}

	addr_list, err := w.WalletList(ctx)
	if err != nil {
		return false, err
	}
	addr_all := make(map[address.Address]*KeyInfo)
	for _, v := range addr_list {
		if v == addr {
			k, err := w.findKey(v)
			if err != nil {
				return false, fmt.Errorf("failed to find key to export: %w", err)
			}
			if k == nil {
				return false, fmt.Errorf("key not found")
			}

			pk, err := key.UnMakeByte(k.PrivateKey)
			if err != nil {
				return false, err
			}

			ki := &KeyInfo{
				KeyInfo: types.KeyInfo{
					Type:       k.Type,
					PrivateKey: pk,
				},
				Enc: key.IsPrivateKeyEnc(k.PrivateKey),
			}
			addr_all[v] = ki

			err = w.WalletDeleteKey2(v)
			if err != nil {
				return false, err
			}
		}
	}

	setDefault := true
	defalutAddr, err := w.GetDefault()
	if err != nil {
		setDefault = false
	}

	for k, v := range addr_all {
		if k == addr {
			k, err := key.NewKey(v.KeyInfo)
			if err != nil {
				return false, fmt.Errorf("failed to make key: %w", err)
			}

			pk, err := key.MakeByte(k.PrivateKey)
			if err != nil {
				return false, err
			}
			k.PrivateKey = pk

			if err := w.keystore.Put(KNamePrefix+k.Address.String(), k.KeyInfo); err != nil {
				return false, fmt.Errorf("saving to keystore: %w", err)
			}

			if k.Address != addr {
				return false, fmt.Errorf("import error")
			}
		}
	}

	if setDefault {
		err = w.SetDefault(defalutAddr)
		if err != nil {
			return false, err
		}
	}

	w.WalletClearCache()
	return true, nil
}

func (w *LocalWallet) WalletDecrypt(ctx context.Context, addr address.Address, passwd string) (bool, error) {
	if !key.IsSetup() {
		return false, xerrors.Errorf("passwd is not setup")
	}

	if err := CheckPasswd([]byte(passwd)); err != nil {
		return false, err
	}

	addr_list, err := w.WalletList(ctx)
	if err != nil {
		return false, err
	}

	addr_all := make(map[address.Address]*KeyInfo)
	for _, v := range addr_list {
		if v == addr {
			k, err := w.findKey(v)
			if err != nil {
				return false, fmt.Errorf("failed to find key to export: %w", err)
			}
			if k == nil {
				return false, fmt.Errorf("key not found")
			}

			pk, err := key.UnMakeByte(k.PrivateKey)
			if err != nil {
				return false, err
			}

			ki := &KeyInfo{
				KeyInfo: types.KeyInfo{
					Type:       k.Type,
					PrivateKey: pk,
				},
				Enc: key.IsPrivateKeyEnc(k.PrivateKey),
			}
			addr_all[v] = ki

			err = w.WalletDeleteKey2(v)
			if err != nil {
				return false, err
			}
		}
	}

	setDefault := true
	defalutAddr, err := w.GetDefault()
	if err != nil {
		setDefault = false
	}

	for k, v := range addr_all {
		if k == addr {
			k, err := key.NewKey(v.KeyInfo)
			if err != nil {
				return false, fmt.Errorf("failed to make key: %w", err)
			}

			if err := w.keystore.Put(KNamePrefix+k.Address.String(), k.KeyInfo); err != nil {
				return false, fmt.Errorf("saving to keystore: %w", err)
			}

			if k.Address != addr {
				return false, fmt.Errorf("import error")
			}
		}
	}

	if setDefault {
		err = w.SetDefault(defalutAddr)
		if err != nil {
			return false, err
		}
	}

	w.WalletClearCache()
	return true, nil
}

func (w *LocalWallet) WalletIsEncrypt(ctx context.Context, addr address.Address) (bool, error) {
	if !key.IsSetup() {
		log.Infof("passwd is not setup")
		return false, nil
	}

	addr_list, err := w.WalletList(ctx)
	if err != nil {
		return false, err
	}

	for _, v := range addr_list {
		if v == addr {
			ki, err := w.tryFind(v)
			if err != nil {
				if xerrors.Is(err, types.ErrKeyInfoNotFound) {
					return false, nil
				}
				return false, xerrors.Errorf("getting from keystore: %w", err)
			}

			// wallet-security 解密
			if bytes.Equal(ki.PrivateKey[:4], key.AddrPrefix) {
				return true, nil
			} else {
				return false, nil
			}
		}
	}
	return false, nil
}

func (w *LocalWallet) WalletDeleteKey2(addr address.Address) error {
	k, err := w.findKey(addr)
	if err != nil {
		return xerrors.Errorf("failed to delete key %s : %w", addr, err)
	}

	if err := w.keystore.Delete(KNamePrefix + k.Address.String()); err != nil {
		return xerrors.Errorf("failed to delete key %s: %w", addr, err)
	}

	return nil
}

func (w *LocalWallet) WalletClearCache() {
	w.keys = make(map[address.Address]*key.Key)
}
