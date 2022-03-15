package vm

import (
	"context"
	"os"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

type Interface interface {
	// Applies the given message onto the VM's current state, returning the result of the execution
	ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error)
	// Same as above but for system messages (the Cron invocation and block reward payments).
	// Must NEVER fail.
	ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error)
	// Flush all buffered objects into the state store provided to the VM at construction.
	Flush(ctx context.Context) (cid.Cid, error)
}

func NewVM(ctx context.Context, opts *VMOpts) (Interface, error) {
	if os.Getenv("LOTUS_USE_FVM_EXPERIMENTAL") == "1" {
		return NewFVM(ctx, opts)
	}

	return NewLegacyVM(ctx, opts)
}
