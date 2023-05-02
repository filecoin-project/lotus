package api

import (
	"context"
)

// wallet-security FullNodeExt WalletCustomMethod
type FullNodeExt interface {
	// WalletCustomMethod wallet extension operation
	WalletCustomMethod(context.Context, WalletMethod, []interface{}) (interface{}, error) //perm:admin
}
