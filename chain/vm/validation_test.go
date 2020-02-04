package vm_test

import (
	"testing"

	"github.com/filecoin-project/chain-validation/pkg/suites"

	"github.com/filecoin-project/lotus/chain/validation"
)

func TestStorageMinerValidation(t *testing.T) {
	t.SkipNow()
	factory := validation.NewFactories()
	suites.CreateStorageMinerAndUpdatePeerID(t, factory)

}

func TestValueTransfer(t *testing.T) {
	factory := validation.NewFactories()
	suites.AccountValueTransferSuccess(t, factory, 126)
	suites.AccountValueTransferZeroFunds(t, factory, 112)
	suites.AccountValueTransferOverBalanceNonZero(t, factory, 0)
	suites.AccountValueTransferOverBalanceZero(t, factory, 0)
	suites.AccountValueTransferToSelf(t, factory, 0)
	suites.AccountValueTransferFromKnownToUnknownAccount(t, factory, 0)
	suites.AccountValueTransferFromUnknownToKnownAccount(t, factory, 0)
	suites.AccountValueTransferFromUnknownToUnknownAccount(t, factory, 0)
}

func TestMultiSig(t *testing.T) {
	t.SkipNow()
	factory := validation.NewFactories()
	suites.MultiSigActorConstructor(t, factory)
	suites.MultiSigActorProposeApprove(t, factory)
	suites.MultiSigActorProposeCancel(t, factory)
}
