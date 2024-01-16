package mock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/chain/verifier"
)

var _ verifier.Verifier = MockVerifier

type mockVerifier struct{}

var MockVerifier = mockVerifier{}

func (mockVerifier) VerifySeal(svi prooftypes.SealVerifyInfo) (bool, error) {
	plen, err := svi.SealProof.ProofSize()
	if err != nil {
		return false, err
	}

	if len(svi.Proof) != int(plen) {
		return false, nil
	}

	// only the first 32 bytes, the rest are 0.
	for i, b := range svi.Proof[:32] {
		// unsealed+sealed-seed*ticket
		if b != svi.UnsealedCID.Bytes()[i]+svi.SealedCID.Bytes()[31-i]-svi.InteractiveRandomness[i]*svi.Randomness[i] {
			return false, nil
		}
	}

	return true, nil
}

func (mockVerifier) VerifyAggregateSeals(aggregate prooftypes.AggregateSealVerifyProofAndInfos) (bool, error) {
	out := make([]byte, aggLen(len(aggregate.Infos)))
	for pi, svi := range aggregate.Infos {
		for i := 0; i < 32; i++ {
			b := svi.UnsealedCID.Bytes()[i] + svi.SealedCID.Bytes()[31-i] - svi.InteractiveRandomness[i]*svi.Randomness[i] // raw proof byte

			b *= uint8(pi) // with aggregate index
			out[i] += b
		}
	}

	ok := bytes.Equal(aggregate.Proof, out)
	return ok, nil
}

func (mockVerifier) VerifyReplicaUpdate(update prooftypes.ReplicaUpdateInfo) (bool, error) {
	return true, nil
}

func (mockVerifier) VerifyWinningPoSt(ctx context.Context, info prooftypes.WinningPoStVerifyInfo) (bool, error) {
	info.Randomness[31] &= 0x3f
	return true, nil
}

func (mockVerifier) VerifyWindowPoSt(ctx context.Context, info prooftypes.WindowPoStVerifyInfo) (bool, error) {
	if len(info.Proofs) != 1 {
		return false, fmt.Errorf("expected 1 proof entry")
	}

	proof := info.Proofs[0]

	expected := generateFakePoStProof(info.ChallengedSectors, info.Randomness)
	if !bytes.Equal(proof.ProofBytes, expected) {
		return false, fmt.Errorf("bad proof")
	}
	return true, nil
}

func (mockVerifier) GenerateWinningPoStSectorChallenge(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, eligibleSectorCount uint64) ([]uint64, error) {
	return []uint64{0}, nil
}

func generateFakePoStProof(sectorInfo []prooftypes.SectorInfo, randomness abi.PoStRandomness) []byte {
	randomness[31] &= 0x3f

	hasher := sha256.New()
	_, _ = hasher.Write(randomness)
	for _, info := range sectorInfo {
		err := info.MarshalCBOR(hasher)
		if err != nil {
			panic(err)
		}
	}
	return hasher.Sum(nil)
}

func aggLen(nproofs int) int {
	switch {
	case nproofs <= 8:
		return 11220
	case nproofs <= 16:
		return 14196
	case nproofs <= 32:
		return 17172
	case nproofs <= 64:
		return 20148
	case nproofs <= 128:
		return 23124
	case nproofs <= 256:
		return 26100
	case nproofs <= 512:
		return 29076
	case nproofs <= 1024:
		return 32052
	case nproofs <= 2048:
		return 35028
	case nproofs <= 4096:
		return 38004
	case nproofs <= 8192:
		return 40980
	default:
		panic("too many proofs")
	}
}
