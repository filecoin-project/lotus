package validation

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/chain-validation/pkg/chain"
	"github.com/filecoin-project/chain-validation/pkg/state"
	"github.com/filecoin-project/chain-validation/pkg/suites"
)

// A basic example validation test.
// At present this code is verbose and demonstrates the opportunity for helper methods.
func TestLotusExample(t *testing.T) {
	factory := NewFactories()
	drv := suites.NewStateDriver(t, factory.NewState())

	_, _, err := drv.State().SetSingletonActor(state.InitAddress, big.NewInt(0))
	require.NoError(t, err)

	alice := drv.NewAccountActor(2000)
	bob := drv.NewAccountActor(0)
	miner := drv.NewAccountActor(0) // Miner owner


	gasPrice := big.NewInt(1)
	gasLimit := state.GasUnit(1000)

	producer := chain.NewMessageProducer(factory.NewMessageFactory(drv.State()), gasLimit, gasPrice)
	msg, err := producer.Transfer(alice, bob, 0, 50)
	require.NoError(t, err)

	validator := chain.NewValidator(factory)
	exeCtx := chain.NewExecutionContext(1, miner)

	msgReceipt, err := validator.ApplyMessage(exeCtx, drv.State(), msg)
	require.NoError(t, err)
	require.NotNil(t, msgReceipt)

	expectedGasUsed := 126 // NB: should be derived from the size of the message + some other lotus VM bits
	assert.Equal(t, uint8(0), msgReceipt.ExitCode)
	assert.Empty(t, msgReceipt.ReturnValue)
	assert.Equal(t, state.GasUnit(expectedGasUsed), msgReceipt.GasUsed)

	drv.AssertBalance(alice, uint64(1950 - expectedGasUsed))
	drv.AssertBalance(bob, 50)
	// This should become non-zero after gas tracking and payments are integrated.
	drv.AssertBalance(miner, uint64(expectedGasUsed))

}
