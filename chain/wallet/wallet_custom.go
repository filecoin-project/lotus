package wallet

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"golang.org/x/xerrors"
)

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
		if len(args) < 2 {
			return nil, xerrors.Errorf("args must is 2 for exec method, but get args is %v", len(args))
		}
		addr := args[0].(address.Address)
		passwd := args[1].(string)
		return w.WalletEncrypt(ctx, addr, passwd)
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
		return w.WalletIsEncrypt(ctx, addr)
	default:
		return nil, xerrors.Errorf("exec method is unknown")
	}
}
