package validation

import (
	"encoding/binary"
	"github.com/filecoin-project/lotus/chain/address"
	"math/big"
	"testing"

	"github.com/libp2p/go-libp2p-core/peer"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/chain-validation/pkg/chain"
	"github.com/filecoin-project/chain-validation/pkg/state"
	"github.com/filecoin-project/chain-validation/pkg/suites"

	"github.com/filecoin-project/lotus/build"
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
func TestLotusCreateStorageMiner(t *testing.T) {
	factory := NewFactories()
	drv := suites.NewStateDriver(t, factory.NewState())
	gasPrice := big.NewInt(1)
	gasLimit := state.GasUnit(2000) // needs to be over ~1100 for lotus operations
	TotalNetworkBalance := big.NewInt(1).Mul(big.NewInt(build.TotalFilecoin), big.NewInt(0).SetUint64(build.FilecoinPrecision))

	_, _, err := drv.State().SetSingletonActor(state.InitAddress, big.NewInt(0))
	require.NoError(t, err)
	_, _, err = drv.State().SetSingletonActor(state.NetworkAddress, TotalNetworkBalance)
	require.NoError(t, err)
	_, _, err = drv.State().SetSingletonActor(state.StoragePowerAddress, big.NewInt(0))
	require.NoError(t, err)


	// miner that mines in this test
	testMiner := drv.NewAccountActor(0)
	// account that will own the miner
	minerOwner := drv.NewAccountActor(20000000000)

	producer := chain.NewMessageProducer(factory.NewMessageFactory(drv.State()), gasLimit, gasPrice)
	validator := chain.NewValidator(factory)

	t.Run("create storage power miner", func(t *testing.T) {
		sectorSize := big.NewInt(16 << 20)
		publicKey := []byte{1} // lotus does not follow spec wrt miner publicKey

		peerID := RequireIntPeerID(t, 1)
		bpid, err := peerID.MarshalBinary()
		require.NoError(t, err)

		msg, err := producer.StoragePowerCreateStorageMiner(minerOwner, 0, minerOwner, publicKey, sectorSize, bpid, chain.Value(1002000))
		require.NoError(t, err)

		exeCtx := chain.NewExecutionContext(1, testMiner)

		msgReceipt, err := validator.ApplyMessage(exeCtx, drv.State(), msg)
		require.NoError(t, err)
		require.NotNil(t, msgReceipt)

		expectedGasUsed := 1703 // NB: should be derived from the size of the message + some other lotus VM bits. Got this value by running the test and inspecting output
		assert.Equal(t, uint8(0), msgReceipt.ExitCode)
		assert.Equal(t, []byte{0, 102}, msgReceipt.ReturnValue)
		assert.Equal(t, state.GasUnit(expectedGasUsed), msgReceipt.GasUsed)

		// TODO make assertions on the state tree to ensure the message application created a miner actor.
	})

	t.Run("update storage power miners storage", func(t *testing.T) {
		// TODO the miner address returned above _could_ be used here instead
		minerAddr, err := address.NewIDAddress(102)
		require.NoError(t, err)

		msg, err := producer.StoragePowerUpdateStorage(state.Address(minerAddr.Bytes()),0, big.NewInt(16<<20), chain.Value(1000000))
		require.NoError(t, err)

		exeCtx := chain.NewExecutionContext(2, testMiner)

		msgReceipt, err := validator.ApplyMessage(exeCtx, drv.State(), msg)
		require.NoError(t, err)
		require.NotNil(t, msgReceipt)

		expectedGasUsed := 327
		assert.Equal(t, uint8(0), msgReceipt.ExitCode)
		assert.Empty(t, msgReceipt.ReturnValue)
		assert.Equal(t, state.GasUnit(expectedGasUsed), msgReceipt.GasUsed)

		// TODO make assertions on the state tree to ensure the message application did something useful.
	})
}

// RequireIntPeerID takes in an integer and creates a unique peer id for it.
func RequireIntPeerID(t *testing.T, i int64) peer.ID {
	buf := make([]byte, 16)
	n := binary.PutVarint(buf, i)
	h, err := mh.Sum(buf[:n], mh.ID, -1)
	require.NoError(t, err)
	pid, err := peer.IDFromBytes(h)
	require.NoError(t, err)
	return pid
}
