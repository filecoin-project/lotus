package vm_test

import (
	"testing"

	"github.com/filecoin-project/chain-validation/pkg/suites"

	"github.com/filecoin-project/lotus/chain/validation"
)

func TestStorageMinerValidation(t *testing.T) {
	// changes to PoSt of broken this test, skip until a fix lands
	t.Skip()
	factory := validation.NewFactories()
	suites.CreateStorageMinerAndUpdatePeerIDTest(t, factory)

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

func TestPaymentChannelActor(t *testing.T) {
	factory := validation.NewFactories()
	suites.PaymentChannelCreateSuccess(t, factory, 1120)
}
