package simulation

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
)

// Ideally, we'd use extern/sector-storage/mock. Unfortunately, those mocks are a bit _too_ accurate
// and would force us to load sector info for window post proofs.

const (
	mockSealProofPrefix          = "valid seal proof:"
	mockAggregateSealProofPrefix = "valid aggregate seal proof:"
	mockPoStProofPrefix          = "valid post proof:"
)

func mockSealProof(proofType abi.RegisteredSealProof, minerAddr address.Address) ([]byte, error) {
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

func mockAggregateSealProof(proofType abi.RegisteredSealProof, minerAddr address.Address, count int) ([]byte, error) {
	proof := make([]byte, aggProofLen(count))
	i := copy(proof, mockAggregateSealProofPrefix)
	binary.BigEndian.PutUint64(proof[i:], uint64(proofType))
	i += 8
	binary.BigEndian.PutUint64(proof[i:], uint64(count))
	i += 8
	i += copy(proof[i:], minerAddr.Bytes())

	return proof, nil
}

func mockWpostProof(proofType abi.RegisteredPoStProof, minerAddr address.Address) ([]byte, error) {
	plen, err := proofType.ProofSize()
	if err != nil {
		return nil, err
	}
	proof := make([]byte, plen)
	i := copy(proof, mockPoStProofPrefix)
	i += copy(proof[i:], minerAddr.Bytes())
	return proof, nil
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

type mockVerifier struct{}

func (mockVerifier) VerifySeal(proof proof5.SealVerifyInfo) (bool, error) {
	addr, err := address.NewIDAddress(uint64(proof.Miner))
	if err != nil {
		return false, err
	}
	mockProof, err := mockSealProof(proof.SealProof, addr)
	if err != nil {
		return false, err
	}
	return bytes.Equal(proof.Proof, mockProof), nil
}

func (mockVerifier) VerifyAggregateSeals(aggregate proof5.AggregateSealVerifyProofAndInfos) (bool, error) {
	addr, err := address.NewIDAddress(uint64(aggregate.Miner))
	if err != nil {
		return false, err
	}
	mockProof, err := mockAggregateSealProof(aggregate.SealProof, addr, len(aggregate.Infos))
	if err != nil {
		return false, err
	}
	return bytes.Equal(aggregate.Proof, mockProof), nil
}
func (mockVerifier) VerifyWinningPoSt(ctx context.Context, info proof5.WinningPoStVerifyInfo) (bool, error) {
	panic("should not be called")
}
func (mockVerifier) VerifyWindowPoSt(ctx context.Context, info proof5.WindowPoStVerifyInfo) (bool, error) {
	if len(info.Proofs) != 1 {
		return false, fmt.Errorf("expected exactly one proof")
	}
	proof := info.Proofs[0]
	addr, err := address.NewIDAddress(uint64(info.Prover))
	if err != nil {
		return false, err
	}
	mockProof, err := mockWpostProof(proof.PoStProof, addr)
	if err != nil {
		return false, err
	}
	return bytes.Equal(proof.ProofBytes, mockProof), nil
}

func (mockVerifier) GenerateWinningPoStSectorChallenge(context.Context, abi.RegisteredPoStProof, abi.ActorID, abi.PoStRandomness, uint64) ([]uint64, error) {
	panic("should not be called")
}
