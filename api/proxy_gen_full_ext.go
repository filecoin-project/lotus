package api

import (
	"context"
)

// wallet-security FullNodeStructExt WalletCustomMethod
type FullNodeStructExt struct {
	Internal struct {
		WalletCustomMethod func(p0 context.Context, p1 WalletMethod, p2 []interface{}) (interface{}, error) `perm:"admin"`
	}
}

func (s *FullNodeStructExt) WalletCustomMethod(p0 context.Context, p1 WalletMethod, p2 []interface{}) (interface{}, error) {
	if s.Internal.WalletCustomMethod == nil {
		return nil, ErrNotSupported
	}
	return s.Internal.WalletCustomMethod(p0, p1, p2)
}
func (s *FullNodeStub) WalletCustomMethod(p0 context.Context, p1 WalletMethod, p2 []interface{}) (interface{}, error) {
	return nil, ErrNotSupported
}

var _ FullNodeExt = new(FullNodeStructExt)
