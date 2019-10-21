package validation

import "github.com/filecoin-project/chain-validation/pkg/suites"

type driver struct {
	*MessageFactory
	*StateFactory
	*Applier
}

var _ suites.Driver = &driver{}

func NewDriver() *driver {
	stateFactory := NewStateFactory()
	msgFactory := NewMessageFactory(stateFactory.Signer())
	applier := NewApplier()
	return &driver{msgFactory, stateFactory, applier}
}
