package api

import (
	"context"
)

// wallet-security WalletStructExt WalletCustomMethod
type WalletStructExt struct {
	Internal struct {
		WalletCustomMethod func(p0 context.Context, p1 WalletMethod, p2 []interface{}) (interface{}, error) `perm:"admin"`
	}
}

func (s *WalletStructExt) WalletCustomMethod(p0 context.Context, p1 WalletMethod, p2 []interface{}) (interface{}, error) {
	if s.Internal.WalletCustomMethod == nil {
		return nil, ErrNotSupported
	}
	return s.Internal.WalletCustomMethod(p0, p1, p2)
}
func (s *WalletStub) WalletCustomMethod(p0 context.Context, p1 WalletMethod, p2 []interface{}) (interface{}, error) {
	return nil, ErrNotSupported
}

var _ WalletExt = new(WalletStructExt)
