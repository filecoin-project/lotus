package validation

import (
	vstate "github.com/filecoin-project/chain-validation/pkg/state"
	"github.com/filecoin-project/chain-validation/pkg/suites"
)

type factories struct {
	*Applier
}

var _ suites.Factories = &factories{}

func NewFactories() *factories {
	applier := NewApplier()
	return &factories{applier}
}

func (f *factories) NewState() vstate.Wrapper {
	return NewState()
}
