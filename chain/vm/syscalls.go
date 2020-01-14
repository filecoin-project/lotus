package vm

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
	"go.opencensus.io/trace"
)

// Actual type is defined in chain/types/vmcontext.go because the VMContext interface is there

func Syscalls(verifier sectorbuilder.Verifier) *types.VMSyscalls {
	return &types.VMSyscalls{
		ValidatePoRep: func(ctx context.Context, maddr address.Address, ssize uint64, commD, commR, ticket, proof, seed []byte, sectorID uint64) (bool, actors.ActorError) {
			_, span := trace.StartSpan(ctx, "ValidatePoRep")
			defer span.End()
			ok, err := verifier.VerifySeal(ssize, commR, commD, maddr, ticket, seed, sectorID, proof)
			if err != nil {
				return false, aerrors.Absorb(err, 25, "verify seal failed")
			}

			return ok, nil
		},
		VerifyFallbackPost: verifier.VerifyFallbackPost,
	}
}
