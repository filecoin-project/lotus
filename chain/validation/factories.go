package validation

import (
	vchain "github.com/filecoin-project/chain-validation/pkg/chain"
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

func (f *factories) NewMessageFactory(wrapper vstate.Wrapper) vchain.MessageFactory {
	signer := wrapper.(*StateWrapper).Signer()
	return NewMessageFactory(signer)
}
