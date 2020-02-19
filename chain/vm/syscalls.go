package vm

import (
	"context"

	ffi "github.com/filecoin-project/filecoin-ffi"
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
		VerifyFallbackPost: func(ctx context.Context,
			sectorSize uint64,
			sectorInfo []types.PublicSectorInfo,
			challengeSeed []byte,
			proof []byte,
			candidates []types.Candidate,
			proverID address.Address,
			faults uint64) (bool, error) {

			sI := make([]ffi.PublicSectorInfo, len(sectorInfo))
			for i, v := range sectorInfo {
				sI[i] = ffi.PublicSectorInfo{
					SectorID: v.SectorID,
					CommR:    v.CommR,
				}
			}

			cand := make([]sectorbuilder.EPostCandidate, len(candidates))
			for i, v := range candidates {
				cand[i] = sectorbuilder.EPostCandidate{
					SectorID:             v.SectorID,
					PartialTicket:        v.PartialTicket,
					Ticket:               v.Ticket,
					SectorChallengeIndex: v.SectorChallengeIndex,
				}
			}

			return verifier.VerifyFallbackPost(
				ctx,
				sectorSize,
				sectorbuilder.NewSortedPublicSectorInfo(sI),
				challengeSeed,
				proof,
				cand,
				proverID,
				faults,
			)

		},
	}
}
