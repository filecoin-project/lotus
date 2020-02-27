package validation

import (
	"context"

	vstate "github.com/filecoin-project/chain-validation/state"
	acrypto "github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type Factories struct {
	*Applier
}

var _ vstate.Factories = &Factories{}

func NewFactories() *Factories {
	applier := NewApplier()
	return &Factories{applier}
}

func (f *Factories) NewState() vstate.VMWrapper {
	return NewState()
}

func (f *Factories) NewKeyManager() vstate.KeyManager {
	return newKeyManager()
}

type fakeRandSrc struct {
}

func (r fakeRandSrc) Randomness(_ context.Context, _ acrypto.DomainSeparationTag, _ abi.ChainEpoch, _ []byte) (abi.Randomness, error) {
	panic("implement me")
}

func (f *Factories) NewRandomnessSource() vstate.RandomnessSource {
	return &fakeRandSrc{}
}

func (f *Factories) NewValidationConfig() vstate.ValidationConfig {
	return &ValidationConfig{
		trackGas:         false,
		checkExitCode:    true,
		checkReturnValue: true,
	}
}
