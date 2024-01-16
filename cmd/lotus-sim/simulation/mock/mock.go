package mock

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	tutils "github.com/filecoin-project/specs-actors/v5/support/testing"

	"github.com/filecoin-project/lotus/chain/verifier"
)

// Ideally, we'd use extern/sealer/mock. Unfortunately, those mocks are a bit _too_ accurate
// and would force us to load sector info for window post proofs.

const (
	mockSealProofPrefix          = "valid seal proof:"
	mockAggregateSealProofPrefix = "valid aggregate seal proof:"
	mockPoStProofPrefix          = "valid post proof:"
)

var log = logging.Logger("simulation-mock")

// mockVerifier is a simple mock for verifying "fake" proofs.
type mockVerifier struct{}

var Verifier verifier.Verifier = mockVerifier{}

func (mockVerifier) VerifySeal(proof prooftypes.SealVerifyInfo) (bool, error) {
	addr, err := address.NewIDAddress(uint64(proof.Miner))
	if err != nil {
		return false, err
	}
	mockProof, err := MockSealProof(proof.SealProof, addr)
	if err != nil {
		return false, err
	}
	if bytes.Equal(proof.Proof, mockProof) {
		return true, nil
	}
	log.Debugw("invalid seal proof", "expected", mockProof, "actual", proof.Proof, "miner", addr)
	return false, nil
}

func (mockVerifier) VerifyAggregateSeals(aggregate prooftypes.AggregateSealVerifyProofAndInfos) (bool, error) {
	addr, err := address.NewIDAddress(uint64(aggregate.Miner))
	if err != nil {
		return false, err
	}
	mockProof, err := MockAggregateSealProof(aggregate.SealProof, addr, len(aggregate.Infos))
	if err != nil {
		return false, err
	}
	if bytes.Equal(aggregate.Proof, mockProof) {
		return true, nil
	}
	log.Debugw("invalid aggregate seal proof",
		"expected", mockProof,
		"actual", aggregate.Proof,
		"count", len(aggregate.Infos),
		"miner", addr,
	)
	return false, nil
}

// TODO: do the thing
func (mockVerifier) VerifyReplicaUpdate(update prooftypes.ReplicaUpdateInfo) (bool, error) {
	return false, nil
}

func (mockVerifier) VerifyWinningPoSt(ctx context.Context, info prooftypes.WinningPoStVerifyInfo) (bool, error) {
	panic("should not be called")
}
func (mockVerifier) VerifyWindowPoSt(ctx context.Context, info prooftypes.WindowPoStVerifyInfo) (bool, error) {
	if len(info.Proofs) != 1 {
		return false, fmt.Errorf("expected exactly one proof")
	}
	proof := info.Proofs[0]
	addr, err := address.NewIDAddress(uint64(info.Prover))
	if err != nil {
		return false, err
	}
	mockProof, err := MockWindowPoStProof(proof.PoStProof, addr)
	if err != nil {
		return false, err
	}
	if bytes.Equal(proof.ProofBytes, mockProof) {
		return true, nil
	}

	log.Debugw("invalid window post proof",
		"expected", mockProof,
		"actual", info.Proofs[0],
		"miner", addr,
	)
	return false, nil
}

func (mockVerifier) GenerateWinningPoStSectorChallenge(context.Context, abi.RegisteredPoStProof, abi.ActorID, abi.PoStRandomness, uint64) ([]uint64, error) {
	panic("should not be called")
}

// MockSealProof generates a mock "seal" proof tied to the specified proof type and the given miner.
func MockSealProof(proofType abi.RegisteredSealProof, minerAddr address.Address) ([]byte, error) {
	plen, err := proofType.ProofSize()
	if err != nil {
		return nil, err
	}
	proof := make([]byte, plen)
	i := copy(proof, mockSealProofPrefix)
	binary.BigEndian.PutUint64(proof[i:], uint64(proofType))
	i += 8
	i += copy(proof[i:], minerAddr.Bytes())
	return proof, nil
}

// MockAggregateSealProof generates a mock "seal" aggregate proof tied to the specified proof type,
// the given miner, and the number of proven sectors.
func MockAggregateSealProof(proofType abi.RegisteredSealProof, minerAddr address.Address, count int) ([]byte, error) {
	proof := make([]byte, aggProofLen(count))
	i := copy(proof, mockAggregateSealProofPrefix)
	binary.BigEndian.PutUint64(proof[i:], uint64(proofType))
	i += 8
	binary.BigEndian.PutUint64(proof[i:], uint64(count))
	i += 8
	i += copy(proof[i:], minerAddr.Bytes())

	return proof, nil
}

// MockWindowPoStProof generates a mock "window post" proof tied to the specified proof type, and the
// given miner.
func MockWindowPoStProof(proofType abi.RegisteredPoStProof, minerAddr address.Address) ([]byte, error) {
	plen, err := proofType.ProofSize()
	if err != nil {
		return nil, err
	}
	proof := make([]byte, plen)
	i := copy(proof, mockPoStProofPrefix)
	i += copy(proof[i:], minerAddr.Bytes())
	return proof, nil
}

// MockCommR generates a "fake" but valid CommR for a sector. It is unique for the given sector/miner.
func MockCommR(minerAddr address.Address, sno abi.SectorNumber) cid.Cid {
	return tutils.MakeCID(fmt.Sprintf("%s:%d", minerAddr, sno), &miner5.SealedCIDPrefix)
}

// TODO: dedup
func aggProofLen(nproofs int) int {
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
