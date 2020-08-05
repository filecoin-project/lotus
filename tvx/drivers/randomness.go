package drivers

import (
	"context"

	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	acrypto "github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

// RandomnessSource provides randomness to actors.
type RandomnessSource interface {
	Randomness(ctx context.Context, tag acrypto.DomainSeparationTag, epoch abi.ChainEpoch, entropy []byte) (abi.Randomness, error)
}

// Specifies a domain for randomness generation.
type RandomnessType int

type randWrapper struct {
	rnd RandomnessSource
}

func (w *randWrapper) GetRandomness(ctx context.Context, pers acrypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return w.rnd.Randomness(ctx, pers, round, entropy)
}

type vmRand struct {
}

func (*vmRand) GetRandomness(ctx context.Context, dst acrypto.DomainSeparationTag, h abi.ChainEpoch, input []byte) ([]byte, error) {
	panic("implement me")
}

type fakeRandSrc struct {
}

func (r fakeRandSrc) Randomness(_ context.Context, _ acrypto.DomainSeparationTag, _ abi_spec.ChainEpoch, _ []byte) (abi_spec.Randomness, error) {
	return abi_spec.Randomness("sausages"), nil
}

func NewRandomnessSource() RandomnessSource {
	return &fakeRandSrc{}
}
