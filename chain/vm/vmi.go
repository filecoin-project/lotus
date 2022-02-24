package vm

import (
	"context"
	"os"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

type VMI interface {
	ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error)
	ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error)
	Flush(ctx context.Context) (cid.Cid, error)
}

func NewVM(ctx context.Context, opts *VMOpts) (VMI, error) {
	if os.Getenv("LOTUS_USE_FVM_DOESNT_WORK_YET") == "1" {
		return NewFVM(ctx, opts)
	}

	return NewLotusVM(ctx, opts)
}
