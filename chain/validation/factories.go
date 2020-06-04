package validation

import (
	"github.com/filecoin-project/specs-actors/actors/runtime"

	vstate "github.com/filecoin-project/chain-validation/state"
)

type Factories struct {
	*Applier
}

var _ vstate.Factories = &Factories{}

func NewFactories() *Factories {
	return &Factories{}
}

func (f *Factories) NewStateAndApplier(syscalls runtime.Syscalls) (vstate.VMWrapper, vstate.Applier) {
	st := NewState()
	return st, NewApplier(st, syscalls)
}

func (f *Factories) NewKeyManager() vstate.KeyManager {
	return newKeyManager()
}

func (f *Factories) NewValidationConfig() vstate.ValidationConfig {
	trackGas := true
	checkExit := true
	checkRet := true
	checkState := true
	return NewConfig(trackGas, checkExit, checkRet, checkState)
}
