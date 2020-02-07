package vm_test

import (
	"testing"

	"github.com/filecoin-project/chain-validation/pkg/suites"

	"github.com/filecoin-project/lotus/chain/validation"
)

func TestValueTransfer(t *testing.T) {
	factory := validation.NewFactories()
	suites.AccountValueTransferSuccess(t, factory, 128)
	suites.AccountValueTransferZeroFunds(t, factory, 114)
	suites.AccountValueTransferOverBalanceNonZero(t, factory, 0)
	suites.AccountValueTransferOverBalanceZero(t, factory, 0)
	suites.AccountValueTransferToSelf(t, factory, 0)
	suites.AccountValueTransferFromKnownToUnknownAccount(t, factory, 0)
	suites.AccountValueTransferFromUnknownToKnownAccount(t, factory, 0)
	suites.AccountValueTransferFromUnknownToUnknownAccount(t, factory, 0)
}

func TestMultiSig(t *testing.T) {
	factory := validation.NewFactories()
	suites.MultiSigActorConstructor(t, factory)
	suites.MultiSigActorProposeApprove(t, factory)
	suites.MultiSigActorProposeCancel(t, factory)
}
