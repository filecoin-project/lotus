package api

import (
	"context"

	"github.com/filecoin-project/go-address"
)

// wallet-security WalletExt WalletCustomMethod
type WalletExt interface {
	WalletCustomMethod(context.Context, WalletMethod, []interface{}) (interface{}, error) //perm:admin
}

// wallet-security AddrListEncrypt stuct
type AddrListEncrypt struct {
	Addr    address.Address
	Encrypt bool
}
type WalletMethod int64

const (
	Unknown            WalletMethod = 0
	WalletListForEnc   WalletMethod = 1
	WalletExportForEnc WalletMethod = 2
	WalletDeleteForEnc WalletMethod = 3
	WalletAddPasswd    WalletMethod = 4
	WalletResetPasswd  WalletMethod = 5
	WalletClearPasswd  WalletMethod = 6
	WalletCheckPasswd  WalletMethod = 7
	WalletEncrypt      WalletMethod = 8
	WalletDecrypt      WalletMethod = 9
	WalletIsEncrypt    WalletMethod = 10
)

var WalletMethodStr = map[WalletMethod]string{
	Unknown:            "Unknown",
	WalletListForEnc:   "WalletListForEnc",
	WalletExportForEnc: "WalletExportForEnc",
	WalletDeleteForEnc: "WalletDeleteForEnc",
	WalletAddPasswd:    "WalletAddPasswd",
	WalletResetPasswd:  "WalletResetPasswd",
	WalletClearPasswd:  "WalletClearPasswd",
	WalletCheckPasswd:  "WalletCheckPasswd",
	WalletEncrypt:      "WalletEncrypt",
	WalletDecrypt:      "WalletDecrypt",
	WalletIsEncrypt:    "WalletIsEncrypt",
}

func (w WalletMethod) String() string {
	return WalletMethodStr[w]
}
