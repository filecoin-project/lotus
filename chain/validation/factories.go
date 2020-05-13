package validation

import (
	"context"

	vstate "github.com/filecoin-project/chain-validation/state"
	"github.com/filecoin-project/specs-actors/actors/abi"
	acrypto "github.com/filecoin-project/specs-actors/actors/crypto"
)

type Factories struct {
	*Applier
}

var _ vstate.Factories = &Factories{}

func NewFactories() *Factories {
	return &Factories{}
}

func (f *Factories) NewStateAndApplier() (vstate.VMWrapper, vstate.Applier) {
	st := NewState()
	return st, NewApplier(st)
}

func (f *Factories) NewKeyManager() vstate.KeyManager {
	return newKeyManager()
}

type fakeRandSrc struct {
}

func (r fakeRandSrc) Randomness(_ context.Context, _ acrypto.DomainSeparationTag, _ abi.ChainEpoch, _ []byte) (abi.Randomness, error) {
	return abi.Randomness("sausages"), nil
}

func (f *Factories) NewRandomnessSource() vstate.RandomnessSource {
	return &fakeRandSrc{}
}

func (f *Factories) NewValidationConfig() vstate.ValidationConfig {
	trackGas := true
	checkExit := true
	checkRet := true
	checkState := true
	return NewConfig(trackGas, checkExit, checkRet, checkState)
}
