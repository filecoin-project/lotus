package common

import (
	"context"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func (a *CommonAPI) NetBlockAdd(ctx context.Context, acl dtypes.NetBlockList) error {
	// TODO
	return nil
}

func (a *CommonAPI) NetBlockRemove(ctx context.Context, acl dtypes.NetBlockList) error {
	// TODO
	return nil
}

func (a *CommonAPI) NetBlockList(ctx context.Context) (dtypes.NetBlockList, error) {
	// TODO
	return dtypes.NetBlockList{}, nil
}
