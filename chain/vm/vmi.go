package vm

import (
	"context"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

type VMI interface {
	ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error)
	ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error)
	Flush(ctx context.Context) (cid.Cid, error)
}
